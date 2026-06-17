package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// costSubcommand implements the /cost slash command. It prints the
// session's cost summary (token usage, USD cost).
type costSubcommand struct{}

func (c *costSubcommand) Name() string        { return "cost" }
func (c *costSubcommand) Aliases() []string   { return nil }
func (c *costSubcommand) Description() string { return "print session cost and token usage summary" }
func (c *costSubcommand) Usage() string       { return "" }
func (c *costSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: m.session.CostValue().Summary()})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&costSubcommand{})
}
