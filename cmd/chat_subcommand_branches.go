package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// branchesSubcommand implements the /branches slash command. It
// lists the conversation DAG's branches. Currently a thin wrapper;
// the full branch viewer UI is non-trivial and lives elsewhere.
type branchesSubcommand struct{}

func (b *branchesSubcommand) Name() string        { return "branches" }
func (b *branchesSubcommand) Aliases() []string   { return nil }
func (b *branchesSubcommand) Description() string { return "list conversation DAG branches" }
func (b *branchesSubcommand) Usage() string       { return "" }
func (b *branchesSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if m.session.Persistence().DAG() == nil {
		m.messages = append(m.messages, displayMsg{role: "system", content: "No conversation branches (DAG not active)."})
		return m, nil
	}
	headID := m.session.ConvoHead()
	if headID == "" {
		m.messages = append(m.messages, displayMsg{role: "system", content: "No conversation history."})
		return m, nil
	}
	m.messages = append(m.messages, displayMsg{role: "system", content: "Current head: " + headID})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&branchesSubcommand{})
}
