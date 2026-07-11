package cmd

import (
			tea "charm.land/bubbletea/v2"
)

// envSubcommand implements the /env slash command. It prints the
// current provider/model environment summary.
type envSubcommand struct{}

func (e *envSubcommand) Name() string        { return "env" }
func (e *envSubcommand) Aliases() []string   { return nil }
func (e *envSubcommand) Description() string { return "print provider, model, and key configuration" }
func (e *envSubcommand) Usage() string       { return "" }
func (e *envSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: envSummary(m.session.Provider(), m.session.Model())})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&envSubcommand{})
}
