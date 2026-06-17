package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// welcomeSubcommand implements the /welcome slash command. It
// re-prints the startup welcome message.
type welcomeSubcommand struct{}

func (w *welcomeSubcommand) Name() string        { return "welcome" }
func (w *welcomeSubcommand) Aliases() []string   { return nil }
func (w *welcomeSubcommand) Description() string { return "re-print the startup welcome message" }
func (w *welcomeSubcommand) Usage() string       { return "" }
func (w *welcomeSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "welcome", content: m.welcomeCache})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&welcomeSubcommand{})
}
