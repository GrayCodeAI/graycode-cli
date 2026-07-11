package cmd

import (
	tea "charm.land/bubbletea/v2"
)

// configSubcommand implements the /config (and /con, /conf) slash
// commands. It delegates to m.handleConfigCommand which owns the
// per-argument parsing for config sub-actions.
type configSubcommand struct{}

func (c *configSubcommand) Name() string      { return "config" }
func (c *configSubcommand) Aliases() []string { return []string{"con", "conf"} }
func (c *configSubcommand) Description() string {
	return "show or edit settings (delegates to handleConfigCommand)"
}
func (c *configSubcommand) Usage() string { return "/config [args...]" }
func (c *configSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.handleConfigCommand(args, text)
}

func init() {
	subcommandRegistry.Register(&configSubcommand{})
}
