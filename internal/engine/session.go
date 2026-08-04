package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	agentcontracts "github.com/GrayCodeAI/hawk-core-contracts/agent"
	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
	"github.com/GrayCodeAI/hawk/internal/types"

	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/observability/metrics"
	"github.com/GrayCodeAI/hawk/internal/observability/oteltrace"
	"github.com/GrayCodeAI/hawk/internal/prompts"
	"github.com/GrayCodeAI/hawk/internal/resilience/ratelimit"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/snapshot"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/tok"
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
// The mu RWMutex protects the remaining session metadata for concurrent
// access. Transcript and system-context state are owned by PersistenceService.
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
// Session retains only orchestration state and integrations that do not yet
// have a dedicated service. Permission, tool execution, transcript, memory,
// and lifecycle state are owned by the corresponding services below.
type Session struct {
	mu      sync.RWMutex
	log     *logger.Logger
	metrics *metrics.Registry
	Cost    Cost

	// llm is the LLM transport service (Phase 1 extraction). All new
	// code should go through s.llm.* rather than duplicating transport state.
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
	// Permission and approval state is owned exclusively by PermissionService.
	// readOnlyBash gates Bash via ExploreBashAllowed for explore/plan subagents.
	readOnlyBash bool
	// workingDir is the preferred cwd for tools (worktree isolation).
	workingDir string

	tokUsage *tok.UsageTracker
	// GLMThinkingEnabled toggles GLM/Z.ai extended reasoning on outgoing requests
	// (applied only when provider is zai_payg or zai_coding). nil leaves the model default.

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
	//   ConversationGraph -> s.Persistence().Graph()
	//   Sleeptime      -> s.MemorySvc().Sleeptime()
	//   Activity       -> s.MemorySvc().Activity()
	//   SkillDistiller -> s.MemorySvc().SkillDistiller()
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
	// Backtrack and limits are owned by LifecycleService.
}

// NewSession creates a conversation session through Eyrie's engine facade.
func NewSession(provider, model, systemPrompt string, registry *tool.Registry) *Session {
	return NewHawkSession(context.Background(), gateway.Selection{
		Provider: provider,
		Model:    model,
	}, provider, model, systemPrompt, registry)
}

// NewSessionWithClient constructs a session with an explicit LLM client (e.g. deployment router).
func NewSessionWithClient(chat ChatClient, provider, model, systemPrompt string, registry *tool.Registry, deploymentRouting bool) *Session {
	if provider == "" || model == "" {
		slog.Debug("NewSessionWithClient called with empty provider or model", "provider", provider, "model", model)
	}
	log := logger.Default()
	s := &Session{
		log:     log,
		metrics: metrics.NewRegistry(),
	}
	rateLimiter := ratelimit.PerSecond(10)
	s.Cost.Model = model
	s.refreshContextWindowCache()

	// Initialize agents accumulator for project learnings.
	cwd, _ := os.Getwd()
	agentsAccum := prompts.NewAgentsAccumulator(cwd)

	// -----------------------------------------------------------------------
	// Wire the 6 sub-services extracted in Phases 1-6 of the god-object
	// decomposition (see docs/session-decomposition.md). New code should
	// prefer the sub-service getters (s.ChatLLM(), s.PermSvc(), etc.).
	// -----------------------------------------------------------------------
	s.llm = NewChatService(chat, ChatServiceConfig{
		Provider:          provider,
		Model:             model,
		DeploymentRouting: deploymentRouting,
		RateLimiter:       rateLimiter,
		Metrics:           s.metrics,
	})
	s.perms = NewPermissionService(log)
	s.life = NewLifecycleService(log)
	s.memory = NewMemoryService(log)
	s.persist = NewPersistenceService(log)
	s.persist.SetAutoCompactThresholdPct(DefaultAutoCompactThresholdPct)
	s.persist.SetSystem(systemPrompt)
	s.tools = NewToolService(registry).WithMetrics(s.metrics).WithTracer(oteltrace.NewTracer())
	s.tools.WithExecutionDeps(toolExecutionDeps{
		permissions: s.perms,
		chat:        s.llm,
		memory:      s.memory,
		agentSpawn: func(ctx context.Context, req agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
			if s.tools.AgentSpawnFn() == nil {
				return agentcontracts.SpawnResult{Status: agentcontracts.StatusFailed, Error: "agent spawning is unavailable"}, fmt.Errorf("agent spawning is unavailable")
			}
			return s.tools.AgentSpawnFn()(ctx, req)
		},
		askUser: func(question string) (string, error) {
			if s.perms.AskUserFn() == nil {
				return "", fmt.Errorf("ask-user callback is unavailable")
			}
			return s.perms.AskUserFn()(question)
		},
		readOnlyBash:       s.readOnlyBash,
		workingDir:         s.workingDir,
		checkApproval:      s.CheckApproval,
		recordPolicy:       s.recordPolicyObservation,
		recordVerification: s.recordVerificationObservation,
		lifecycle:          s.life,
		appendSystem:       s.AppendSystemContext,
	})
	s.refreshContextWindowCache()
	s.life.SetAgentsAccumulator(agentsAccum)
	s.life.SetLintLoop(NewLintLoop())
	s.life.SetTestLoop(NewTestLoop())
	return s
}

// ReattachTransport swaps the LLM client after deployment routing or provider.json changes.
// Also reattaches the ChatService so the agent loop's `s.ChatLLM().Stream`
// call site picks up the new client (Phase 7 migration).
func (s *Session) ReattachTransport(chat ChatClient, provider string, deploymentRouting bool) {
	if chat == nil {
		return
	}
	if llm := s.ChatLLM(); llm != nil {
		llm.Reattach(chat, strings.TrimSpace(provider))
	}
	// deploymentRouting is now read through ChatService; the ChatService
	// constructed at session creation already holds the value. If a
	// future path needs to toggle it post-construction, extend
	// ChatService with a setter.
	_ = deploymentRouting
}

// SubSession clones transport and routing mode for explore/general sub-agents.
func (s *Session) SubSession(model, systemPrompt string, registry *tool.Registry) *Session {
	if registry == nil {
		if tools := s.Tools(); tools != nil {
			registry = tools.Registry()
		}
	}
	var chat ChatClient
	provider := ""
	deploymentRouting := false
	if llm := s.ChatLLM(); llm != nil {
		chat = llm.Client()
		provider = llm.Provider()
		deploymentRouting = llm.DeploymentRouting()
	}
	sub := NewSessionWithClient(chat, provider, model, systemPrompt, registry, deploymentRouting)
	return sub
}

func (s *Session) Model() string {
	if llm := s.ChatLLM(); llm != nil {
		return llm.Model()
	}
	return ""
}

func (s *Session) Provider() string {
	if llm := s.ChatLLM(); llm != nil {
		return llm.Provider()
	}
	return ""
}
func (s *Session) Metrics() *metrics.Registry { return s.metrics }

// Logger returns the session logger through the observability boundary.
func (s *Session) Logger() *logger.Logger { return s.log }

// TracerValue returns the session tracer through the observability boundary.
func (s *Session) TracerValue() *oteltrace.Tracer {
	if s == nil || s.tools == nil {
		return nil
	}
	return s.tools.Tracer()
}

// ChatLLM returns the extracted ChatService (Phase 1 of the god-object
// decomposition). New code should prefer this over the legacy Client /
// Provider / Model / Router fields. Returns nil only if the
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
func (s *Session) Persistence() *PersistenceService {
	if s == nil {
		return nil
	}
	if s.persist != nil {
		return s.persist
	}
	// A zero-value Session can still be used by narrow UI/test adapters. Keep
	// lazy service materialization for that compatibility case, but there is no
	// second transcript or system-prompt state to import.
	s.persist = NewPersistenceService(s.log)
	return s.persist
}

// Tools returns the extracted ToolService (Phase 6).
func (s *Session) Tools() *ToolService { return s.tools }

// DeploymentRouting reports whether the chat client is catalog-backed
// (e.g. DeploymentRouter). Read through to the ChatService so there is
// no separate stored field to drift out of sync on ReattachTransport.
func (s *Session) DeploymentRouting() bool {
	if s.llm != nil {
		return s.llm.DeploymentRouting()
	}
	return false
}

// ContainerExecutor returns the executor that runs Bash in an isolated
// container, or nil. Read through to the ToolService.
func (s *Session) ContainerExecutor() tool.ContainerExecutor {
	if s.tools != nil {
		return s.tools.ContainerExecutor()
	}
	return nil
}

// ContainerRequired reports whether tools are blocked until the
// container executor is running (container-first mode). Read through to
// the ToolService.
func (s *Session) ContainerRequired() bool {
	if s.tools != nil {
		return s.tools.ContainerRequired()
	}
	return false
}

// SubServices is the composed view of the 6 sub-services extracted
// in Phases 1-6 of the god-object decomposition. New code should
// prefer the SubServices() accessor over direct Session state.
// Existing code (cmd/, daemon/, multiagent/, …) continues to use
// the legacy fields until they're migrated.
//
// SubServices is a struct (not an interface) because all 6
// sub-services are concrete types; this keeps the API discoverable
// via godoc and avoids the indirection cost of interface dispatch
// on the agent-loop hot path.
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
	if s == nil {
		return SubServices{}
	}
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
	m := strings.TrimSpace(model)
	s.mu.Lock()
	s.Cost.Model = m
	s.mu.Unlock()
	if s.llm != nil {
		s.llm.SetModel(m)
	}
	s.syncCascadeDefaultModel()
	s.refreshContextWindowCache()
}

// syncCascadeDefaultModel keeps the cascade router aligned after /config model picks.
func (s *Session) syncCascadeDefaultModel() {
	if s == nil || s.LifecycleSvc() == nil || s.LifecycleSvc().Cascade() == nil {
		return
	}
	if m := strings.TrimSpace(s.Model()); m != "" {
		cascade := s.LifecycleSvc().Cascade()
		cascade.DefaultModel = m
	}
}

// SetProvider updates the active provider for subsequent requests.
func (s *Session) SetProvider(provider string) {
	p := strings.TrimSpace(provider)
	if llm := s.ChatLLM(); llm != nil {
		llm.SetProvider(p)
	}
}

func (s *Session) AddUser(content string) {
	if p := s.Persistence(); p != nil {
		p.AddUser(content)
		if graph := p.Graph(); graph != nil {
			parentID := ""
			if head, err := graph.Head(); err == nil && head != nil {
				parentID = head.ID
			}
			_, _ = graph.Append(parentID, "user", content)
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
		p.AddUserWithImage(content, imageBase64, imageType)
		if graph := p.Graph(); graph != nil {
			parentID := ""
			if head, err := graph.Head(); err == nil && head != nil {
				parentID = head.ID
			}
			_, _ = graph.Append(parentID, "user", content+" [image attached]")
		}
	}
}

func (s *Session) AddAssistant(content string) {
	if p := s.Persistence(); p != nil {
		p.AddAssistant(content)
		if graph := p.Graph(); graph != nil {
			parentID := ""
			if head, err := graph.Head(); err == nil && head != nil {
				parentID = head.ID
			}
			_, _ = graph.Append(parentID, "assistant", content)
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
	graph := p.Graph()
	if graph == nil {
		return "", nil
	}
	fork, err := graph.Fork(nodeID)
	if err != nil {
		return "", err
	}
	// Rebuild messages from the forked branch.
	history, err := graph.History(fork.ID)
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
	return fork.ID, nil
}

// SwitchBranch navigates to a different branch point and rebuilds messages.
func (s *Session) SwitchBranch(nodeID string) error {
	p := s.Persistence()
	if p == nil {
		return nil
	}
	graph := p.Graph()
	if graph == nil {
		return nil
	}
	if err := graph.SetHead(nodeID); err != nil {
		return err
	}
	history, err := graph.History(nodeID)
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
	return nil
}

// ListBranches returns child nodes (alternative branches) from a given node.
func (s *Session) ListBranches(nodeID string) ([]*session.ConversationNode, error) {
	p := s.Persistence()
	if p == nil {
		return nil, nil
	}
	graph := p.Graph()
	if graph == nil {
		return nil, nil
	}
	return graph.Branches(nodeID)
}

// ConvoHead returns the current conversation head node ID.
func (s *Session) ConvoHead() string {
	p := s.Persistence()
	if p == nil {
		return ""
	}
	graph := p.Graph()
	if graph == nil {
		return ""
	}
	if head, err := graph.Head(); err == nil && head != nil {
		return head.ID
	}
	return ""
}

// AppendSystemContext adds runtime context, such as /add-dir, to future model calls.
func (s *Session) AppendSystemContext(content string) {
	if p := s.Persistence(); p != nil {
		p.AppendSystemContext(content)
	}
}

// ReplaceSystemContextSection replaces the content of a system prompt section identified by its header.
// If the header is not found, appends the content as a new section.
func (s *Session) ReplaceSystemContextSection(header, content string) {
	if p := s.Persistence(); p != nil {
		p.ReplaceSystemContextSection(header, content)
	}
}

// SetLogger replaces the session logger.
func (s *Session) SetLogger(l *logger.Logger) {
	s.log = l
}

// SetAllowedDirs sets directories that file tools are allowed to access.
func (s *Session) SetAllowedDirs(dirs []string) {
	if s.perms != nil {
		s.perms.SetAllowedDirs(dirs)
	}
}

// SetAutoCompactThresholdPct sets the auto-compact threshold.
func (s *Session) SetAutoCompactThresholdPct(pct int) {
	if s.persist != nil {
		s.persist.SetAutoCompactThresholdPct(pct)
	}
}

// SetPinnedMessages sets the number of recent messages protected from compaction.
func (s *Session) SetPinnedMessages(n int) {
	if s.persist != nil {
		s.persist.SetPinnedMessages(n)
	}
}

// SetThinkingEnabled sets the generic host thinking/reasoning toggle on
// the ChatService (the source of truth).
func (s *Session) SetThinkingEnabled(v *bool) {
	if s.llm != nil {
		s.llm.SetThinkingEnabled(v)
	}
}

// SetGLMThinkingEnabled is a deprecated alias of SetThinkingEnabled.
func (s *Session) SetGLMThinkingEnabled(v *bool) {
	s.SetThinkingEnabled(v)
}

// SetSnapshots attaches the snapshot tracker. New code should call
// this instead of writing to the legacy s.Snapshots field directly.
func (s *Session) SetSnapshots(snap *snapshot.Tracker) {
	if s.tools != nil {
		s.tools.WithSnapshots(snap)
	}
}

// SetContainerRequired sets the container-first mode flag on the
// ToolService (the source of truth).
func (s *Session) SetContainerRequired(v bool) {
	if s.tools != nil {
		s.tools.WithContainerExecutor(s.tools.ContainerExecutor(), v)
	}
}

// SetContainerExecutor sets the container executor on the ToolService
// (the source of truth), preserving the current required flag.
func (s *Session) SetContainerExecutor(ce tool.ContainerExecutor) {
	if s.tools != nil {
		s.tools.WithContainerExecutor(ce, s.ContainerRequired())
	}
}

// SetAskUserFn sets the user-prompt callback. New code should
// call this instead of writing to the legacy s.AskUserFn field.
func (s *Session) SetAskUserFn(fn func(question string) (string, error)) {
	if s.perms != nil {
		s.perms.SetAskUserFn(fn)
	}
}

// SetPermissionFn configures the permission callback on PermissionService.
func (s *Session) SetPermissionFn(fn func(PermissionRequest)) {
	if s.perms != nil {
		s.perms.SetPermissionFn(fn)
	}
}

// SetApproval sets the high-risk action gate on PermissionService.
func (s *Session) SetApproval(a *ApprovalGate) {
	if s.perms != nil {
		s.perms.SetApproval(a)
	}
}

// SetConversationGraph attaches Hawk's product-owned conversation graph and
// seeds it from an already-resumed linear transcript when the graph is new.
func (s *Session) SetConversationGraph(graph *session.ConversationGraph) {
	if s.persist != nil {
		s.persist.SetGraph(graph)
		if graph != nil && graph.Empty() {
			parentID := ""
			for _, message := range s.persist.RawMessages() {
				if message.Role != "user" && message.Role != "assistant" {
					continue
				}
				node, err := graph.Append(parentID, message.Role, message.Content)
				if err != nil {
					break
				}
				parentID = node.ID
			}
		}
	}
}

// SetContextWindowCached sets the catalog context window.
func (s *Session) SetContextWindowCached(n int) {
	if s.persist != nil {
		s.persist.SetContextWindowCached(n)
	}
}

// ContextWindowCachedValue returns the cached context window size.
func (s *Session) ContextWindowCachedValue() int {
	if s.persist != nil {
		return s.persist.ContextWindowCached()
	}
	return 0
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
// and compaction/governor paths read it. Delegating here means TUI/CLI
// consumers — notably saveSession — see the real, populated transcript.
func (s *Session) RawMessages() []types.EyrieMessage {
	if p := s.Persistence(); p != nil {
		return p.RawMessages()
	}
	return nil
}

// Chat implements the LLMClient interface by delegating to the underlying client.
// This allows Session to be passed to components that need LLM access (e.g. Reflector, SelfReview).
func (s *Session) Chat(ctx context.Context, msgs []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	if s.ChatLLM() == nil {
		return nil, fmt.Errorf("session: no LLM client configured")
	}
	return s.ChatLLM().Chat(ctx, msgs, opts)
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
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	CacheReadTokens  int    `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int    `json:"cache_write_tokens,omitempty"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
}
