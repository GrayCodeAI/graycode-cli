package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/feature/shellmode"
)

// modeSubcommand implements the /mode slash command.
//
// Two layers:
//   - Work modes (plan|act|review): tool visibility + read-only bash (control plane)
//   - Shell modes (auto|shell|agent|toggle): input routing for the TUI
type modeSubcommand struct{}

func (mo *modeSubcommand) Name() string      { return "mode" }
func (mo *modeSubcommand) Aliases() []string { return nil }
func (mo *modeSubcommand) Description() string {
	return "work mode (plan|act|review) or shell mode (auto|shell|agent|toggle)"
}

func (mo *modeSubcommand) Usage() string {
	return "/mode [plan|act|review|auto|shell|agent|toggle]"
}

func (mo *modeSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		shell := m.modeManager.Current().String()
		work := engine.WorkModeAct
		if m.session != nil {
			work = m.session.WorkMode()
		}
		iso := "dev"
		if m.session != nil {
			iso = m.session.Isolation().String()
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf(
			"Work mode: %s (plan | act | review)\nShell mode: %s (auto | shell | agent)\nIsolation: %s",
			work, shell, iso,
		)})
		return m, nil
	}
	arg := strings.ToLower(args[0])

	// Work modes first (product control plane).
	if wm, err := engine.ParseWorkMode(arg); err == nil &&
		(arg == "plan" || arg == "act" || arg == "review" ||
			arg == "planning" || arg == "research" || arg == "build" ||
			arg == "inspect" || arg == "readonly" || arg == "read-only" || arg == "ro") {
		if m.session == nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "No session for work mode"})
			return m, nil
		}
		if err := m.session.SetWorkMode(wm); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		hint := ""
		switch wm {
		case engine.WorkModePlan:
			hint = " — research only; no file writes"
		case engine.WorkModeReview:
			hint = " — inspect with evidence; no writes"
		case engine.WorkModeAct:
			hint = " — full tools (lazy essential surface)"
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Work mode → %s%s", wm, hint)})
		return m, nil
	}

	if arg == "toggle" {
		newMode := m.modeManager.Toggle()
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Shell mode → %s", newMode.String())})
		return m, nil
	}
	mode, ok := shellmode.ParseMode(arg)
	if !ok {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /mode [plan|act|review|auto|shell|agent|toggle]"})
		return m, nil
	}
	m.modeManager.Set(mode)
	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Shell mode → %s", mode.String())})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&modeSubcommand{})
}
