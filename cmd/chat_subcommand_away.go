package cmd

import (
	"fmt"
	"strings"

			tea "charm.land/bubbletea/v2"
)

// awaySubcommand implements the /away slash command. It asks the
// model to produce a 1-3 sentence "while you were away" recap of
// the recent conversation.
type awaySubcommand struct{}

func (a *awaySubcommand) Name() string        { return "away" }
func (a *awaySubcommand) Aliases() []string   { return nil }
func (a *awaySubcommand) Description() string { return "generate a brief recap of recent conversation" }
func (a *awaySubcommand) Usage() string       { return "" }
func (a *awaySubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	msgs := m.session.RawMessages()
	if len(msgs) < 4 {
		m.messages = append(m.messages, displayMsg{role: "system", content: "Not enough conversation history for a recap."})
		return m, nil
	}
	start := 0
	if len(msgs) > 30 {
		start = len(msgs) - 30
	}
	var summary strings.Builder
	for _, msg := range msgs[start:] {
		preview := msg.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		if msg.Role == "user" && preview != "" {
			summary.WriteString(fmt.Sprintf("User: %s\n", preview))
		} else if msg.Role == "assistant" && msg.Content != "" {
			summary.WriteString(fmt.Sprintf("Assistant: %s\n", preview))
		}
	}
	awayPrompt := fmt.Sprintf(`You are generating a brief "while you were away" recap for a coding session.

Rules:
- 1-3 sentences maximum
- Focus on what was accomplished and what's next
- Do NOT include status reports or tool call details
- Be concise and actionable

Recent conversation:
%s

Generate the recap:`, summary.String())
	m.messages = append(m.messages, displayMsg{role: "system", content: "Generating recap..."})
	return m.startPromptCommand("/away", awayPrompt)
}

func init() {
	subcommandRegistry.Register(&awaySubcommand{})
}
