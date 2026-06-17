package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// usageSubcommand implements the /usage slash command. It prints
// the session's token usage / cost summary (alias for /cost).
type usageSubcommand struct{}

func (u *usageSubcommand) Name() string        { return "usage" }
func (u *usageSubcommand) Aliases() []string   { return nil }
func (u *usageSubcommand) Description() string { return "show session token usage (alias for /cost)" }
func (u *usageSubcommand) Usage() string       { return "" }
func (u *usageSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: m.session.Cost.Summary()})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&usageSubcommand{})
}
