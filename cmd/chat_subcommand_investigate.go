package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// investigateSubcommand implements the /investigate slash command.
// It prompts the model to reproduce and investigate an issue.
type investigateSubcommand struct{}

func (i *investigateSubcommand) Name() string        { return "investigate" }
func (i *investigateSubcommand) Aliases() []string   { return nil }
func (i *investigateSubcommand) Description() string { return "reproduce and investigate an issue" }
func (i *investigateSubcommand) Usage() string       { return "/investigate [context]" }
func (i *investigateSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	ctx := strings.TrimSpace(strings.TrimPrefix(text, "/investigate"))
	if ctx == "" {
		ctx = "the issue described above"
	}
	return m.startPromptCommand("/investigate", engine.InvestigatePrompt(engine.InvestigateReproduce, ctx))
}

func init() {
	subcommandRegistry.Register(&investigateSubcommand{})
}
