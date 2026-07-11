package cmd

import (
	"strings"

			tea "charm.land/bubbletea/v2"
)

// thinkSubcommand implements the /think slash command. It prompts
// the model with buildThinkPrompt for the given topic.
type thinkSubcommand struct{}

func (t *thinkSubcommand) Name() string        { return "think" }
func (t *thinkSubcommand) Aliases() []string   { return nil }
func (t *thinkSubcommand) Description() string { return "ask the model to plan a topic before acting" }
func (t *thinkSubcommand) Usage() string       { return "/think <what to plan>" }
func (t *thinkSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	topic := strings.TrimSpace(strings.TrimPrefix(text, "/think"))
	if topic == "" {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /think <what to plan>"})
		return m, nil
	}
	return m.startPromptCommand("/think", buildThinkPrompt(topic))
}

func init() {
	subcommandRegistry.Register(&thinkSubcommand{})
}
