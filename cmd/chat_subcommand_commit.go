package cmd

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// commitSubcommand implements the /commit slash command. It shows
// the pending diff stat and then prompts the model to compose a
// commit message and run `git commit`.
type commitSubcommand struct{}

func (c *commitSubcommand) Name() string      { return "commit" }
func (c *commitSubcommand) Aliases() []string { return nil }
func (c *commitSubcommand) Description() string {
	return "review pending changes and create a git commit"
}
func (c *commitSubcommand) Usage() string { return "" }
func (c *commitSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	stat, _ := gitOutput("diff", "--stat")
	if strings.TrimSpace(stat) == "" {
		stat, _ = gitOutput("diff", "--cached", "--stat")
	}
	if strings.TrimSpace(stat) != "" {
		m.messages = append(m.messages, displayMsg{role: "system", content: "Changes to commit:\n" + stat})
	}
	return m.startPromptCommand("/commit", "Review the changes I've made, then create a git commit with an appropriate commit message. Use git add for specific files and git commit.")
}

func init() {
	subcommandRegistry.Register(&commitSubcommand{})
}
