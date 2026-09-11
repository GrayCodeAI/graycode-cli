package cmd

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
)

// harrierSubcommand implements the /harrier slash command. It shows a
// summary of the harrier graph memory, or searches it (/harrier search).
type harrierSubcommand struct{}

func (y *harrierSubcommand) Name() string        { return "harrier" }
func (y *harrierSubcommand) Aliases() []string   { return nil }
func (y *harrierSubcommand) Description() string { return "show or search the harrier graph memory" }
func (y *harrierSubcommand) Usage() string       { return "/harrier [search <query>]" }
func (y *harrierSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if len(args) >= 2 && args[0] == "search" {
		query := strings.TrimSpace(strings.Join(args[1:], " "))
		if query == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /harrier search <query>"})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: memory.FormatHarrierSearch(query, 10)})
		return m, nil
	}
	m.messages = append(m.messages, displayMsg{role: "system", content: memory.FormatHarrierDetail(5)})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&harrierSubcommand{})
}
