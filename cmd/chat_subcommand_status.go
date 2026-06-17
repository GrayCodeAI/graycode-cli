package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// statusSubcommand implements the /status slash command. It prints
// a multi-line session summary: id, provider, model, mode, tools,
// message count, cost.
type statusSubcommand struct{}

func (s *statusSubcommand) Name() string        { return "status" }
func (s *statusSubcommand) Aliases() []string   { return nil }
func (s *statusSubcommand) Description() string { return "print session id, model, mode, tools, cost" }
func (s *statusSubcommand) Usage() string       { return "" }
func (s *statusSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: buildStatusInfo(m)})
	return m, nil
}

// buildStatusInfo constructs the multi-line status string. Pulled
// out of the original handleCommand switch case so the subcommand
// file can stay self-contained.
func buildStatusInfo(m *chatModel) string {
	toolCount := 0
	if m.registry != nil {
		toolCount = len(m.registry.EyrieTools())
	}
	info := fmt.Sprintf("Session: %s\nModel: %s/%s\nMode: %s\nPermission mode: %s\nMessages: %d\nTools: %d\n%s",
		m.sessionID, m.session.Provider(), m.session.Model(),
		m.modeManager.Current().String(),
		permissionModeLabel(m.session), m.session.MessageCount(), toolCount, m.session.Cost.Summary())
	if len(addDirs) > 0 {
		info += "\nAdditional dirs: " + strings.Join(addDirs, ", ")
	}
	return info
}

func init() {
	subcommandRegistry.Register(&statusSubcommand{})
}

