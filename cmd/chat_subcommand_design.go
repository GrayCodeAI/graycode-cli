package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// designSubcommand implements the /design slash command. It
// dispatches to design-specific prompt builders based on the
// sub-action (screenshot, system, component, regress, or default).
type designSubcommand struct{}

func (d *designSubcommand) Name() string      { return "design" }
func (d *designSubcommand) Aliases() []string { return nil }
func (d *designSubcommand) Description() string {
	return "design a feature (screenshot|system|component|regress)"
}

func (d *designSubcommand) Usage() string {
	return "/design [screenshot|system|component|regress] [args...]"
}

func (d *designSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(text)
	if len(fields) >= 2 {
		switch fields[1] {
		case "screenshot":
			path := strings.TrimSpace(strings.TrimPrefix(text, "/design screenshot"))
			if path == "" {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /design screenshot <path/to/screenshot.png>"})
				return m, nil
			}
			return m.startPromptCommand("/design screenshot", buildDesignScreenshotPrompt(path))
		case "system":
			dir := strings.TrimSpace(strings.TrimPrefix(text, "/design system"))
			if dir == "" {
				dir = "."
			}
			return m.startPromptCommand("/design system", buildDesignSystemPrompt(dir))
		case "component":
			rest := strings.TrimSpace(strings.TrimPrefix(text, "/design component"))
			parts2 := strings.Fields(rest)
			name := ""
			fw := ""
			if len(parts2) >= 1 {
				name = parts2[0]
				if len(parts2) >= 2 {
					fw = parts2[1]
				}
			}
			if name == "" {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /design component <name> [framework]"})
				return m, nil
			}
			return m.startPromptCommand("/design component", buildDesignComponentPrompt(name, fw))
		case "regress":
			rest := strings.TrimSpace(strings.TrimPrefix(text, "/design regress"))
			if rest == "" {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /design regress <baseline> [current]"})
				return m, nil
			}
			parts2 := strings.Fields(rest)
			baseline := parts2[0]
			current := "."
			if len(parts2) >= 2 {
				current = parts2[1]
			}
			return m.startPromptCommand("/design regress", buildDesignRegressionPrompt(baseline, current))
		default:
			topic := strings.TrimSpace(strings.TrimPrefix(text, "/design"))
			if topic == "" {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /design <what to build or improve> or /design screenshot|system|component|regress"})
				return m, nil
			}
			return m.startPromptCommand("/design", buildDesignPrompt(topic))
		}
	}
	topic := strings.TrimSpace(strings.TrimPrefix(text, "/design"))
	if topic == "" {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /design <what to build or improve> or /design screenshot|system|component|regress"})
		return m, nil
	}
	return m.startPromptCommand("/design", buildDesignPrompt(topic))
}

func init() {
	subcommandRegistry.Register(&designSubcommand{})
}
