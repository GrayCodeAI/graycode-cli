package cmd

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
)

// pinSubcommand implements the /pin slash command. It sets the
// number of recent messages (default 2) that are protected from
// compaction.
type pinSubcommand struct{}

func (p *pinSubcommand) Name() string      { return "pin" }
func (p *pinSubcommand) Aliases() []string { return nil }
func (p *pinSubcommand) Description() string {
	return "pin the last N exchanges as protected from compaction"
}
func (p *pinSubcommand) Usage() string { return "/pin [N]" }
func (p *pinSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	n := 2
	if len(args) >= 1 {
		if parsed, err := strconv.Atoi(args[0]); err == nil && parsed > 0 {
			n = parsed
		}
	}
	m.session.SetPinnedMessages(n)
	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Pinned last %d messages (protected from compaction).", n)})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&pinSubcommand{})
}
