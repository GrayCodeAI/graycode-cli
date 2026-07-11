package cmd

import (
			tea "charm.land/bubbletea/v2"
)

// welcomeSubcommand implements the /welcome slash command. It
// re-prints the inline welcome header at the top of chat.
type welcomeSubcommand struct{}

func (w *welcomeSubcommand) Name() string        { return "welcome" }
func (w *welcomeSubcommand) Aliases() []string   { return nil }
func (w *welcomeSubcommand) Description() string { return "re-print the welcome header" }
func (w *welcomeSubcommand) Usage() string       { return "" }
func (w *welcomeSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "welcome", content: m.welcomeCache})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&welcomeSubcommand{})
}
