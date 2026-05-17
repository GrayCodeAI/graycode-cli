package engine

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/hawk/prompts"
	"github.com/GrayCodeAI/hawk/tool"
)

// SubAgentMode determines the capabilities and cost profile of a sub-agent.
type SubAgentMode string

const (
	SubAgentExplore SubAgentMode = "explore"
	SubAgentGeneral SubAgentMode = "general"
)

// Sub-agent budget defaults per mode.
const (
	DefaultExploreTurns = 15
	DefaultGeneralTurns = 20
	MaxAgentDepth       = 2
)

// ExploreTools are the read-only tools available to explore-mode sub-agents.
var ExploreTools = []string{
	"Glob", "Grep", "Read", "Bash", "LS",
}

// WireAgentTool sets up sub-agent spawning with two modes:
//   - explore: fast/cheap model, read-only tools, higher turn budget
//   - general: full model, all tools, standard budget
func (s *Session) WireAgentTool() {
	s.AgentSpawnFn = func(ctx context.Context, prompt string) (string, error) {
		return s.spawnSubAgent(ctx, prompt, SubAgentExplore, 0)
	}
}

// SpawnSubAgent creates a sub-agent with the given mode and depth tracking.
func (s *Session) spawnSubAgent(ctx context.Context, prompt string, mode SubAgentMode, depth int) (string, error) {
	if depth >= MaxAgentDepth {
		return "", nil
	}

	maxTurns := DefaultExploreTurns
	if mode == SubAgentGeneral {
		maxTurns = DefaultGeneralTurns
	}

	model := s.resolveSubAgentModel(mode)
	registry := s.resolveSubAgentTools(mode)

	subPromptCtx := prompts.PromptContext{
		MaxTurns: maxTurns,
		Task:     prompt,
	}
	subSystemPrompt, err := prompts.BuildSubAgentPrompt(subPromptCtx)
	if err != nil {
		subSystemPrompt = s.system
	}

	sub := NewSession(s.provider, model, subSystemPrompt, registry)
	sub.SetAPIKeys(s.apiKeys)
	sub.PermissionFn = s.PermissionFn
	sub.Permissions = s.Permissions
	sub.Mode = s.Mode
	sub.MaxTurns = maxTurns
	sub.MaxBudgetUSD = s.MaxBudgetUSD
	sub.AddUser(prompt)

	ch, err := sub.Stream(ctx)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for ev := range ch {
		switch ev.Type {
		case "content":
			b.WriteString(ev.Content)
		case "error":
			return b.String(), nil
		}
	}
	return b.String(), nil
}

func (s *Session) resolveSubAgentModel(mode SubAgentMode) string {
	if s.Cascade == nil {
		return s.model
	}
	switch mode {
	case SubAgentExplore:
		return s.Cascade.SelectModel("summarize", s.model, "")
	case SubAgentGeneral:
		return s.Cascade.SelectModel("implement", s.model, "")
	default:
		return s.model
	}
}

func (s *Session) resolveSubAgentTools(mode SubAgentMode) *tool.Registry {
	if mode == SubAgentExplore {
		return s.registry.Filter(ExploreTools)
	}
	return s.registry
}
