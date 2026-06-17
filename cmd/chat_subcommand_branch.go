package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// branchSubcommand implements the /branch slash command. It shows
// the current git branch, short HEAD hash, upstream tracking branch,
// and a short status output. This is the first command migrated out
// of chat_commands.go as an exemplar of the SubcommandRegistry
// pattern; future commands should follow this same template.
//
// The init() function registers the subcommand in the package-level
// subcommandRegistry. The dispatcher in handleCommand will use the
// registry once the migration is complete; for now the case
// statement in chat_commands.go is the active dispatch path.
type branchSubcommand struct{}

func (b *branchSubcommand) Name() string      { return "branch" }
func (b *branchSubcommand) Aliases() []string { return nil }
func (b *branchSubcommand) Description() string {
	return "show current branch, HEAD, upstream, and status"
}
func (b *branchSubcommand) Usage() string { return "" }
func (b *branchSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: branchSummary()})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&branchSubcommand{})
}
