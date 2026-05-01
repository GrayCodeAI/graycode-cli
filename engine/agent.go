package engine

import (
	"context"
	"strings"
)

// WireAgentTool sets up sub-agent spawning that inherits permissions.
func (s *Session) WireAgentTool() {
	s.AgentSpawnFn = func(ctx context.Context, prompt string) (string, error) {
		sub := NewSession(s.provider, s.model, s.system, s.registry)
		sub.SetAPIKeys(s.apiKeys)
		sub.PermissionFn = s.PermissionFn // inherit parent's permission handler
		sub.Permissions = s.Permissions   // share permission memory
		sub.Mode = s.Mode
		sub.MaxTurns = s.MaxTurns
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
