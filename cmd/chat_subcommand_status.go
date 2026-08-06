package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
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
	if m == nil || m.session == nil {
		return "No active session."
	}
	toolCount := 0
	visible := 0
	if m.registry != nil {
		toolCount = len(m.registry.PrimaryTools())
		visible = len(m.registry.EyrieTools())
	}
	work := string(m.session.WorkMode())
	if work == "" {
		work = "act"
	}
	iso := m.session.Isolation().String()
	tr := engine.ProjectTrust("")
	git := engine.InspectGitBranch("")
	ac := "off"
	if m.session.AutoCommit() {
		ac = "on"
	}
	shell := "agent"
	if m.modeManager != nil {
		shell = m.modeManager.Current().String()
	}
	containerInfo := "Host"
	if m.containerReady {
		containerInfo = "Docker Sandbox (bridge net, SSH agent, non-root UID)"
	} else if m.containerErr != nil {
		containerInfo = fmt.Sprintf("Docker Required (error: %v)", m.containerErr)
	} else if m.containerEnabled {
		containerInfo = "Docker Sandbox (starting)"
	}

	info := fmt.Sprintf(
		"Session: %s\nModel: %s/%s\nShell mode: %s\nWork mode: %s\nIsolation: %s\nContainer: %s\nAuto-commit: %s\nFolder trust: %s\nSpec stage: %s\nMessages: %d\nTools: %d visible / %d registered\nGit: %s\n%s",
		m.sessionID, m.session.Provider(), m.session.Model(),
		shell, work, iso, containerInfo, ac, tr.String(),
		specStageLabel(m.session), m.session.MessageCount(),
		visible, toolCount,
		engine.GitSafetyAdvice(git),
		m.session.CostValue().Summary(),
	)
	if tr.Blocked {
		info += "\n" + icons.Alert() + " " + tr.Detail()
	}
	if len(addDirs) > 0 {
		info += "\nAdditional dirs: " + strings.Join(addDirs, ", ")
	}
	return info
}

func init() {
	subcommandRegistry.Register(&statusSubcommand{})
}
