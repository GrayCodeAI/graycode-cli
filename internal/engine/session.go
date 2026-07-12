package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/types"

	"github.com/GrayCodeAI/eyrie/storage"
	"github.com/GrayCodeAI/hawk/internal/engine/branching"
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/observability/metrics"
	"github.com/GrayCodeAI/hawk/internal/observability/oteltrace"
	"github.com/GrayCodeAI/hawk/internal/permissions"
	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/prompts"
	modelPkg "github.com/GrayCodeAI/hawk/internal/provider/routing"
	"github.com/GrayCodeAI/hawk/internal/resilience/ratelimit"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/snapshot"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// MemoryRecaller abstracts memory recall/remember so engine avoids importing memory directly.
type MemoryRecaller interface {
	Recall(query string, tokenBudget int) (string, error)
	Remember(content, category string) error
}

// SnapshotTracker abstracts the snapshot system so engine doesn't import snapshot directly.
type SnapshotTracker interface {
	Track(message string) (string, error)
	TrackCtx(ctx context.Context, message string) (string, error)
}

// Session manages a conversation with an LLM via eyrie.
// The mu RWMutex protects messages and system for concurrent access
// (e.g. daemon handling concurrent requests, background memory goroutines).
//
// Phases 1-7 of the god-object decomposition (see
// docs/session-decomposition.md) have extracted the 35-collaborator
// god object into 7 cohesive sub-services. Session is now a thin
// orchestrator that delegates to:
//
//	llm            *ChatService        (Phase 1: LLM transport)
//	perms          *PermissionService  (Phase 2: safety/approval)
//	life           *LifecycleService   (Phase 3: self-improvement loop)
//	memory         *MemoryService      (Phase 4: yaad bridge)
//	persist        *PersistenceService (Phase 5: conversation store)
//	tools          *ToolService        (Phase 6: tool execution)
//
// The legacy fields (client, provider, model, apiKeys, Router,
// DeploymentRouting, RateLimiter, Perm, Permissions, AutoMode,
// Classifier, BypassKill, MaxTurns, MaxBudgetUSD, AllowedDirs,
// PermissionFn, Autonomy, Approval, Memory, YaadBridge, EnhancedMemory,
// messages, system, Cascade, Lifecycle, Reflector, CostTracker,
// Beliefs, Critic, Backtrack, Limits, Trajectory, Shadow, etc.) stay
// on Session for backward compat with code that reads them directly.
// They are all thin forwarders to the new sub-services. The agent
// loop (stream.go) is being migrated to use the sub-services one
// call site at a time. Once every call site is migrated, the
// legacy fields will be removed.
type Session struct {
	mu       sync.RWMutex
	client   ChatClient
	registry *tool.Registry
	messages []types.EyrieMessage
	provider string
	model    string
	apiKeys  map[string]string
	system   string
	log      *logger.Logger
	metrics  *metrics.Registry
	Cost     Cost
	Router   *modelPkg.Router
	// DeploymentRouting is true when the chat client is catalog-backed (e.g. DeploymentRouter).
	//
	// Deprecated: use s.ChatLLM().DeploymentRouting() (Phase 1 sub-service).
	DeploymentRouting bool

	// ContainerExecutor runs Bash in an isolated container when set (no API keys in container env).
	//
	// Deprecated: use s.Tools().ContainerExecutor() (Phase 6 sub-service).
	ContainerExecutor tool.ContainerExecutor
	// ContainerRequired blocks tools until ContainerExecutor is running (container-first mode).
	//
	// Deprecated: use s.Tools().ContainerRequired() (Phase 6 sub-service).
	ContainerRequired bool

	// llm is the LLM transport service (Phase 1 extraction). All new
	// code should go through s.llm.* rather than touching the legacy
	// client/provider/model/apiKeys/Router/DeploymentRouting fields.
	// Named lowercase (unexported) to avoid colliding with the public
	// Session.Chat() method used by Reflector and SelfReview.
	llm *ChatService
	// perms (Phase 2), life (Phase 3), memory (Phase 4), persist
	// (Phase 5), tools (Phase 6) are the remaining 5 sub-services.
	// All optional; nil is the default and the agent loop preserves
	// its `if s.X != nil` branching.
	perms   *PermissionService
	life    *LifecycleService
	memory  *MemoryService
	persist *PersistenceService
	tools   *ToolService

	Perm *PermissionEngine // extracted permission subsystem
	// Backward-compatible accessors below (will be removed after full migration)
	//
	// Deprecated: use s.PermSvc() (Phase 2 sub-service) for all of:
	//   Permissions, AutoMode, Classifier, BypassKill, PermissionFn.
	Permissions *PermissionMemory             // use Perm.Memory
	AutoMode    *permissions.AutoModeState    // use Perm.AutoMode
	Classifier  *permissions.Classifier       // use Perm.Classifier
	BypassKill  *permissions.BypassKillswitch // use Perm.BypassKill
	//
	// Deprecated: use s.LifecycleSvc() (Phase 3 sub-service) for:
	//   MaxBudgetUSD, AllowedDirs, Memory, YaadBridge,
	//   EnhancedMemory, Cascade, Lifecycle, Reflector, CostTracker,
	//   ConvoDAG, Sleeptime, Activity, SkillDistiller, AutoCompactor,
	//   FewShotStore, AdaptivePrompt.
	AllowedDirs  []string
	PermissionFn func(PermissionRequest) // use Perm.PromptFn
	//
	// Deprecated: use s.MemorySvc() (Phase 4 sub-service) for:
	//   Memory, YaadBridge, EnhancedMemory.
	AgentSpawnFn   func(ctx context.Context, prompt string) (string, error)
	AskUserFn      func(question string) (string, error)
	Memory         MemoryRecaller
	YaadBridge     *memory.YaadBridge
	EnhancedMemory *memory.EnhancedMemoryManager
	SettingsGet    func(key string) (string, bool)
	SettingsSet    func(key, value string) error

	PinnedMessages          int // messages to protect from compaction (from /pin)
	AutoCompactThresholdPct int // token % to trigger auto-compact (default 85)
	ContextWindowCached     int // catalog context window; 0 → governor default
	AutoCompactor           *AutoCompactor
	persistID               string
	lastPromptTokens        int
	lastCompletionTokens    int
	estTokensCache          int
	estTokensMsgCount       int
	estTokensLastLen        int
	checkpointMgr           *session.CheckpointManager
	OnCompaction            OnCompaction
	Verbose                 bool // show tool calls, timing, token counts in output
	// GLMThinkingEnabled toggles GLM/Z.ai extended reasoning on outgoing requests
	// (applied only when provider is zai_payg or zai_coding). nil leaves the model default.
	GLMThinkingEnabled *bool

	// Cost optimization
	//
	// Deprecated: use s.LifecycleSvc() (Phase 3 sub-service) for:
	//   Cascade, Lifecycle, Reflector, CostTracker.
	Cascade     *branching.CascadeRouter // cascade.go — model tier routing
	Lifecycle   *SessionLifecycle        // lifecycle.go — self-improvement loop
	Reflector   *Reflector               // reflect.go — verbal self-reflection
	CostTracker *CostTracker             // cost_tracker.go — per-request cost persistence

	// Advanced features
	//
	// Deprecated: most of these have been folded into sub-services;
	// a few remain as legacy fields without a sub-service accessor
	// (Trajectory, LintLoop, TestLoop, FileMentions, Files, Snapshots).
	// For those, keep reading the legacy field — they're
	// populated at session construction and don't have a setter.
	//   Autonomy       -> s.PermSvc().Autonomy()
	//   Sandbox        -> s.Tools().Sandbox()
	//   Plan           -> s.Plan (legacy field; not yet on a sub-service)
	//   Beliefs        -> s.LifecycleSvc().Beliefs()
	//   Critic         -> s.LifecycleSvc().Critic()
	//   Backtrack      -> s.LifecycleSvc().Backtrack()
	//   Limits         -> s.LifecycleSvc().Limits()
	//   Trajectory     -> legacy field; not yet on a sub-service
	//   Shadow         -> s.LifecycleSvc().Shadow()
	//   ConvoDAG       -> s.Persistence().DAG()
	//   Sleeptime      -> s.MemorySvc().Sleeptime()
	//   Activity       -> s.MemorySvc().Activity()
	//   SkillDistiller -> s.MemorySvc().SkillDistiller()
	//   RateLimiter    -> s.RateLimiter (legacy field; not yet on ChatLLM)
	//   AgentsAccum    -> s.LifecycleSvc().AgentsAccum()
	//   FewShotStore   -> s.LifecycleSvc().FewShotStore()
	//   AdaptivePrompt -> s.LifecycleSvc().AdaptivePrompt()
	//   LintLoop       -> legacy field; not yet on a sub-service
	//   TestLoop       -> legacy field; not yet on a sub-service
	//   FileMentions   -> legacy field; not yet on a sub-service
	//   ResponseCache  -> s.LifecycleSvc().ResponseCache()
	//   Pipeline       -> s.LifecycleSvc().Pipeline()
	//   Files          -> legacy field; not yet on Persistence
	//   Steering       -> s.Persistence().Steering()
	//   Snapshots      -> legacy field; not yet on Persistence
	//   Tracer         -> legacy field; oteltrace.NewTracer() for new code
	Autonomy       AutonomyLevel              // autonomy.go — permission level
	Sandbox        *DiffSandbox               // diffsandbox.go — staged file changes
	Plan           *PlanState                 // subtask.go — user-activated plan
	Beliefs        *BeliefState               // belief.go — discovered knowledge
	Critic         *Critic                    // critic.go — patch pre-screening
	Backtrack      *BacktrackEngine           // backtrack.go — decision recording
	Limits         *LimitTracker              // limits.go — safety limits
	Teach          TeachConfig                // teach.go — explanation depth
	Trajectory     *TrajectoryDistiller       // trajectory.go — multi-run distillation
	Shadow         *branching.ShadowWorkspace // shadow.go — edit pre-validation
	Snapshots      SnapshotTracker            // snapshot integration for auto-tracking
	ConvoDAG       *storage.DAG               // conversation DAG for branching/forking
	Sleeptime      *memory.SleeptimeAgent     // sleeptime.go — background memory consolidation
	Activity       *memory.ActivityTracker    // activity.go — memory save nudging (Engram pattern)
	SkillDistiller *memory.SkillDistiller     // skill_distill.go — auto-skill extraction
	Tracer         *oteltrace.Tracer          // oteltrace.go — distributed tracing spans
	LintLoop       *LintLoop                  // lint_loop.go — auto lint-fix reflected messages
	TestLoop       *TestLoop                  // test_loop.go — auto test-fix loop
	FileMentions   *FileMentionDetector       // file_mentions.go — detect referenced files
	ResponseCache  *ResponseCache             // response_cache.go — cache similar prompts
	Pipeline       *IntegrationPipeline       // integration.go — unified feature orchestration
	Files          *FileTracker               // compact_files.go — cumulative file tracking across compactions
	Steering       *SteeringQueue             // steering.go — user guidance injection between tool batches
	RateLimiter    *ratelimit.Limiter         // ratelimit — token bucket for LLM API calls
	AgentsAccum    *prompts.AgentsAccumulator // agents_accumulator.go — auto-capture learnings

	// Few-shot learning and prompt optimization
	//
	// Deprecated: use s.LifecycleSvc() (Phase 3 sub-service) for:
	//   FewShotStore, AdaptivePrompt.
	FewShotStore   *FewShotStore   // scaffold/fewshot.go — successful pattern collection
	AdaptivePrompt *AdaptivePrompt // adaptive_prompt.go — user preference learning

	// OutputSchema, when non-empty, requests a JSON-schema-constrained response.
	// It is plumbed into eyrie's ChatOptions.ResponseFormat (json_schema) and the
	// model output is validated against it. See structured_output.go.
	OutputSchema string // structured_output.go — JSON schema for constrained output

	// Approval, when non-nil and enabled, gates high-risk tool actions behind an
	// explicit human confirmation. Nil keeps existing behavior unchanged.
	Approval *ApprovalGate // approval_gate.go — human-in-the-loop gate

	// smartSkills caches loaded SmartSkills for auto-discovery per-turn.
	smartSkills []plugin.SmartSkill
}

// NewSession creates a new conversation session with a legacy string-named provider.
func NewSession(provider, model, systemPrompt string, registry *tool.Registry) *Session {
	return NewSessionWithClient(types.NewClient(&types.ClientConfig{Provider: provider}), provider, model, systemPrompt, registry, false)
}

// NewSessionWithClient constructs a session with an explicit LLM client (e.g. deployment router).
func NewSessionWithClient(chat ChatClient, provider, model, systemPrompt string, registry *tool.Registry, deploymentRouting bool) *Session {
	if provider == "" || model == "" {
		slog.Debug("NewSessionWithClient called with empty provider or model", "provider", provider, "model", model)
	}
	pe := NewPermissionEngine()
	log := logger.Default()
	s := &Session{
		client:            chat,
		registry:          registry,
		provider:          provider,
		model:             model,
		apiKeys:           map[string]string{},
		system:            systemPrompt,
		log:               log,
		metrics:           metrics.NewRegistry(),
		Perm:              pe,
		Permissions:       pe.Memory,
		AutoMode:          pe.AutoMode,
		Classifier:        pe.Classifier,
		BypassKill:        pe.BypassKill,
		Beliefs:           NewBeliefState(),
		Backtrack:         NewBacktrackEngine(),
		Limits:            NewLimitTracker(DefaultLimits()),
		Tracer:            oteltrace.NewTracer(),
		LintLoop:          NewLintLoop(),
		TestLoop:          NewTestLoop(),
		FileMentions:      NewFileMentionDetector("."),
		ResponseCache:     NewResponseCache(1000, 24*time.Hour),
		Pipeline:          NewIntegrationPipeline(),
		DeploymentRouting: deploymentRouting,
		RateLimiter:       ratelimit.PerSecond(10),
	}
	s.Cost.Model = model
	s.Router = modelPkg.NewRouter(modelPkg.StrategyBalanced)
	s.AutoCompactThresholdPct = DefaultAutoCompactThresholdPct
	s.refreshContextWindowCache()

	// Initialize agents accumulator for project learnings.
	cwd, _ := os.Getwd()
	s.AgentsAccum = prompts.NewAgentsAccumulator(cwd)

	// -----------------------------------------------------------------------
	// Wire the 6 sub-services extracted in Phases 1-6 of the god-object
	// decomposition (see docs/session-decomposition.md). New code should
	// prefer the sub-service getters (s.ChatLLM(), s.PermSvc(), etc.) over
	// the legacy fields. The legacy fields stay on Session for backward
	// compat with external code (cmd/, daemon/, multiagent/, etc.) that
	// reads them directly. They will be removed in a follow-up cleanup PR
	// once all call sites are migrated.
	//
	// For each service whose state is also held as a Session field, we
	// point the Session field at the service's instance so reads stay
	// in sync (the two are aliases, not duplicates).
	// -----------------------------------------------------------------------
	s.llm = NewChatService(chat, ChatServiceConfig{
		Provider:          provider,
		Model:             model,
		APIKeys:           s.apiKeys,
		Router:            s.Router,
		DeploymentRouting: deploymentRouting,
		RateLimiter:       s.RateLimiter,
		Metrics:           s.metrics,
	})
	s.perms = NewPermissionService(log).WithEngine(pe)
	s.life = NewLifecycleService(log)
	s.memory = NewMemoryService(log)
	s.persist = NewPersistenceService(log)
	s.persist.SetSystem(systemPrompt)
	s.tools = NewToolService(registry)

	// Emergency compact: when a provider rejects a request as too large,
	// ChatService.Stream calls this to shrink the conversation in place and
	// retries with the compacted history. Installed after s.persist exists
	// because the hook reads and rewrites the persisted messages.
	s.llm.SetOnContextOverflow(func(ctx context.Context) ([]types.EyrieMessage, bool) {
		before := len(s.Persistence().RawMessages())
		s.smartCompact()
		after := s.Persistence().RawMessages()
		if len(after) >= before {
			return nil, false
		}
		s.log.Info("emergency compact after context overflow", map[string]interface{}{
			"messages_before": before, "messages_after": len(after),
		})
		return after, true
	})

	// Alias legacy fields at the service instances so legacy readers see
	// the same state as new code that goes through the sub-service getters.
	// After this point, mutations to the sub-service internal state
	// (e.g., s.memory.SetMemory(...)) need a corresponding write to the
	// legacy field — see the various Set* helpers (SetConvoDAG,
	// SetSnapshots, etc.) which perform the dual write.
	s.Limits = s.life.Limits()
	s.Beliefs = s.life.Beliefs()
	s.Backtrack = s.life.Backtrack()
	s.ResponseCache = s.life.ResponseCache()
	s.Pipeline = s.life.Pipeline()
	// Fields read by AddUser/AddAssistant/AddUserWithImage/ForkConversation/
	// SwitchBranch: alias them so legacy direct-field reads return
	// the sub-service state.
	s.Memory = s.memory.Memory()
	s.YaadBridge = s.memory.Yaad()
	s.EnhancedMemory = s.memory.Enhanced()
	s.ConvoDAG = s.persist.DAG()
	s.Steering = s.persist.Steering()

	return s
}

// ReattachTransport swaps the LLM client after deployment routing or provider.json changes.
// Also reattaches the ChatService so the agent loop's `s.ChatLLM().Stream`
// call site picks up the new client (Phase 7 migration).
func (s *Session) ReattachTransport(chat ChatClient, provider string, deploymentRouting bool) {
	if chat == nil {
		return
	}
	s.client = chat
	if strings.TrimSpace(provider) != "" {
		s.provider = strings.TrimSpace(provider)
	}
	s.DeploymentRouting = deploymentRouting
	if s.llm != nil {
		s.llm.Reattach(chat, s.provider)
	}
	for name, key := range s.apiKeys {
		if strings.TrimSpace(key) != "" {
			s.client.SetAPIKey(name, key)
		}
	}
}

// SubSession clones transport and routing mode for explore/general sub-agents.
func (s *Session) SubSession(model, systemPrompt string, registry *tool.Registry) *Session {
	if registry == nil {
		registry = s.registry
	}
	sub := NewSessionWithClient(s.client, s.provider, model, systemPrompt, registry, s.DeploymentRouting)
	for provider, key := range s.apiKeys {
		sub.SetAPIKey(provider, key)
	}
	return sub
}

func (s *Session) Model() string              { return s.model }
func (s *Session) Provider() string           { return s.provider }
func (s *Session) Metrics() *metrics.Registry { return s.metrics }

// ChatLLM returns the extracted ChatService (Phase 1 of the god-object
// decomposition). New code should prefer this over the legacy Client /
// Provider / Model / APIKeys / Router fields. Returns nil only if the
// session was constructed without going through NewSessionWithClient,
// which should not happen in production.
func (s *Session) ChatLLM() *ChatService { return s.llm }

// PermSvc returns the extracted PermissionService (Phase 2). Returns
// nil only if the session was constructed without
// NewSessionWithClient, which should not happen in production.
func (s *Session) PermSvc() *PermissionService { return s.perms }

// LifecycleSvc returns the extracted LifecycleService (Phase 3).
func (s *Session) LifecycleSvc() *LifecycleService { return s.life }

// MemorySvc returns the extracted MemoryService (Phase 4).
func (s *Session) MemorySvc() *MemoryService { return s.memory }

// Persistence returns the extracted PersistenceService (Phase 5).
// Provides the messages slice and system prompt (read/write) with
// the underlying RWMutex.
func (s *Session) Persistence() *PersistenceService { return s.persist }

// Tools returns the extracted ToolService (Phase 6).
func (s *Session) Tools() *ToolService { return s.tools }

// SubServices is the composed view of the 6 sub-services extracted
// in Phases 1-6 of the god-object decomposition. New code should
// prefer the SubServices() accessor over the legacy Session fields.
// Existing code (cmd/, daemon/, multiagent/, …) continues to use
// the legacy fields until they're migrated.
//
// SubServices is a struct (not an interface) because all 6
// sub-services are concrete types; this keeps the API discoverable
// via godoc and avoids the indirection cost of interface dispatch
// on the agent-loop hot path.
//
// Note: this is distinct from the older *SessionServices returned
// by Services(), which is a bridge view over the LEGACY fields
// (CoreLoop, SafetyLayer, Intelligence, etc.). SubServices is the
// new canonical view; SessionServices will be removed once legacy
// migration is complete.
type SubServices struct {
	LLM         *ChatService
	Perms       *PermissionService
	Life        *LifecycleService
	Memory      *MemoryService
	Persistence *PersistenceService
	Tools       *ToolService
}

// SubServices returns the 6 new sub-services. All sub-services are
// non-nil for a session constructed via NewSessionWithClient (the
// only production constructor); the nil cases are reachable only
// via direct struct literal construction in tests.
func (s *Session) SubServices() SubServices {
	return SubServices{
		LLM:         s.llm,
		Perms:       s.perms,
		Life:        s.life,
		Memory:      s.memory,
		Persistence: s.persist,
		Tools:       s.tools,
	}
}

// SetModel updates the active model for subsequent requests.
func (s *Session) SetModel(model string) {
	s.model = strings.TrimSpace(model)
	s.Cost.Model = s.model
	s.syncCascadeDefaultModel()
	s.refreshContextWindowCache()
}

// syncCascadeDefaultModel keeps the cascade router aligned after /config model picks.
func (s *Session) syncCascadeDefaultModel() {
	if s == nil || s.Cascade == nil {
		return
	}
	if m := strings.TrimSpace(s.model); m != "" {
		s.Cascade.DefaultModel = m
		s.Cascade.Roles = modelPkg.DefaultRoles(m)
	}
}

// SetProvider updates the active provider for subsequent requests.
func (s *Session) SetProvider(provider string) {
	p := strings.TrimSpace(provider)
	s.provider = p
	if s.DeploymentRouting {
		return
	}
	s.client = types.NewClient(&types.ClientConfig{Provider: p})
	// Copy keys to avoid map iteration race with concurrent SetAPIKey calls.
	keys := make(map[string]string, len(s.apiKeys))
	for k, v := range s.apiKeys {
		keys[k] = v
	}
	for provider, apiKey := range keys {
		if strings.TrimSpace(apiKey) != "" {
			s.client.SetAPIKey(provider, apiKey)
		}
	}
}

// SetAPIKey updates a provider API key for subsequent requests.
func (s *Session) SetAPIKey(provider, apiKey string) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	apiKey = strings.TrimSpace(apiKey)
	if provider == "" || apiKey == "" {
		return
	}
	if s.apiKeys == nil {
		s.apiKeys = map[string]string{}
	}
	s.apiKeys[provider] = apiKey
	if s.client != nil {
		s.client.SetAPIKey(provider, apiKey)
	}
}

// SetAPIKeys updates all known provider API keys for subsequent requests.
func (s *Session) SetAPIKeys(apiKeys map[string]string) {
	for provider, apiKey := range apiKeys {
		s.SetAPIKey(provider, apiKey)
	}
}

func (s *Session) AddUser(content string) {
	if p := s.Persistence(); p != nil {
		p.AddUser(content)
		if dag := p.DAG(); dag != nil {
			parentID := ""
			if head, err := dag.Head(context.Background()); err == nil && head != nil {
				parentID = head.ID
			}
			_, _ = dag.Append(context.Background(), parentID, "user", content)
		}
	}
	if memSvc := s.MemorySvc(); memSvc != nil {
		if mem := memSvc.Memory(); mem != nil && strings.Contains(strings.ToLower(content), "remember") {
			go func(c string) {
				// Use timeout context so goroutine doesn't hang if backend is slow.
				rCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = rCtx // timeout context available if Remember is extended to accept it
				if err := mem.Remember(c, "user_explicit"); err != nil {
					slog.Warn("background memory remember failed", "error", err)
				}
			}(content)
		}
	}
}

// AddUserWithImage adds a user message with an attached image (base64-encoded).
// The imageType should be "image/png", "image/jpeg", etc.
func (s *Session) AddUserWithImage(content string, imageBase64 string, imageType string) {
	if p := s.Persistence(); p != nil {
		p.AddUser(content + " [image attached]")
		if dag := p.DAG(); dag != nil {
			parentID := ""
			if head, err := dag.Head(context.Background()); err == nil && head != nil {
				parentID = head.ID
			}
			_, _ = dag.Append(context.Background(), parentID, "user", content+" [image attached]")
		}
	}
	s.mu.Lock()
	msg := types.EyrieMessage{
		Role:    "user",
		Content: content,
		Images:  []string{"data:" + imageType + ";base64," + imageBase64},
	}
	s.messages = append(s.messages, msg)
	s.mu.Unlock()
}

func (s *Session) AddAssistant(content string) {
	if p := s.Persistence(); p != nil {
		p.AddAssistant(content)
		if dag := p.DAG(); dag != nil {
			parentID := ""
			if head, err := dag.Head(context.Background()); err == nil && head != nil {
				parentID = head.ID
			}
			_, _ = dag.Append(context.Background(), parentID, "assistant", content)
		}
	}
}

// ForkConversation creates a new branch from a specific point in history.
// Returns the fork node ID and the messages up to that point.
func (s *Session) ForkConversation(nodeID string) (string, error) {
	p := s.Persistence()
	if p == nil {
		return "", nil
	}
	dag := p.DAG()
	if dag == nil {
		return "", nil
	}
	fork, err := dag.Fork(context.Background(), nodeID)
	if err != nil {
		return "", err
	}
	// Rebuild messages from the forked branch.
	history, err := dag.History(context.Background(), fork.ID)
	if err != nil {
		return "", err
	}
	msgs := make([]types.EyrieMessage, 0, len(history))
	for _, node := range history {
		if node.Role == "user" || node.Role == "assistant" {
			msgs = append(msgs, types.EyrieMessage{Role: node.Role, Content: node.Content})
		}
	}
	p.SetRawMessages(msgs)
	s.mu.Lock()
	s.messages = append(s.messages[:0], msgs...)
	s.mu.Unlock()
	return fork.ID, nil
}

// SwitchBranch navigates to a different branch point and rebuilds messages.
func (s *Session) SwitchBranch(nodeID string) error {
	p := s.Persistence()
	if p == nil {
		return nil
	}
	dag := p.DAG()
	if dag == nil {
		return nil
	}
	if err := dag.SetHead(context.Background(), nodeID); err != nil {
		return err
	}
	history, err := dag.History(context.Background(), nodeID)
	if err != nil {
		return err
	}
	msgs := make([]types.EyrieMessage, 0, len(history))
	for _, node := range history {
		if node.Role == "user" || node.Role == "assistant" {
			msgs = append(msgs, types.EyrieMessage{Role: node.Role, Content: node.Content})
		}
	}
	p.SetRawMessages(msgs)
	s.mu.Lock()
	s.messages = append(s.messages[:0], msgs...)
	s.mu.Unlock()
	return nil
}

// ListBranches returns child nodes (alternative branches) from a given node.
func (s *Session) ListBranches(nodeID string) ([]*storage.DAGNode, error) {
	p := s.Persistence()
	if p == nil {
		return nil, nil
	}
	dag := p.DAG()
	if dag == nil {
		return nil, nil
	}
	return dag.Branches(context.Background(), nodeID)
}

// ConvoHead returns the current conversation head node ID.
func (s *Session) ConvoHead() string {
	p := s.Persistence()
	if p == nil {
		return ""
	}
	dag := p.DAG()
	if dag == nil {
		return ""
	}
	if head, err := dag.Head(context.Background()); err == nil && head != nil {
		return head.ID
	}
	return ""
}

// AppendSystemContext adds runtime context, such as /add-dir, to future model calls.
func (s *Session) AppendSystemContext(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	s.mu.Lock()
	if strings.TrimSpace(s.system) == "" {
		s.system = content
	} else {
		s.system += "\n\n" + content
	}
	updated := s.system
	persist := s.persist
	s.mu.Unlock()
	if persist != nil {
		persist.SetSystem(updated)
	}
}

// ReplaceSystemContextSection replaces the content of a system prompt section identified by its header.
// If the header is not found, appends the content as a new section.
func (s *Session) ReplaceSystemContextSection(header, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	s.mu.Lock()
	idx := strings.Index(s.system, header)
	if idx < 0 {
		// AppendSystemContext is not called here to avoid double-locking;
		// replicate its logic inline.
		if strings.TrimSpace(s.system) == "" {
			s.system = content
		} else {
			s.system += "\n\n" + content
		}
		updated := s.system
		persist := s.persist
		s.mu.Unlock()
		if persist != nil {
			persist.SetSystem(updated)
		}
		return
	}
	rest := s.system[idx+len(header):]
	endIdx := strings.Index(rest, "\n\n## ")
	if endIdx < 0 {
		s.system = s.system[:idx] + content
	} else {
		s.system = s.system[:idx] + content + rest[endIdx:]
	}
	updated := s.system
	persist := s.persist
	s.mu.Unlock()
	if persist != nil {
		persist.SetSystem(updated)
	}
}

// SetLogger replaces the session logger.
func (s *Session) SetLogger(l *logger.Logger) {
	s.log = l
}

// SetAllowedDirs sets directories that file tools are allowed to access.
func (s *Session) SetAllowedDirs(dirs []string) {
	s.AllowedDirs = append([]string(nil), dirs...)
}

// SetAutoCompactThresholdPct sets the auto-compact threshold.
// New code should call this instead of writing to the legacy
// s.AutoCompactThresholdPct field directly.
func (s *Session) SetAutoCompactThresholdPct(pct int) {
	s.AutoCompactThresholdPct = pct
}

// SetPinnedMessages sets the number of recent messages that are
// protected from compaction. New code should call this instead of
// writing to the legacy s.PinnedMessages field directly.
func (s *Session) SetPinnedMessages(n int) {
	s.PinnedMessages = n
	if s.persist != nil {
		s.persist.SetPinnedMessages(n)
	}
}

// SetGLMThinkingEnabled sets the GLM/Z.AI extended-reasoning toggle.
// New code should call this instead of writing to the legacy
// s.GLMThinkingEnabled field directly.
func (s *Session) SetGLMThinkingEnabled(v *bool) {
	s.GLMThinkingEnabled = v
}

// SetSnapshots attaches the snapshot tracker. New code should call
// this instead of writing to the legacy s.Snapshots field directly.
func (s *Session) SetSnapshots(snap *snapshot.Tracker) {
	s.Snapshots = snap
}

// SetContainerRequired sets the container-first mode flag.
// New code should call this instead of writing to the legacy
// s.ContainerRequired field directly.
func (s *Session) SetContainerRequired(v bool) {
	s.ContainerRequired = v
}

// SetContainerExecutor sets the container executor and updates
// the ToolService so the legacy s.ContainerExecutor field and
// s.Tools().ContainerExecutor() stay in sync.
func (s *Session) SetContainerExecutor(ce tool.ContainerExecutor) {
	s.ContainerExecutor = ce
	if s.tools != nil {
		s.tools.WithContainerExecutor(ce, s.ContainerRequired)
	}
}

// SetAskUserFn sets the user-prompt callback. New code should
// call this instead of writing to the legacy s.AskUserFn field.
func (s *Session) SetAskUserFn(fn func(question string) (string, error)) {
	s.AskUserFn = fn
}

// SetApproval sets the high-risk action gate. New code should
// call this instead of writing to the legacy s.Approval field.
func (s *Session) SetApproval(a *ApprovalGate) {
	s.Approval = a
}

// SetConvoDAG attaches the conversation DAG. New code should
// call this instead of writing to the legacy s.ConvoDAG field.
// The DAG is also propagated to the persistence service so
// s.Persistence().DAG() and the legacy field stay in sync.
func (s *Session) SetConvoDAG(dag *storage.DAG) {
	s.ConvoDAG = dag
	if s.persist != nil {
		s.persist.SetDAG(dag)
	}
}

// SetContextWindowCached sets the catalog context window. New code
// should call this instead of writing to the legacy
// s.ContextWindowCached field directly.
func (s *Session) SetContextWindowCached(n int) {
	s.ContextWindowCached = n
	if s.persist != nil {
		s.persist.SetContextWindowCached(n)
	}
}

// ContextWindowCachedValue returns the cached context window size.
// New code should call this instead of reading s.ContextWindowCached
// directly. Falls back to the legacy field for back-compat with
// code paths that still write to s.ContextWindowCached.
func (s *Session) ContextWindowCachedValue() int {
	if s.persist != nil {
		if w := s.persist.ContextWindowCached(); w > 0 {
			return w
		}
	}
	return s.ContextWindowCached
}

// CostValue returns the session's cost accumulator (a pointer
// to a value type, so its methods can be called). New code
// should call this instead of reading s.Cost directly.
func (s *Session) CostValue() *Cost {
	return &s.Cost
}

func (s *Session) LoadMessages(msgs []types.EyrieMessage) {
	s.Persistence().SetRawMessages(msgs)
}

func (s *Session) MessageCount() int {
	return len(s.Persistence().RawMessages())
}

// RawMessages returns the conversation messages for persistence.
//
// PersistenceService is the single source of truth for the live transcript:
// AddUser/AddAssistant and the agent loop (stream.go) all write through it,
// and compaction/governor paths read it. The legacy s.messages field is kept
// only for Sessions constructed without a PersistenceService (some unit
// tests). Delegating here means TUI/CLI consumers — notably saveSession,
// which returned early when the legacy slice was empty — see the real,
// populated transcript instead of a stale empty slice.
func (s *Session) RawMessages() []types.EyrieMessage {
	if p := s.Persistence(); p != nil {
		return p.RawMessages()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.messages
}

// Chat implements the LLMClient interface by delegating to the underlying client.
// This allows Session to be passed to components that need LLM access (e.g. Reflector, SelfReview).
func (s *Session) Chat(ctx context.Context, msgs []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	if s.client == nil {
		return nil, fmt.Errorf("session: no LLM client configured")
	}
	return s.client.Chat(ctx, msgs, opts)
}

// RemoveLastExchange removes the last user+assistant message pair.
func (s *Session) RemoveLastExchange() {
	msgs := s.Persistence().RawMessages()
	if len(msgs) < 2 {
		return
	}
	// Remove from the end until we've removed one user and one assistant message
	removed := 0
	for i := len(msgs) - 1; i >= 0 && removed < 2; i-- {
		role := msgs[i].Role
		if role == "user" || role == "assistant" {
			removed++
			msgs = msgs[:i]
		}
	}
	s.Persistence().SetRawMessages(msgs)
}

// StreamEvent is sent from the engine to the TUI.
type StreamEvent struct {
	Type     string // content, thinking, tool_use, tool_result, usage, compact, done, error
	Content  string
	ToolName string
	ToolID   string
	Usage    *StreamUsage // usage data for this event
	// Compaction metadata (Type == "compact")
	TokensBefore int
	TokensAfter  int
}

// StreamUsage tracks token usage for a single stream event.
type StreamUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}
