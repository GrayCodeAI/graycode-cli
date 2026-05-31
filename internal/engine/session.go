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
	"github.com/GrayCodeAI/hawk/internal/prompts"
	modelPkg "github.com/GrayCodeAI/hawk/internal/provider/routing"
	"github.com/GrayCodeAI/hawk/internal/resilience/ratelimit"
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
}

// Session manages a conversation with an LLM via eyrie.
// The mu RWMutex protects messages and system for concurrent access
// (e.g. daemon handling concurrent requests, background memory goroutines).
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
	DeploymentRouting bool

	// ContainerExecutor runs Bash in an isolated container when set (no API keys in container env).
	ContainerExecutor tool.ContainerExecutor

	Perm *PermissionEngine // extracted permission subsystem
	// Backward-compatible accessors below (will be removed after full migration)
	Permissions    *PermissionMemory             // use Perm.Memory
	AutoMode       *permissions.AutoModeState    // use Perm.AutoMode
	Classifier     *permissions.Classifier       // use Perm.Classifier
	BypassKill     *permissions.BypassKillswitch // use Perm.BypassKill
	Mode           PermissionMode                // use Perm.Mode
	MaxTurns       int
	MaxBudgetUSD   float64
	AllowedDirs    []string
	PermissionFn   func(PermissionRequest) // use Perm.PromptFn
	AgentSpawnFn   func(ctx context.Context, prompt string) (string, error)
	AskUserFn      func(question string) (string, error)
	Memory         MemoryRecaller
	YaadBridge     *memory.YaadBridge
	EnhancedMemory *memory.EnhancedMemoryManager
	SettingsGet    func(key string) (string, bool)
	SettingsSet    func(key, value string) error

	PinnedMessages          int // messages to protect from compaction (from /pin)
	AutoCompactThresholdPct int // token % to trigger auto-compact (default 85)
	Verbose                 bool // show tool calls, timing, token counts in output

	// Cost optimization
	Cascade     *branching.CascadeRouter // cascade.go — model tier routing
	Lifecycle   *SessionLifecycle        // lifecycle.go — self-improvement loop
	Reflector   *Reflector               // reflect.go — verbal self-reflection
	CostTracker *CostTracker             // cost_tracker.go — per-request cost persistence

	// Advanced features
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
	FewShotStore   *FewShotStore   // scaffold/fewshot.go — successful pattern collection
	AdaptivePrompt *AdaptivePrompt // adaptive_prompt.go — user preference learning
}

// NewSession creates a new conversation session with a legacy string-named provider.
func NewSession(provider, model, systemPrompt string, registry *tool.Registry) *Session {
	return NewSessionWithClient(types.NewClient(&types.EyrieConfig{Provider: provider}), provider, model, systemPrompt, registry, false)
}

// NewSessionWithClient constructs a session with an explicit LLM client (e.g. deployment router).
func NewSessionWithClient(chat ChatClient, provider, model, systemPrompt string, registry *tool.Registry, deploymentRouting bool) *Session {
	pe := NewPermissionEngine()
	s := &Session{
		client:            chat,
		registry:          registry,
		provider:          provider,
		model:             model,
		apiKeys:           map[string]string{},
		system:            systemPrompt,
		log:               logger.Default(),
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

	// Initialize agents accumulator for .hawk/agents.md
	cwd, _ := os.Getwd()
	s.AgentsAccum = prompts.NewAgentsAccumulator(cwd)

	return s
}

// ReattachTransport swaps the LLM client after deployment routing or provider.json changes.
func (s *Session) ReattachTransport(chat ChatClient, provider string, deploymentRouting bool) {
	if chat == nil {
		return
	}
	s.client = chat
	if strings.TrimSpace(provider) != "" {
		s.provider = strings.TrimSpace(provider)
	}
	s.DeploymentRouting = deploymentRouting
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

// SetModel updates the active model for subsequent requests.
func (s *Session) SetModel(model string) {
	s.model = strings.TrimSpace(model)
	s.Cost.Model = s.model
}

// SetProvider updates the active provider for subsequent requests.
func (s *Session) SetProvider(provider string) {
	p := strings.TrimSpace(provider)
	s.provider = p
	if s.DeploymentRouting {
		return
	}
	s.client = types.NewClient(&types.EyrieConfig{Provider: p})
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
	s.mu.Lock()
	s.messages = append(s.messages, types.EyrieMessage{Role: "user", Content: content})
	s.mu.Unlock()
	if s.ConvoDAG != nil {
		parentID := ""
		if head, err := s.ConvoDAG.Head(context.Background()); err == nil && head != nil {
			parentID = head.ID
		}
		_, _ = s.ConvoDAG.Append(context.Background(), parentID, "user", content)
	}
	if s.Memory != nil && strings.Contains(strings.ToLower(content), "remember") {
		go func(c string) {
			// Use timeout context so goroutine doesn't hang if backend is slow.
			rCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = rCtx // timeout context available if Remember is extended to accept it
			if err := s.Memory.Remember(c, "user_explicit"); err != nil {
				slog.Warn("background memory remember failed", "error", err)
			}
		}(content)
	}
}

// AddUserWithImage adds a user message with an attached image (base64-encoded).
// The imageType should be "image/png", "image/jpeg", etc.
func (s *Session) AddUserWithImage(content string, imageBase64 string, imageType string) {
	s.mu.Lock()
	msg := types.EyrieMessage{
		Role:    "user",
		Content: content,
		Images:  []string{"data:" + imageType + ";base64," + imageBase64},
	}
	s.messages = append(s.messages, msg)
	s.mu.Unlock()
	if s.ConvoDAG != nil {
		parentID := ""
		if head, err := s.ConvoDAG.Head(context.Background()); err == nil && head != nil {
			parentID = head.ID
		}
		_, _ = s.ConvoDAG.Append(context.Background(), parentID, "user", content+" [image attached]")
	}
}

func (s *Session) AddAssistant(content string) {
	s.mu.Lock()
	s.messages = append(s.messages, types.EyrieMessage{Role: "assistant", Content: content})
	s.mu.Unlock()
	if s.ConvoDAG != nil {
		parentID := ""
		if head, err := s.ConvoDAG.Head(context.Background()); err == nil && head != nil {
			parentID = head.ID
		}
		_, _ = s.ConvoDAG.Append(context.Background(), parentID, "assistant", content)
	}
}

// ForkConversation creates a new branch from a specific point in history.
// Returns the fork node ID and the messages up to that point.
func (s *Session) ForkConversation(nodeID string) (string, error) {
	if s.ConvoDAG == nil {
		return "", nil
	}
	fork, err := s.ConvoDAG.Fork(context.Background(), nodeID)
	if err != nil {
		return "", err
	}
	// Rebuild messages from the forked branch
	history, err := s.ConvoDAG.History(context.Background(), fork.ID)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.messages = s.messages[:0]
	for _, node := range history {
		if node.Role == "user" || node.Role == "assistant" {
			s.messages = append(s.messages, types.EyrieMessage{Role: node.Role, Content: node.Content})
		}
	}
	s.mu.Unlock()
	return fork.ID, nil
}

// SwitchBranch navigates to a different branch point and rebuilds messages.
func (s *Session) SwitchBranch(nodeID string) error {
	if s.ConvoDAG == nil {
		return nil
	}
	if err := s.ConvoDAG.SetHead(context.Background(), nodeID); err != nil {
		return err
	}
	history, err := s.ConvoDAG.History(context.Background(), nodeID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.messages = s.messages[:0]
	for _, node := range history {
		if node.Role == "user" || node.Role == "assistant" {
			s.messages = append(s.messages, types.EyrieMessage{Role: node.Role, Content: node.Content})
		}
	}
	s.mu.Unlock()
	return nil
}

// ListBranches returns child nodes (alternative branches) from a given node.
func (s *Session) ListBranches(nodeID string) ([]*storage.DAGNode, error) {
	if s.ConvoDAG == nil {
		return nil, nil
	}
	return s.ConvoDAG.Branches(context.Background(), nodeID)
}

// ConvoHead returns the current conversation head node ID.
func (s *Session) ConvoHead() string {
	if s.ConvoDAG == nil {
		return ""
	}
	if head, err := s.ConvoDAG.Head(context.Background()); err == nil && head != nil {
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
	defer s.mu.Unlock()
	if strings.TrimSpace(s.system) == "" {
		s.system = content
		return
	}
	s.system += "\n\n" + content
}

// ReplaceSystemContextSection replaces the content of a system prompt section identified by its header.
// If the header is not found, appends the content as a new section.
func (s *Session) ReplaceSystemContextSection(header, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := strings.Index(s.system, header)
	if idx < 0 {
		// AppendSystemContext is not called here to avoid double-locking;
		// replicate its logic inline.
		if strings.TrimSpace(s.system) == "" {
			s.system = content
		} else {
			s.system += "\n\n" + content
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
}

// SetLogger replaces the session logger.
func (s *Session) SetLogger(l *logger.Logger) {
	s.log = l
}

// SetAllowedDirs sets directories that file tools are allowed to access.
func (s *Session) SetAllowedDirs(dirs []string) {
	s.AllowedDirs = append([]string(nil), dirs...)
}

func (s *Session) LoadMessages(msgs []types.EyrieMessage) {
	s.mu.Lock()
	s.messages = msgs
	s.mu.Unlock()
}

func (s *Session) MessageCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages)
}

// RawMessages returns the conversation messages for persistence.
func (s *Session) RawMessages() []types.EyrieMessage {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) < 2 {
		return
	}
	// Remove from the end until we've removed one user and one assistant message
	removed := 0
	for i := len(s.messages) - 1; i >= 0 && removed < 2; i-- {
		role := s.messages[i].Role
		if role == "user" || role == "assistant" {
			removed++
			s.messages = s.messages[:i]
		}
	}
}

// StreamEvent is sent from the engine to the TUI.
type StreamEvent struct {
	Type     string // content, thinking, tool_use, tool_result, usage, done, error
	Content  string
	ToolName string
	ToolID   string
	Usage    *StreamUsage // usage data for this event
}

// StreamUsage tracks token usage for a single stream event.
type StreamUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}
