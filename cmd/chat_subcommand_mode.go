package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/feature/shellmode"
)

// modeSubcommand implements the /mode slash command. It shows or
// changes the chat's mode (auto, shell, agent, or toggle).
type modeSubcommand struct{}

func (mo *modeSubcommand) Name() string      { return "mode" }
func (mo *modeSubcommand) Aliases() []string { return nil }
func (mo *modeSubcommand) Description() string {
	return "show or change the chat mode (auto|shell|agent|toggle)"
}
func (mo *modeSubcommand) Usage() string { return "/mode [auto|shell|agent|toggle]" }
func (mo *modeSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		current := m.modeManager.Current()
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Mode: %s (auto | shell | agent)", current.String())})
		return m, nil
	}
	arg := strings.ToLower(args[0])
	if arg == "toggle" {
		newMode := m.modeManager.Toggle()
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Mode → %s", newMode.String())})
		return m, nil
	}
	mode, ok := shellmode.ParseMode(arg)
	if !ok {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /mode [auto|shell|agent|toggle]"})
		return m, nil
	}
	m.modeManager.Set(mode)
	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Mode → %s", mode.String())})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&modeSubcommand{})
}
