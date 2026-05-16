// Package lifecycle is the Stage-1 namespace for session lifecycle, limits,
// timeouts, and sleep-time operations. See ../REFACTOR_PLAN.md.
//
// Note: engine.go (the Engine type itself) stays in the root engine package
// as the coordinator — it is NOT re-exported here. This cluster covers the
// supporting lifecycle infrastructure only.
package lifecycle

import "github.com/GrayCodeAI/hawk/engine"

type (
	SessionLifecycle        = engine.SessionLifecycle
	EvolvingMemoryInterface = engine.EvolvingMemoryInterface
	SkillStoreInterface     = engine.SkillStoreInterface
	CostTrackerInterface    = engine.CostTrackerInterface
	CostEntry               = engine.CostEntry
	SessionOutcome          = engine.SessionOutcome
	EvolvingMemoryAdapter   = engine.EvolvingMemoryAdapter
	SkillDistillerAdapter   = engine.SkillDistillerAdapter
	SafetyLimits            = engine.SafetyLimits
	LimitTracker            = engine.LimitTracker
	TimeoutConfig           = engine.TimeoutConfig
)

var (
	NewLimitTracker      = engine.NewLimitTracker
	DefaultLimits        = engine.DefaultLimits
	VibeLimits           = engine.VibeLimits
	ResearchLimits       = engine.ResearchLimits
	DefaultTimeoutConfig = engine.DefaultTimeoutConfig
	WithTimeout          = engine.WithTimeout
	RemainingTime        = engine.RemainingTime
	TimeoutMessage       = engine.TimeoutMessage
)
