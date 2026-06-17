package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
)

// yaadSubcommand implements the /yaad slash command. It shows a
// summary of the yaad graph memory, or searches it (/yaad search).
type yaadSubcommand struct{}

func (y *yaadSubcommand) Name() string        { return "yaad" }
func (y *yaadSubcommand) Aliases() []string   { return nil }
func (y *yaadSubcommand) Description() string { return "show or search the yaad graph memory" }
func (y *yaadSubcommand) Usage() string       { return "/yaad [search <query>]" }
func (y *yaadSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if len(args) >= 2 && args[0] == "search" {
		query := strings.TrimSpace(strings.Join(args[1:], " "))
		if query == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /yaad search <query>"})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: memory.FormatYaadSearch(query, 10)})
		return m, nil
	}
	m.messages = append(m.messages, displayMsg{role: "system", content: memory.FormatYaadDetail(5)})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&yaadSubcommand{})
}
