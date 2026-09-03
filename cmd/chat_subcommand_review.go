package cmd

import (
	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

// reviewSubcommand implements the /review slash command. It
// prompts the model to review recent changes.
type reviewSubcommand struct{}

func (r *reviewSubcommand) Name() string        { return "review" }
func (r *reviewSubcommand) Aliases() []string   { return nil }
func (r *reviewSubcommand) Description() string { return "ask the model to review changes" }
func (r *reviewSubcommand) Usage() string       { return "" }
func (r *reviewSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.startPromptCommand("/review", engine.ReviewPrompt(nil))
}

func init() {
	subcommandRegistry.Register(&reviewSubcommand{})
}
