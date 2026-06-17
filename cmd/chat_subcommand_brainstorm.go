package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// brainstormSubcommand implements the /brainstorm slash command. It
// asks the model to generate ideas on a topic.
type brainstormSubcommand struct{}

func (b *brainstormSubcommand) Name() string        { return "brainstorm" }
func (b *brainstormSubcommand) Aliases() []string   { return nil }
func (b *brainstormSubcommand) Description() string { return "ask the model to brainstorm ideas on a topic" }
func (b *brainstormSubcommand) Usage() string       { return "/brainstorm <topic>" }
func (b *brainstormSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	topic := strings.TrimSpace(strings.TrimPrefix(text, "/brainstorm"))
	if topic == "" {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /brainstorm <topic>"})
		return m, nil
	}
	return m.startPromptCommand("/brainstorm", engine.BrainstormPrompt(engine.BrainstormSetup, topic, ""))
}

func init() {
	subcommandRegistry.Register(&brainstormSubcommand{})
}
