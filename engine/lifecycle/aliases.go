// Package lifecycle is the Stage-1 namespace for session lifecycle, limits,
// timeouts, and sleep-time operations. See ../REFACTOR_PLAN.md.
//
// Note: engine.go (the Engine type itself) stays in the root engine package
// as the coordinator — it is NOT re-exported here. This cluster covers the
// supporting lifecycle infrastructure only.
package lifecycle

import "github.com/GrayCodeAI/hawk/engine"

type SessionLifecycle = engine.SessionLifecycle
type EvolvingMemoryInterface = engine.EvolvingMemoryInterface
type SkillStoreInterface = engine.SkillStoreInterface
type CostTrackerInterface = engine.CostTrackerInterface
type CostEntry = engine.CostEntry
type SessionOutcome = engine.SessionOutcome
type EvolvingMemoryAdapter = engine.EvolvingMemoryAdapter
type SkillDistillerAdapter = engine.SkillDistillerAdapter
type SafetyLimits = engine.SafetyLimits
type LimitTracker = engine.LimitTracker
type TimeoutConfig = engine.TimeoutConfig

var NewLimitTracker = engine.NewLimitTracker
var DefaultLimits = engine.DefaultLimits
var VibeLimits = engine.VibeLimits
var ResearchLimits = engine.ResearchLimits
var DefaultTimeoutConfig = engine.DefaultTimeoutConfig
var WithTimeout = engine.WithTimeout
var RemainingTime = engine.RemainingTime
var TimeoutMessage = engine.TimeoutMessage
