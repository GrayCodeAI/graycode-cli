// This file re-exports symbols from the agent sub-package so that existing
// callers of engine.SubAgentMode, engine.NewSubAgentBudget, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/agent"

type (
	SubAgentMode        = agent.SubAgentMode
	SubAgentConfig      = agent.SubAgentConfig
	SubAgentBudget      = agent.SubAgentBudget
	BackgroundAgentPool = agent.BackgroundAgentPool
	BackgroundResult    = agent.BackgroundResult
)

const (
	SubAgentExplore     = agent.SubAgentExplore
	SubAgentGeneral     = agent.SubAgentGeneral
	DefaultExploreTurns = agent.DefaultExploreTurns
	DefaultGeneralTurns = agent.DefaultGeneralTurns
	MaxAgentDepth       = agent.MaxAgentDepth
)

var (
	ExploreTools      = agent.ExploreTools
	ModeToolAllowlist = agent.ModeToolAllowlist
)

func DefaultSubAgentConfig() SubAgentConfig { return agent.DefaultSubAgentConfig() }
func NewSubAgentBudget(mode SubAgentMode, cfg SubAgentConfig) *SubAgentBudget {
	return agent.NewSubAgentBudget(mode, cfg)
}

func FilterToolsForMode(mode SubAgentMode, available []string) []string {
	return agent.FilterToolsForMode(mode, available)
}
func NewBackgroundAgentPool() *BackgroundAgentPool    { return agent.NewBackgroundAgentPool() }
func FormatResults(results []BackgroundResult) string { return agent.FormatResults(results) }
