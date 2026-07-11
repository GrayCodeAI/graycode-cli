package cmd

import (
			tea "charm.land/bubbletea/v2"
)

// summarySubcommand implements the /summary slash command. It
// asks the model to summarize the current session.
type summarySubcommand struct{}

func (s *summarySubcommand) Name() string        { return "summary" }
func (s *summarySubcommand) Aliases() []string   { return nil }
func (s *summarySubcommand) Description() string { return "summarize the current session" }
func (s *summarySubcommand) Usage() string       { return "" }
func (s *summarySubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.startPromptCommand("/summary", "Summarize the current session, important decisions, modified files, test status, and remaining work.")
}

func init() {
	subcommandRegistry.Register(&summarySubcommand{})
}
