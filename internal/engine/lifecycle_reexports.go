// This file re-exports symbols from the lifecycle sub-package so that existing
// callers of engine.SessionLifecycle, engine.NewLimitTracker, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/lifecycle"

type (
	SessionLifecycle        = lifecycle.SessionLifecycle
	EvolvingMemoryInterface = lifecycle.EvolvingMemoryInterface
	SkillStoreInterface     = lifecycle.SkillStoreInterface
	CostTrackerInterface    = lifecycle.CostTrackerInterface
	CostEntry               = lifecycle.CostEntry
	SessionOutcome          = lifecycle.SessionOutcome
	EvolvingMemoryAdapter   = lifecycle.EvolvingMemoryAdapter
	SkillDistillerAdapter   = lifecycle.SkillDistillerAdapter
	SafetyLimits            = lifecycle.SafetyLimits
	LimitTracker            = lifecycle.LimitTracker
	TimeoutConfig           = lifecycle.TimeoutConfig
)

var (
	ParseAndApplyMemoryOps = lifecycle.ParseAndApplyMemoryOps
	NewLimitTracker        = lifecycle.NewLimitTracker
	DefaultLimits          = lifecycle.DefaultLimits
	VibeLimits             = lifecycle.VibeLimits
	ResearchLimits         = lifecycle.ResearchLimits
	DefaultTimeoutConfig   = lifecycle.DefaultTimeoutConfig
	WithTimeout            = lifecycle.WithTimeout
	RemainingTime          = lifecycle.RemainingTime
	TimeoutMessage         = lifecycle.TimeoutMessage
)
