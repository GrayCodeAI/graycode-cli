package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// partySubcommand implements the /party slash command. It starts a
// multi-perspective "party" discussion on the given topic.
type partySubcommand struct{}

func (p *partySubcommand) Name() string        { return "party" }
func (p *partySubcommand) Aliases() []string   { return nil }
func (p *partySubcommand) Description() string { return "multi-perspective discussion on a topic" }
func (p *partySubcommand) Usage() string       { return "/party <topic>" }
func (p *partySubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	topic := strings.TrimSpace(strings.TrimPrefix(text, "/party"))
	if topic == "" {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /party <topic to discuss>"})
		return m, nil
	}
	ps := engine.NewPartySession(topic, nil)
	return m.startPromptCommand("/party", ps.GeneratePrompt(1))
}

func init() {
	subcommandRegistry.Register(&partySubcommand{})
}
