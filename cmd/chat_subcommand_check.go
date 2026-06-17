package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// checkSubcommand implements the /check slash command. It runs the
// buildCheckPrompt() through the model.
type checkSubcommand struct{}

func (c *checkSubcommand) Name() string        { return "check" }
func (c *checkSubcommand) Aliases() []string   { return nil }
func (c *checkSubcommand) Description() string { return "run a self-check prompt against the project" }
func (c *checkSubcommand) Usage() string       { return "" }
func (c *checkSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.startPromptCommand("/check", buildCheckPrompt())
}

func init() {
	subcommandRegistry.Register(&checkSubcommand{})
}
