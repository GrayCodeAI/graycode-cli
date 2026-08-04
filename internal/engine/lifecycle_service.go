package engine

import (
	"context"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine/branching"
	"github.com/GrayCodeAI/hawk/internal/engine/token"
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/prompts"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// LifecycleService is the Session's view of the self-improvement and
// observability surface: do-omom-loop detection, snowball detection,
// beliefs, backtrack, limits, critic, shadow, cascade model selection,
// reflect, sleeptime, agent-distill, skill-distill, file-mention
// detection, response caching, steering queue, belief recording, agents
// accumulator, and the few-shot + adaptive-prompt memory. These are
// small but numerous — extracted together in Phase 3 of the
// god-object decomposition (see docs/session-decomposition.md).
//
// All sub-fields are optional. A Session with the defaults
// (LifecycleService{} zero value plus the constructors in New()) is
// fully functional — the agent loop's branching on `if s.X != nil`
// is preserved.
type LifecycleService struct {
	// model selection.
	cascade *branching.CascadeRouter
	// limit tracking.
	limits *LimitTracker
	// doom-loop / snowball / loop detection.
	loopDet  *LoopDetector
	snowball *branching.SnowballDetector
	// beliefs.
	beliefs *BeliefState
	// decision recording.
	backtrack *BacktrackEngine
	// post-write critics.
	critic *Critic
	// pre-edit shadow validation.
	shadow *branching.ShadowWorkspace
	// verbal self-reflection on tool failure.
	reflector *Reflector
	// few-shot + adaptive prompt.
	fewShotStore   *FewShotStore
	adaptivePrompt *AdaptivePrompt
	// activity tracker.
	activity *memory.ActivityTracker
	// agents accumulator.
	agentsAccum *prompts.AgentsAccumulator
	// response cache (used in agentLoop for cache hits).
	responseCache *ResponseCache
	// integration pipeline (pre-query / post-response / end-session).
	pipeline *IntegrationPipeline
	// steering queue.
	steering *SteeringQueue
	// local quality loops run after write tools and belong to lifecycle
	// feedback rather than transport or persistence.
	lintLoop *LintLoop
	testLoop *TestLoop
	// session-level lifecycle hook.
	lifecycle   *SessionLifecycle
	costTracker *CostTracker
	teach       TeachConfig
	trajectory  *TrajectoryDistiller
	// smartSkills caches loaded SmartSkills for auto-discovery per-turn.
	smartSkills []plugin.SmartSkill
	usageMu     sync.Mutex
	usage       *token.UsageTracker
	verbose     bool
	// log is the session logger.
	log *logger.Logger
}

// NewLifecycleService constructs a LifecycleService with all default
// sub-fields populated. log must be non-nil.
func NewLifecycleService(log *logger.Logger) *LifecycleService {
	if log == nil {
		log = logger.Default()
	}
	return &LifecycleService{
		limits:         NewLimitTracker(DefaultLimits()),
		loopDet:        NewLoopDetector(10, DoomLoopThreshold),
		snowball:       branching.NewSnowballDetector(500000),
		beliefs:        NewBeliefState(),
		backtrack:      NewBacktrackEngine(),
		lifecycle:      nil, // constructed in New() with cwd
		responseCache:  NewResponseCache(1000, 24*time.Hour),
		pipeline:       NewIntegrationPipeline(),
		log:            log,
		fewShotStore:   nil, // lazy
		adaptivePrompt: nil, // lazy
	}
}

// OnSessionStart is called by Stream() at the beginning of each session.
// Injects learned guidelines + few-shot examples + user-preference
// learning + accumulated project learnings into the system prompt.
func (s *LifecycleService) OnSessionStart(ctx context.Context, s2 *Session, lastUserMsg string) string {
	if s.lifecycle != nil {
		if ctx := s.lifecycle.OnSessionStart(ctx, lastUserMsg); ctx != "" {
			s2.AppendSystemContext(ctx)
			return ctx
		}
	}
	return ""
}

// OnSessionEnd is called by Stream() when the agent loop exits. Runs
// the post-session pipeline: lifecycle postprocess, enhanced-memory
// EndSession, yaad session summary, few-shot pattern storage,
// adaptive-prompt learning feedback.
func (s *LifecycleService) OnSessionEnd(ctx context.Context, s2 *Session, success bool, duration time.Duration) {
	if s.lifecycle != nil {
		outcome := SessionOutcome{Success: success, Duration: duration}
		messages := s2.Persistence().RawMessages()
		if len(messages) > 0 {
			for _, m := range messages {
				if m.Role == "user" && len(m.ToolResults) == 0 && outcome.TaskGoal == "" {
					outcome.TaskGoal = m.Content
				}
			}
		}
		_ = s.lifecycle.OnSessionEnd(ctx, s2, outcome)
	}
	if s.adaptivePrompt != nil {
		for _, m := range s2.Persistence().RawMessages() {
			if m.Role == "user" && len(m.ToolResults) == 0 {
				s.adaptivePrompt.LearnFromFeedback(m.Content)
			}
		}
	}
}

// StartContext prepares session-start context without requiring a Session
// object. This is the service boundary used by the agent loop.
func (s *LifecycleService) StartContext(ctx context.Context, lastUserMsg string) string {
	if s == nil || s.lifecycle == nil {
		return ""
	}
	return s.lifecycle.OnSessionStart(ctx, lastUserMsg)
}

// Finalize performs lifecycle bookkeeping from immutable session snapshots.
// It intentionally accepts data rather than *Session so the lifecycle layer
// cannot reach through the god object for unrelated state.
func (s *LifecycleService) Finalize(ctx context.Context, messages []types.EyrieMessage, success bool, duration time.Duration, totalCost float64) {
	if s == nil {
		return
	}
	outcome := SessionOutcome{Success: success, Duration: duration, TotalCost: totalCost}
	for _, message := range messages {
		if message.Role == "user" && len(message.ToolResults) == 0 && outcome.TaskGoal == "" {
			outcome.TaskGoal = message.Content
		}
		// Collect tools used and files changed so post-session learning has
		// real signal. Previously these were never populated, so
		// isComplex() was always false and skill distillation never fired
		// in production (H1).
		for _, tc := range message.ToolUse {
			if tc.Name == "" {
				continue
			}
			if !containsStringVec(outcome.ToolsUsed, tc.Name) {
				outcome.ToolsUsed = append(outcome.ToolsUsed, tc.Name)
			}
			cn := canonicalToolName(tc.Name)
			if (cn == "Write" || cn == "Edit") && tc.Arguments != nil {
				if p, ok := pathArgument(tc.Arguments); ok && p != "" && !containsStringVec(outcome.FilesChanged, p) {
					outcome.FilesChanged = append(outcome.FilesChanged, p)
				}
			}
		}
	}
	if s.lifecycle != nil {
		_ = s.lifecycle.OnSessionEnd(ctx, struct{}{}, outcome)
	}
	if s.fewShotStore != nil && success && len(messages) >= 2 {
		response := ""
		for _, message := range messages {
			if message.Role == "assistant" && message.Content != "" {
				response = message.Content
			}
		}
		if outcome.TaskGoal != "" && response != "" {
			s.fewShotStore.Record(outcome.TaskGoal, response, "general")
		}
	}
	if s.adaptivePrompt != nil {
		for _, message := range messages {
			if message.Role == "user" && len(message.ToolResults) == 0 {
				s.adaptivePrompt.LearnFromFeedback(message.Content)
			}
		}
	}
}

// SelectModel picks the optimal model for a turn. Returns the current
// model unchanged if cascade is nil.
func (s *LifecycleService) SelectModel(currentModel, lastUserMsg, hint string) string {
	if s.cascade == nil || !s.cascade.Enabled {
		return currentModel
	}
	return s.cascade.SelectModel(lastUserMsg, currentModel, hint)
}

// CheckLimits returns false if the agent loop should stop (max turns
// hit, max tokens hit, doom loop detected, snowball exceeded).
func (s *LifecycleService) CheckLimits(turnCount int) bool {
	if s.limits != nil {
		s.limits.RecordTurn()
	}
	if s.loopDet != nil && s.loopDet.IsDoomLoop() {
		return false
	}
	if s.snowball != nil && s.snowball.IsSnowballing() {
		return false
	}
	return true
}

// RecordToolCall updates the per-tool call counter used by limits.
func (s *LifecycleService) RecordToolCall(name string) {
	if s.limits != nil {
		s.limits.RecordToolCall(name)
	}
}

// RecordStep updates the doom-loop detector with the latest tool step.
func (s *LifecycleService) RecordStep(toolNames []string, inputs []string, outputs []string) {
	if s.loopDet != nil {
		s.loopDet.RecordStep(toolNames, inputs, outputs)
	}
}

// SnapshotTurnProgress feeds the snowball detector.
func (s *LifecycleService) SnapshotTurnProgress(tokens int, progress float64) {
	if s.snowball != nil {
		s.snowball.RecordTurn(tokens, progress)
	}
}

// Setter accessors used by NewSessionWithClient and the agent loop
// to wire optional collaborators. All nil-safe.

func (s *LifecycleService) SetCascade(c *branching.CascadeRouter)       { s.cascade = c }
func (s *LifecycleService) SetLifecycle(l *SessionLifecycle)            { s.lifecycle = l }
func (s *LifecycleService) SetReflector(r *Reflector)                   { s.reflector = r }
func (s *LifecycleService) SetCritic(c *Critic)                         { s.critic = c }
func (s *LifecycleService) SetShadow(sh *branching.ShadowWorkspace)     { s.shadow = sh }
func (s *LifecycleService) SetFewShotStore(f *FewShotStore)             { s.fewShotStore = f }
func (s *LifecycleService) SetAdaptivePrompt(a *AdaptivePrompt)         { s.adaptivePrompt = a }
func (s *LifecycleService) SetActivity(act *memory.ActivityTracker)     { s.activity = act }
func (s *LifecycleService) SetAgentsAccum(a *prompts.AgentsAccumulator) { s.agentsAccum = a }
func (s *LifecycleService) SetSteering(st *SteeringQueue)               { s.steering = st }
func (s *LifecycleService) SetLintLoop(loop *LintLoop)                  { s.lintLoop = loop }
func (s *LifecycleService) SetTestLoop(loop *TestLoop)                  { s.testLoop = loop }

// Accessors used by stream.go and the agent loop. nil-safe.
func (s *LifecycleService) Beliefs() *BeliefState                   { return s.beliefs }
func (s *LifecycleService) Backtrack() *BacktrackEngine             { return s.backtrack }
func (s *LifecycleService) Limits() *LimitTracker                   { return s.limits }
func (s *LifecycleService) Critic() *Critic                         { return s.critic }
func (s *LifecycleService) Shadow() *branching.ShadowWorkspace      { return s.shadow }
func (s *LifecycleService) Reflector() *Reflector                   { return s.reflector }
func (s *LifecycleService) Cascade() *branching.CascadeRouter       { return s.cascade }
func (s *LifecycleService) FewShotStore() *FewShotStore             { return s.fewShotStore }
func (s *LifecycleService) AdaptivePrompt() *AdaptivePrompt         { return s.adaptivePrompt }
func (s *LifecycleService) Activity() *memory.ActivityTracker       { return s.activity }
func (s *LifecycleService) AgentsAccum() *prompts.AgentsAccumulator { return s.agentsAccum }

// SetAgentsAccumulator attaches the project-learning accumulator.
func (s *LifecycleService) SetAgentsAccumulator(a *prompts.AgentsAccumulator) { s.agentsAccum = a }

func (s *LifecycleService) ResponseCache() *ResponseCache        { return s.responseCache }
func (s *LifecycleService) Pipeline() *IntegrationPipeline       { return s.pipeline }
func (s *LifecycleService) Steering() *SteeringQueue             { return s.steering }
func (s *LifecycleService) Lifecycle() *SessionLifecycle         { return s.lifecycle }
func (s *LifecycleService) CostTracker() *CostTracker            { return s.costTracker }
func (s *LifecycleService) SetCostTracker(c *CostTracker)        { s.costTracker = c }
func (s *LifecycleService) Teach() TeachConfig                   { return s.teach }
func (s *LifecycleService) SetTeach(t TeachConfig)               { s.teach = t }
func (s *LifecycleService) Trajectory() *TrajectoryDistiller     { return s.trajectory }
func (s *LifecycleService) SetTrajectory(t *TrajectoryDistiller) { s.trajectory = t }

// LoadSmartSkills loads the session's auto-discovery skills once.
func (s *LifecycleService) LoadSmartSkills() {
	if s == nil || s.smartSkills != nil {
		return
	}
	s.smartSkills = plugin.LoadSmartSkills(plugin.DefaultSkillDirs())
}

// SmartSkills returns the loaded auto-discovery skills.
func (s *LifecycleService) SmartSkills() []plugin.SmartSkill {
	if s == nil {
		return nil
	}
	return s.smartSkills
}

// EnsureUsageTracker returns the session token-budget tracker, creating it
// with ceilings disabled until the caller opts into local limits.
func (s *LifecycleService) EnsureUsageTracker() *token.UsageTracker {
	if s == nil {
		return nil
	}
	s.usageMu.Lock()
	defer s.usageMu.Unlock()
	if s.usage == nil {
		s.usage = token.NewUsageTracker()
		s.usage.SetLimits(token.UsageLimits{})
	}
	return s.usage
}

// UsageTracker returns the initialized token-budget tracker, if any.
func (s *LifecycleService) UsageTracker() *token.UsageTracker {
	if s == nil {
		return nil
	}
	s.usageMu.Lock()
	defer s.usageMu.Unlock()
	return s.usage
}

// Logger returns the logger shared by lifecycle collaborators.
func (s *LifecycleService) Logger() *logger.Logger {
	if s == nil {
		return nil
	}
	return s.log
}

// SetLogger replaces the logger shared by lifecycle collaborators.
func (s *LifecycleService) SetLogger(l *logger.Logger) {
	if s == nil {
		return
	}
	if l == nil {
		l = logger.Default()
	}
	s.log = l
}

func (s *LifecycleService) ToggleVerbose() bool {
	if s == nil {
		return false
	}
	s.verbose = !s.verbose
	return s.verbose
}
func (s *LifecycleService) Verbose() bool       { return s != nil && s.verbose }
func (s *LifecycleService) LintLoop() *LintLoop { return s.lintLoop }
func (s *LifecycleService) TestLoop() *TestLoop { return s.testLoop }

// containsStringVec reports whether s is present in the slice.
func containsStringVec(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
