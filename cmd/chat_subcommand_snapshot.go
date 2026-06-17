package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// snapshotSubcommand implements the /snapshot slash command.
// It delegates to the existing chatModel.handleSnapshot helper
// in snapshot_cmd.go, which supports `list`, `restore <hash>`,
// and `diff <hash>` sub-commands.
type snapshotSubcommand struct{}

func (s *snapshotSubcommand) Name() string { return "snapshot" }
func (s *snapshotSubcommand) Aliases() []string {
	return nil
}
func (s *snapshotSubcommand) Description() string {
	return "manage file snapshots: list, restore <hash>, diff <hash>"
}
func (s *snapshotSubcommand) Usage() string { return "/snapshot [list|restore <hash>|diff <hash>]" }
func (s *snapshotSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.handleSnapshot(text)
}

func init() {
	subcommandRegistry.Register(&snapshotSubcommand{})
}
