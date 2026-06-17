package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// specSubcommand implements the /spec slash command. It prompts the
// model to generate a spec for a feature or system described by
// the user.
type specSubcommand struct{}

func (s *specSubcommand) Name() string        { return "spec" }
func (s *specSubcommand) Aliases() []string   { return nil }
func (s *specSubcommand) Description() string { return "generate a spec from a description" }
func (s *specSubcommand) Usage() string       { return "/spec <what to build>" }
func (s *specSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/spec"))
	if arg == "" {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /spec <what to build>"})
		return m, nil
	}
	return m.startPromptCommand("/spec", engine.SpecGeneratePrompt(arg))
}

func init() {
	subcommandRegistry.Register(&specSubcommand{})
}
