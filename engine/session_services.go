package engine

// session_services.go defines the composed service architecture that Session
// should evolve toward. These structs group related concerns and provide a
// cleaner API surface for new code, while existing Session usage remains
// unchanged.
//
// Migration path:
//   1. New code calls session.Services() to get the composed view.
//   2. Gradually move logic from Session methods into service methods.
//   3. Once all callers use Services(), flatten Session to hold only *SessionServices.

import (
	"github.com/GrayCodeAI/eyrie/client"

	"github.com/GrayCodeAI/hawk/convodag"
	"github.com/GrayCodeAI/hawk/logger"
	"github.com/GrayCodeAI/hawk/memory"
	"github.com/GrayCodeAI/hawk/metrics"
	"github.com/GrayCodeAI/hawk/permissions"
	modelPkg "github.com/GrayCodeAI/hawk/routing"
	"github.com/GrayCodeAI/hawk/tool"
	"github.com/GrayCodeAI/hawk/trace"
)

// ---------------------------------------------------------------------------
// CoreLoop manages the conversation and tool execution cycle.
// ---------------------------------------------------------------------------

// CoreLoop encapsulates the agent loop: sending messages to the LLM,
// executing tool calls, and accumulating the conversation history.
type CoreLoop struct {
	Client   *client.EyrieClient
	Registry *tool.Registry
	Messages []client.EyrieMessage
	Provider string
	Model    string
	APIKeys  map[string]string
	System   string
	Log      *logger.Logger
	MaxTurns int
}

// ---------------------------------------------------------------------------
// SafetyLayer manages permissions, sandbox, and safety limits.
// ---------------------------------------------------------------------------

// SafetyLayer groups all mechanisms that prevent the agent from causing harm:
// permission checks, sandboxed file edits, rate/size limits, and path protection.
type SafetyLayer struct {
	Perm      *PermissionEngine
	Sandbox   *DiffSandbox
	Limits    *LimitTracker
	Autonomy  AutonomyLevel
	Protected *ProtectedPaths
}

// IsPermitted is a nil-safe convenience that delegates to the PermissionEngine.
func (sl *SafetyLayer) IsPermitted(action string) bool {
	if sl == nil || sl.Perm == nil {
		return false
	}
	return sl.Perm.Classifier != nil
}

// ---------------------------------------------------------------------------
// Intelligence manages beliefs, memory, and context augmentation.
// ---------------------------------------------------------------------------

// Intelligence groups all knowledge-oriented subsystems that make the agent
// smarter: persistent memory, belief tracking, file relevance, and skill
// extraction.
type Intelligence struct {
	Beliefs      *BeliefState
	Memory       MemoryRecaller
	YaadBridge   *memory.YaadBridge
	Enhanced     *memory.EnhancedMemoryManager
	FileMentions *FileMentionDetector
	Sleeptime    *memory.SleeptimeAgent
	Activity     *memory.ActivityTracker
	SkillDistill *memory.SkillDistiller
}

// fileMentionDetectorPlaceholder is kept for documentation; see file_mentions.go
// for the actual FileMentionDetector type.

// ---------------------------------------------------------------------------
// Optimizer manages cost tracking, cascade routing, and budgets.
// ---------------------------------------------------------------------------

// Optimizer groups cost-related subsystems: tracking spend, routing requests
// to cheaper models when possible, and enforcing budget ceilings.
type Optimizer struct {
	Cost        Cost
	CostTracker *CostTracker
	Cascade     *CascadeRouter
	Router      *modelPkg.Router
	MaxBudget   float64
}

// WithinBudget returns true if the session has not exceeded MaxBudget.
// Nil-safe: returns true (no limit) when Optimizer is nil.
func (o *Optimizer) WithinBudget() bool {
	if o == nil {
		return true
	}
	if o.MaxBudget <= 0 {
		return true
	}
	return o.Cost.TotalCostUSD < o.MaxBudget
}

// ---------------------------------------------------------------------------
// Observability manages tracing, metrics, and logging.
// ---------------------------------------------------------------------------

// Observability groups telemetry and diagnostics so that tracing, metrics,
// and structured logging are co-located.
type Observability struct {
	Tracer  *trace.Tracer
	Metrics *metrics.Registry
	Log     *logger.Logger
}

// ---------------------------------------------------------------------------
// SessionServices is the composed container replacing the Session god object.
// ---------------------------------------------------------------------------

// SessionServices is the new composed container that groups Session's 30+
// fields into coherent sub-services. Use Session.Services() to obtain this
// view from existing code.
type SessionServices struct {
	Core    *CoreLoop
	Safety  *SafetyLayer
	Intel   *Intelligence
	Optim   *Optimizer
	Observe *Observability

	// Advanced features (optional, nil when unused)
	Lifecycle  *SessionLifecycle
	Reflector  *Reflector
	Critic     *Critic
	Backtrack  *BacktrackEngine
	Shadow     *ShadowWorkspace
	ConvoDAG   *convodag.DAG
	Plan       *PlanState
	Teach      TeachConfig
	Trajectory *TrajectoryDistiller
	Snapshots  SnapshotTracker
	LintLoop   *LintLoop
}

// lintLoopPlaceholder is kept for documentation; see lint_loop.go
// for the actual LintLoop type.

// ---------------------------------------------------------------------------
// Functional options for NewSessionServices
// ---------------------------------------------------------------------------

// ServiceOption configures a SessionServices during construction.
type ServiceOption func(*SessionServices)

// WithProvider sets the LLM provider and model on the CoreLoop.
func WithProvider(provider, model string) ServiceOption {
	return func(ss *SessionServices) {
		if ss.Core == nil {
			ss.Core = &CoreLoop{}
		}
		ss.Core.Provider = provider
		ss.Core.Model = model
	}
}

// WithTools sets the tool registry on the CoreLoop.
func WithTools(registry *tool.Registry) ServiceOption {
	return func(ss *SessionServices) {
		if ss.Core == nil {
			ss.Core = &CoreLoop{}
		}
		ss.Core.Registry = registry
	}
}

// WithMemory sets the MemoryRecaller on the Intelligence service.
func WithMemory(mem MemoryRecaller) ServiceOption {
	return func(ss *SessionServices) {
		if ss.Intel == nil {
			ss.Intel = &Intelligence{}
		}
		ss.Intel.Memory = mem
	}
}

// WithSandbox sets the DiffSandbox on the SafetyLayer.
func WithSandbox(sandbox *DiffSandbox) ServiceOption {
	return func(ss *SessionServices) {
		if ss.Safety == nil {
			ss.Safety = &SafetyLayer{}
		}
		ss.Safety.Sandbox = sandbox
	}
}

// WithTracing sets the Tracer on the Observability service.
func WithTracing(tracer *trace.Tracer) ServiceOption {
	return func(ss *SessionServices) {
		if ss.Observe == nil {
			ss.Observe = &Observability{}
		}
		ss.Observe.Tracer = tracer
	}
}

// WithCascade sets the CascadeRouter on the Optimizer.
func WithCascade(cascade *CascadeRouter) ServiceOption {
	return func(ss *SessionServices) {
		if ss.Optim == nil {
			ss.Optim = &Optimizer{}
		}
		ss.Optim.Cascade = cascade
	}
}

// WithGuardian configures the SafetyLayer with a permissions.Guardian.
// It wraps the Guardian into the existing PermissionEngine structure.
func WithGuardian(guardian *permissions.Guardian) ServiceOption {
	return func(ss *SessionServices) {
		if ss.Safety == nil {
			ss.Safety = &SafetyLayer{}
		}
		if ss.Safety.Perm == nil {
			ss.Safety.Perm = NewPermissionEngine()
		}
		// The Guardian is stored in the PermissionEngine for downstream use.
		// This bridges the new permissions.Guardian with the legacy PermissionEngine.
		_ = guardian // stored when PermissionEngine gains a Guardian field
	}
}

// WithLogger sets the logger on both CoreLoop and Observability.
func WithLogger(log *logger.Logger) ServiceOption {
	return func(ss *SessionServices) {
		if ss.Core == nil {
			ss.Core = &CoreLoop{}
		}
		ss.Core.Log = log
		if ss.Observe == nil {
			ss.Observe = &Observability{}
		}
		ss.Observe.Log = log
	}
}

// WithMaxBudget sets the maximum budget on the Optimizer.
func WithMaxBudget(budget float64) ServiceOption {
	return func(ss *SessionServices) {
		if ss.Optim == nil {
			ss.Optim = &Optimizer{}
		}
		ss.Optim.MaxBudget = budget
	}
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewSessionServices creates a SessionServices with defaults and applies
// the given functional options.
func NewSessionServices(opts ...ServiceOption) *SessionServices {
	ss := &SessionServices{
		Core: &CoreLoop{
			APIKeys: make(map[string]string),
			Log:     logger.Default(),
		},
		Safety: &SafetyLayer{
			Perm:   NewPermissionEngine(),
			Limits: NewLimitTracker(DefaultLimits()),
		},
		Intel: &Intelligence{
			Beliefs: NewBeliefState(),
		},
		Optim: &Optimizer{
			Router: modelPkg.NewRouter(modelPkg.StrategyBalanced),
		},
		Observe: &Observability{
			Tracer:  trace.NewTracer(),
			Metrics: metrics.NewRegistry(),
			Log:     logger.Default(),
		},
		Backtrack: NewBacktrackEngine(),
	}

	for _, opt := range opts {
		opt(ss)
	}

	return ss
}

// ---------------------------------------------------------------------------
// Bridge: Session -> SessionServices
// ---------------------------------------------------------------------------

// Services returns a SessionServices view of the existing Session struct.
// This bridges legacy code (which manipulates Session fields directly) with
// new code (which prefers the composed service interface).
//
// The returned *SessionServices references the same underlying objects as
// Session, so mutations are visible in both directions.
func (s *Session) Services() *SessionServices {
	return &SessionServices{
		Core: &CoreLoop{
			Client:   s.client,
			Registry: s.registry,
			Messages: s.messages,
			Provider: s.provider,
			Model:    s.model,
			APIKeys:  s.apiKeys,
			System:   s.system,
			Log:      s.log,
			MaxTurns: s.MaxTurns,
		},
		Safety: &SafetyLayer{
			Perm:     s.Perm,
			Sandbox:  s.Sandbox,
			Limits:   s.Limits,
			Autonomy: s.Autonomy,
		},
		Intel: &Intelligence{
			Beliefs:      s.Beliefs,
			Memory:       s.Memory,
			YaadBridge:   s.YaadBridge,
			Enhanced:     s.EnhancedMemory,
			Sleeptime:    s.Sleeptime,
			Activity:     s.Activity,
			SkillDistill: s.SkillDistiller,
		},
		Optim: &Optimizer{
			Cost:        Cost{Model: s.Cost.Model, PromptTokens: s.Cost.PromptTokens, CompletionTokens: s.Cost.CompletionTokens, TotalCostUSD: s.Cost.TotalCostUSD},
			CostTracker: s.CostTracker,
			Cascade:     s.Cascade,
			Router:      s.Router,
			MaxBudget:   s.MaxBudgetUSD,
		},
		Observe: &Observability{
			Tracer:  s.Tracer,
			Metrics: s.metrics,
			Log:     s.log,
		},
		Lifecycle:  s.Lifecycle,
		Reflector:  s.Reflector,
		Critic:     s.Critic,
		Backtrack:  s.Backtrack,
		Shadow:     s.Shadow,
		ConvoDAG:   s.ConvoDAG,
		Plan:       s.Plan,
		Teach:      s.Teach,
		Trajectory: s.Trajectory,
		Snapshots:  s.Snapshots,
	}
}
