// This file re-exports symbols from the agent sub-package so that existing
// callers of engine.SubAgentMode, engine.NewSubAgentBudget, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import (
	"context"

	"github.com/GrayCodeAI/hawk/internal/engine/agent"
)

type (
	SubAgentMode   = agent.SubAgentMode
	SubAgentConfig = agent.SubAgentConfig
	SubAgentBudget = agent.SubAgentBudget
	// Deprecated: use Session.SpawnController() and BackgroundAgentManager
	// (taskruntime-backed) instead. BackgroundAgentPool is retained for
	// compatibility with older callers and tests.
	BackgroundAgentPool = agent.BackgroundAgentPool
	BackgroundResult    = agent.BackgroundResult
)

const (
	SubAgentExplore     = agent.SubAgentExplore
	SubAgentGeneral     = agent.SubAgentGeneral
	SubAgentPlan        = agent.SubAgentPlan
	DefaultExploreTurns = agent.DefaultExploreTurns
	DefaultGeneralTurns = agent.DefaultGeneralTurns
	DefaultPlanTurns    = agent.DefaultPlanTurns
	MaxAgentDepth       = agent.MaxAgentDepth
)

var (
	ExploreTools      = agent.ExploreTools
	PlanTools         = agent.PlanTools
	ModeToolAllowlist = agent.ModeToolAllowlist
)

func DefaultSubAgentConfig() SubAgentConfig { return agent.DefaultSubAgentConfig() }
func NewSubAgentBudget(mode SubAgentMode, cfg SubAgentConfig) *SubAgentBudget {
	return agent.NewSubAgentBudget(mode, cfg)
}

func FilterToolsForMode(mode SubAgentMode, available []string) []string {
	return agent.FilterToolsForMode(mode, available)
}
func DefaultTurnsForMode(mode SubAgentMode) int { return agent.DefaultTurnsForMode(mode) }
func IsReadOnlyMode(mode SubAgentMode) bool     { return agent.IsReadOnlyMode(mode) }

// Deprecated: prefer Session.SpawnController().SpawnBackground for async
// sub-agents. Retained for compatibility.
func NewBackgroundAgentPool() *BackgroundAgentPool { return agent.NewBackgroundAgentPool() }

// Deprecated: prefer Session.SpawnController().SpawnBackground for async
// sub-agents. Retained for compatibility.
func NewBackgroundAgentPoolWithContext(ctx context.Context) *BackgroundAgentPool {
	return agent.NewBackgroundAgentPoolWithContext(ctx)
}

// Deprecated: prefer SpawnController for background result formatting.
func FormatResults(results []BackgroundResult) string { return agent.FormatResults(results) }
