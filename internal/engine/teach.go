package engine

import "fmt"

// TeachConfig controls explanation depth.
type TeachConfig struct {
	Enabled bool
	Depth   int // 1=what, 2=why, 3=how (default: 2)
}

// TeachPromptAugment returns a system prompt addition that instructs the agent
// to explain its reasoning at the given depth level.
func TeachPromptAugment(depth int) string {
	switch depth {
	case 1:
		return "Before each action, state what you're about to do in one sentence."
	case 3:
		return "Before each action, explain your reasoning process: what you considered, " +
			"why you chose this approach, and what alternatives you rejected."
	default: // depth 2 is the default
		return "Before each action, explain what you're doing and why (2-3 sentences)."
	}
}

// FormatTeachingMoment wraps agent output with teaching context.
func FormatTeachingMoment(action, reasoning string) string {
	if reasoning == "" {
		return action
	}
	return fmt.Sprintf("\U0001f4a1 %s\n\n%s", reasoning, action)
}
