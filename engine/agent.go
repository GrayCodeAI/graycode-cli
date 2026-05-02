package engine

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/hawk/prompts"
)

// DefaultSubAgentMaxTurns is the default turn budget for sub-agents.
const DefaultSubAgentMaxTurns = 10

// WireAgentTool sets up sub-agent spawning that inherits permissions.
func (s *Session) WireAgentTool() {
	s.AgentSpawnFn = func(ctx context.Context, prompt string) (string, error) {
		// Build a sub-agent system prompt with budget awareness
		subMaxTurns := DefaultSubAgentMaxTurns
		if s.MaxTurns > 0 && s.MaxTurns < subMaxTurns {
			subMaxTurns = s.MaxTurns
		}
		subPromptCtx := prompts.PromptContext{
			MaxTurns: subMaxTurns,
			Task:     prompt,
		}
		subSystemPrompt, err := prompts.BuildSubAgentPrompt(subPromptCtx)
		if err != nil {
			// Fall back to parent system prompt if template fails
			subSystemPrompt = s.system
		}

		sub := NewSession(s.provider, s.model, subSystemPrompt, s.registry)
		sub.SetAPIKeys(s.apiKeys)
		sub.PermissionFn = s.PermissionFn // inherit parent's permission handler
		sub.Permissions = s.Permissions   // share permission memory
		sub.Mode = s.Mode
		sub.MaxTurns = subMaxTurns
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
}
