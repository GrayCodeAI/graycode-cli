package cmd

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// reflectSubcommand implements the /reflect slash command. It
// asks the model to reflect on the current session.
type reflectSubcommand struct{}

func (r *reflectSubcommand) Name() string        { return "reflect" }
func (r *reflectSubcommand) Aliases() []string   { return nil }
func (r *reflectSubcommand) Description() string { return "ask the model to reflect on this session" }
func (r *reflectSubcommand) Usage() string       { return "" }
func (r *reflectSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.startPromptCommand("/reflect", engine.ReflectPrompt("this session so far"))
}

func init() {
	subcommandRegistry.Register(&reflectSubcommand{})
}
