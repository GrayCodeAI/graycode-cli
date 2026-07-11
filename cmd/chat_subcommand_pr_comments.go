package cmd

import (
			tea "charm.land/bubbletea/v2"
)

// prCommentsSubcommand implements the /pr-comments slash command.
// It asks the model to review open PR comments or suggest fixes
// for likely review feedback.
type prCommentsSubcommand struct{}

func (p *prCommentsSubcommand) Name() string      { return "pr-comments" }
func (p *prCommentsSubcommand) Aliases() []string { return nil }
func (p *prCommentsSubcommand) Description() string {
	return "review PR comments and suggest responses"
}
func (p *prCommentsSubcommand) Usage() string { return "" }
func (p *prCommentsSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.startPromptCommand("/pr-comments", "Review open PR comments or, if unavailable, inspect the current diff and suggest responses or fixes for likely review comments.")
}

func init() {
	subcommandRegistry.Register(&prCommentsSubcommand{})
}
