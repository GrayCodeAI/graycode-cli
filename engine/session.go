package engine

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/client"

	"github.com/GrayCodeAI/hawk/logger"
	"github.com/GrayCodeAI/hawk/memory"
	"github.com/GrayCodeAI/hawk/metrics"
	modelPkg "github.com/GrayCodeAI/hawk/routing"
	"github.com/GrayCodeAI/hawk/permissions"
	"github.com/GrayCodeAI/hawk/tool"
)

// MemoryRecaller abstracts memory recall/remember so engine avoids importing memory directly.
type MemoryRecaller interface {
	Recall(query string, tokenBudget int) (string, error)
	Remember(content, category string) error
}

// Session manages a conversation with an LLM via eyrie.
type Session struct {
	client       *client.EyrieClient
	registry     *tool.Registry
	messages     []client.EyrieMessage
	provider     string
	model        string
	apiKeys      map[string]string
	system       string
	log          *logger.Logger
	metrics      *metrics.Registry
	Cost         Cost
	Router       *modelPkg.Router
	Perm         *PermissionEngine // extracted permission subsystem
	// Backward-compatible accessors below (will be removed after full migration)
	Permissions  *PermissionMemory            // use Perm.Memory
	AutoMode     *permissions.AutoModeState   // use Perm.AutoMode
	Classifier   *permissions.Classifier      // use Perm.Classifier
	BypassKill   *permissions.BypassKillswitch // use Perm.BypassKill
	Mode         PermissionMode               // use Perm.Mode
	MaxTurns     int
	MaxBudgetUSD float64
	AllowedDirs  []string
	PermissionFn func(PermissionRequest)      // use Perm.PromptFn
	AgentSpawnFn func(ctx context.Context, prompt string) (string, error)
	AskUserFn    func(question string) (string, error)
	Memory       MemoryRecaller
	YaadBridge   *memory.YaadBridge

	PinnedMessages int // messages to protect from compaction (from /pin)
	AutoCompactThresholdPct int // token % to trigger auto-compact (default 85)

	// Cost optimization
	Cascade      *CascadeRouter       // cascade.go — model tier routing
	Lifecycle    *SessionLifecycle    // lifecycle.go — self-improvement loop
	Reflector    *Reflector           // reflect.go — verbal self-reflection
	CostTracker  *CostTracker         // cost_tracker.go — per-request cost persistence

	// Advanced features
	Autonomy   AutonomyLevel        // autonomy.go — permission level
	Sandbox    *DiffSandbox         // diffsandbox.go — staged file changes
	Plan       *PlanState           // subtask.go — user-activated plan
	Beliefs    *BeliefState         // belief.go — discovered knowledge
	Critic     *Critic              // critic.go — patch pre-screening
	Backtrack  *BacktrackEngine     // backtrack.go — decision recording
	Limits     *LimitTracker        // limits.go — safety limits
	Teach      TeachConfig          // teach.go — explanation depth
	Trajectory *TrajectoryDistiller // trajectory.go — multi-run distillation
	Shadow     *ShadowWorkspace     // shadow.go — edit pre-validation
	Sleeptime      *memory.SleeptimeAgent   // sleeptime.go — background memory consolidation
	Activity       *memory.ActivityTracker  // activity.go — memory save nudging (Engram pattern)
	SkillDistiller *memory.SkillDistiller   // skill_distill.go — auto-skill extraction
}

// NewSession creates a new conversation session.
func NewSession(provider, model, systemPrompt string, registry *tool.Registry) *Session {
	pe := NewPermissionEngine()
	s := &Session{
		client:      client.NewEyrieClient(&client.EyrieConfig{Provider: provider}),
		registry:    registry,
		provider:    provider,
		model:       model,
		apiKeys:     map[string]string{},
		system:      systemPrompt,
		log:         logger.Default(),
		metrics:     metrics.NewRegistry(),
		Perm:        pe,
		Permissions: pe.Memory,
		AutoMode:    pe.AutoMode,
		Classifier:  pe.Classifier,
		BypassKill:  pe.BypassKill,
		Beliefs:     NewBeliefState(),
		Backtrack:   NewBacktrackEngine(),
		Limits:      NewLimitTracker(DefaultLimits()),
	}
	s.Cost.Model = model
	s.Router = modelPkg.NewRouter(modelPkg.StrategyBalanced)
	return s
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
	s.client = client.NewEyrieClient(&client.EyrieConfig{Provider: p})
	for provider, apiKey := range s.apiKeys {
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
	s.messages = append(s.messages, client.EyrieMessage{Role: "user", Content: content})
	// Persist explicit "remember" requests via yaad
	if s.Memory != nil && strings.Contains(strings.ToLower(content), "remember") {
		go s.Memory.Remember(content, "user_explicit")
	}
}

func (s *Session) AddAssistant(content string) {
	s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: content})
}

// AppendSystemContext adds runtime context, such as /add-dir, to future model calls.
func (s *Session) AppendSystemContext(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	if strings.TrimSpace(s.system) == "" {
		s.system = content
		return
	}
	s.system += "\n\n" + content
}

// SetLogger replaces the session logger.
func (s *Session) SetLogger(l *logger.Logger) {
	s.log = l
}

// SetAllowedDirs sets directories that file tools are allowed to access.
func (s *Session) SetAllowedDirs(dirs []string) {
	s.AllowedDirs = append([]string(nil), dirs...)
}

func (s *Session) LoadMessages(msgs []client.EyrieMessage) {
	s.messages = msgs
}

func (s *Session) MessageCount() int { return len(s.messages) }

// RawMessages returns the conversation messages for persistence.
func (s *Session) RawMessages() []client.EyrieMessage { return s.messages }

// RemoveLastExchange removes the last user+assistant message pair.
func (s *Session) RemoveLastExchange() {
	if len(s.messages) < 2 {
		return
	}
	// Remove from the end until we've removed one user and one assistant message
	removed := 0
	for i := len(s.messages) - 1; i >= 0 && removed < 2; i-- {
		role := s.messages[i].Role
		if role == "user" || role == "assistant" {
			removed++
		}
		s.messages = s.messages[:i]
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
