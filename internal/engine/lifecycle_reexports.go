// This file re-exports symbols from the lifecycle sub-package so that existing
// callers of engine.SessionLifecycle, engine.NewLimitTracker, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/lifecycle"

type SessionLifecycle = lifecycle.SessionLifecycle
type EvolvingMemoryInterface = lifecycle.EvolvingMemoryInterface
type SkillStoreInterface = lifecycle.SkillStoreInterface
type CostTrackerInterface = lifecycle.CostTrackerInterface
type CostEntry = lifecycle.CostEntry
type SessionOutcome = lifecycle.SessionOutcome
type EvolvingMemoryAdapter = lifecycle.EvolvingMemoryAdapter
type SkillDistillerAdapter = lifecycle.SkillDistillerAdapter
type SafetyLimits = lifecycle.SafetyLimits
type LimitTracker = lifecycle.LimitTracker
type TimeoutConfig = lifecycle.TimeoutConfig

var ParseAndApplyMemoryOps = lifecycle.ParseAndApplyMemoryOps
var NewLimitTracker = lifecycle.NewLimitTracker
var DefaultLimits = lifecycle.DefaultLimits
var VibeLimits = lifecycle.VibeLimits
var ResearchLimits = lifecycle.ResearchLimits
var DefaultTimeoutConfig = lifecycle.DefaultTimeoutConfig
var WithTimeout = lifecycle.WithTimeout
var RemainingTime = lifecycle.RemainingTime
var TimeoutMessage = lifecycle.TimeoutMessage
