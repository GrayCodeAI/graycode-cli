// This file re-exports symbols from the agent sub-package so that existing
// callers of engine.SubAgentMode, engine.NewSubAgentBudget, etc. keep compiling
// during the Stage 2 migration. See docs/plans/engine-refactor-plan.md.
package engine

import (
	"github.com/GrayCodeAI/graycode-cli/internal/engine/agent"
)

type (
	SubAgentMode     = agent.SubAgentMode
	SubAgentConfig   = agent.SubAgentConfig
	SubAgentBudget   = agent.SubAgentBudget
	BackgroundResult = agent.BackgroundResult
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
