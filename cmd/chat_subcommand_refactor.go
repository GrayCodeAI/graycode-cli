package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// refactorSubcommand implements the /refactor slash command. It
// delegates to m.handleRefactorCommand (a chatModel method that
// owns the per-argument parsing for refactor sub-actions).
type refactorSubcommand struct{}

func (r *refactorSubcommand) Name() string      { return "refactor" }
func (r *refactorSubcommand) Aliases() []string { return nil }
func (r *refactorSubcommand) Description() string {
	return "refactor code (delegates to handleRefactorCommand)"
}
func (r *refactorSubcommand) Usage() string { return "/refactor [args...]" }
func (r *refactorSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.handleRefactorCommand(args, text)
}

func init() {
	subcommandRegistry.Register(&refactorSubcommand{})
}
