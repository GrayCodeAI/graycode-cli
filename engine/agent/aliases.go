// Package agent is the Stage-1 namespace for sub-agent orchestration types.
// See ../REFACTOR_PLAN.md.
package agent

import "github.com/GrayCodeAI/hawk/engine"

type SubAgentMode = engine.SubAgentMode
type SubAgentConfig = engine.SubAgentConfig
type SubAgentBudget = engine.SubAgentBudget
type BackgroundAgentPool = engine.BackgroundAgentPool
type BackgroundResult = engine.BackgroundResult

func DefaultSubAgentConfig() SubAgentConfig { return engine.DefaultSubAgentConfig() }
func NewSubAgentBudget(mode SubAgentMode, cfg SubAgentConfig) *SubAgentBudget {
	return engine.NewSubAgentBudget(mode, cfg)
}
func FilterToolsForMode(mode SubAgentMode, available []string) []string {
	return engine.FilterToolsForMode(mode, available)
}
func NewBackgroundAgentPool() *BackgroundAgentPool { return engine.NewBackgroundAgentPool() }
func FormatResults(results []BackgroundResult) string { return engine.FormatResults(results) }
