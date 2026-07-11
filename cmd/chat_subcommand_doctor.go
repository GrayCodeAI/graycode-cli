package cmd

import (
	tea "charm.land/bubbletea/v2"
)

// doctorSubcommand implements the /doctor slash command. It runs
// a diagnostic prompt that asks the model to check the project
// (build, tests, lint) and report issues.
type doctorSubcommand struct{}

func (d *doctorSubcommand) Name() string        { return "doctor" }
func (d *doctorSubcommand) Aliases() []string   { return nil }
func (d *doctorSubcommand) Description() string { return "run diagnostics: build, tests, lint" }
func (d *doctorSubcommand) Usage() string       { return "" }
func (d *doctorSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.startPromptCommand("/doctor", "Run diagnostics on this project: check if it builds, run tests, check for lint errors. Report any issues found.")
}

func init() {
	subcommandRegistry.Register(&doctorSubcommand{})
}
