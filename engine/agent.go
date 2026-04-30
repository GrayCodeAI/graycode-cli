package engine

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/hawk/tool"
)

// WireAgentTool sets up the AgentSpawnFn to create sub-agent sessions.
func (s *Session) WireAgentTool() {
	tool.AgentSpawnFn = func(ctx context.Context, prompt string) (string, error) {
		sub := NewSession(s.provider, s.model, s.system, s.registry)
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
