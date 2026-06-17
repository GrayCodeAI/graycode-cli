package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// huntSubcommand implements the /hunt slash command. It asks the
// model to find a likely cause of a given error or symptom.
type huntSubcommand struct{}

func (h *huntSubcommand) Name() string        { return "hunt" }
func (h *huntSubcommand) Aliases() []string   { return nil }
func (h *huntSubcommand) Description() string { return "hunt for a cause given a symptom or error" }
func (h *huntSubcommand) Usage() string       { return "/hunt <error or symptom>" }
func (h *huntSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	symptom := strings.TrimSpace(strings.TrimPrefix(text, "/hunt"))
	if symptom == "" {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /hunt <error or symptom>"})
		return m, nil
	}
	return m.startPromptCommand("/hunt", buildHuntPrompt(symptom))
}

func init() {
	subcommandRegistry.Register(&huntSubcommand{})
}
