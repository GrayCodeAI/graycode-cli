package cmd

import (
	tea "charm.land/bubbletea/v2"
)

// releaseNotesSubcommand implements the /release-notes slash
// command. It prompts the model to draft release notes from the
// recent changes.
type releaseNotesSubcommand struct{}

func (r *releaseNotesSubcommand) Name() string      { return "release-notes" }
func (r *releaseNotesSubcommand) Aliases() []string { return nil }
func (r *releaseNotesSubcommand) Description() string {
	return "draft release notes from recent changes"
}
func (r *releaseNotesSubcommand) Usage() string { return "" }
func (r *releaseNotesSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.startPromptCommand("/release-notes", "Draft release notes for the changes in this branch. Group by feature/fix/breaking-change and link to relevant issues.")
}

func init() {
	subcommandRegistry.Register(&releaseNotesSubcommand{})
}
