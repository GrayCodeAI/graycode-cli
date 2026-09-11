package cmd

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

// memorySubcommand implements the /memory slash command.
// It prints the project's AGENTS.md content.
type memorySubcommand struct{}

func (m *memorySubcommand) Name() string        { return "memory" }
func (m *memorySubcommand) Aliases() []string   { return nil }
func (m *memorySubcommand) Description() string { return "print project instructions (AGENTS.md)" }
func (m *memorySubcommand) Usage() string       { return "" }
func (m *memorySubcommand) Handle(ml *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	md := strings.TrimSpace(hawkconfig.LoadAgentsMD())
	if md == "" {
		ml.messages = append(ml.messages, displayMsg{role: "system", content: "No AGENTS.md project instructions found.\nUse /harrier for persistent graph memory."})
	} else {
		ml.messages = append(ml.messages, displayMsg{role: "system", content: "Project instructions (AGENTS.md):\n" + md})
	}
	return ml, nil
}

func init() {
	subcommandRegistry.Register(&memorySubcommand{})
}
