package engine

import (
	"context"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine/branching"
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/prompts"
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
	// agents-accumulator (.hawk/agents.md).
	agentsAccum *prompts.AgentsAccumulator
	// response cache (used in agentLoop for cache hits).
	responseCache *ResponseCache
	// integration pipeline (pre-query / post-response / end-session).
	pipeline *IntegrationPipeline
	// steering queue.
	steering *SteeringQueue
	// session-level lifecycle hook.
	lifecycle *SessionLifecycle
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
// learning + .hawk/agents.md learnings into the system prompt.
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
		if len(s2.messages) > 0 {
			for _, m := range s2.messages {
				if m.Role == "user" && len(m.ToolResults) == 0 && outcome.TaskGoal == "" {
					outcome.TaskGoal = m.Content
				}
			}
		}
		_ = s.lifecycle.OnSessionEnd(ctx, s2, outcome)
	}
	if s.adaptivePrompt != nil {
		for _, m := range s2.messages {
			if m.Role == "user" && len(m.ToolResults) == 0 {
				s.adaptivePrompt.LearnFromFeedback(m.Content)
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

func (s *LifecycleService) SetCascade(c *branching.CascadeRouter)        { s.cascade = c }
func (s *LifecycleService) SetLifecycle(l *SessionLifecycle)            { s.lifecycle = l }
func (s *LifecycleService) SetReflector(r *Reflector)                  { s.reflector = r }
func (s *LifecycleService) SetCritic(c *Critic)                          { s.critic = c }
func (s *LifecycleService) SetShadow(sh *branching.ShadowWorkspace)     { s.shadow = sh }
func (s *LifecycleService) SetFewShotStore(f *FewShotStore)             { s.fewShotStore = f }
func (s *LifecycleService) SetAdaptivePrompt(a *AdaptivePrompt)         { s.adaptivePrompt = a }
func (s *LifecycleService) SetActivity(act *memory.ActivityTracker)        { s.activity = act }
func (s *LifecycleService) SetAgentsAccum(a *prompts.AgentsAccumulator) { s.agentsAccum = a }
func (s *LifecycleService) SetSteering(st *SteeringQueue)               { s.steering = st }

// Accessors used by stream.go and the agent loop. nil-safe.
func (s *LifecycleService) Beliefs() *BeliefState                        { return s.beliefs }
func (s *LifecycleService) Backtrack() *BacktrackEngine                  { return s.backtrack }
func (s *LifecycleService) Limits() *LimitTracker                        { return s.limits }
func (s *LifecycleService) Critic() *Critic                              { return s.critic }
func (s *LifecycleService) Shadow() *branching.ShadowWorkspace          { return s.shadow }
func (s *LifecycleService) Reflector() *Reflector                        { return s.reflector }
func (s *LifecycleService) FewShotStore() *FewShotStore                  { return s.fewShotStore }
func (s *LifecycleService) AdaptivePrompt() *AdaptivePrompt              { return s.adaptivePrompt }
func (s *LifecycleService) Activity() *memory.ActivityTracker            { return s.activity }
func (s *LifecycleService) AgentsAccum() *prompts.AgentsAccumulator       { return s.agentsAccum }
func (s *LifecycleService) ResponseCache() *ResponseCache                { return s.responseCache }
func (s *LifecycleService) Pipeline() *IntegrationPipeline              { return s.pipeline }
func (s *LifecycleService) Steering() *SteeringQueue                    { return s.steering }
func (s *LifecycleService) Lifecycle() *SessionLifecycle                { return s.lifecycle }
