package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// filesSubcommand implements the /files slash command. It prints a
// summary of files in the working directory.
type filesSubcommand struct{}

func (f *filesSubcommand) Name() string      { return "files" }
func (f *filesSubcommand) Aliases() []string { return nil }
func (f *filesSubcommand) Description() string {
	return "print a summary of files in the working directory"
}
func (f *filesSubcommand) Usage() string { return "" }
func (f *filesSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: filesSummary()})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&filesSubcommand{})
}
