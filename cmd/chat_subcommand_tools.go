package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// toolsSubcommand implements the /tools slash command. It prints
// a list of all enabled tools in the session.
type toolsSubcommand struct{}

func (t *toolsSubcommand) Name() string        { return "tools" }
func (t *toolsSubcommand) Aliases() []string   { return nil }
func (t *toolsSubcommand) Description() string { return "list enabled tools in this session" }
func (t *toolsSubcommand) Usage() string       { return "" }
func (t *toolsSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: toolListSummary(m.registry)})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&toolsSubcommand{})
}
