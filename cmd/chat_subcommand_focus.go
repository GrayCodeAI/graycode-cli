package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// focusSubcommand implements the /focus slash command. It sets a
// system-level FOCUS directive that tells the model to only work
// with the given paths.
type focusSubcommand struct{}

func (f *focusSubcommand) Name() string      { return "focus" }
func (f *focusSubcommand) Aliases() []string { return nil }
func (f *focusSubcommand) Description() string {
	return "restrict agent focus to specific files/directories"
}
func (f *focusSubcommand) Usage() string { return "/focus <path> [path...]" }
func (f *focusSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if len(args) < 1 {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /focus <path> [path...]"})
		return m, nil
	}
	paths := strings.TrimSpace(strings.TrimPrefix(text, "/focus"))
	m.session.AppendSystemContext("FOCUS: Only work with these files/directories: " + paths + ". Ignore files outside this scope unless explicitly asked.")
	m.messages = append(m.messages, displayMsg{role: "system", content: "Focus set: " + paths})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&focusSubcommand{})
}
