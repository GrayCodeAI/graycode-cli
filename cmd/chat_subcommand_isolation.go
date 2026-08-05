package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// isolationSubcommand sets the unified IsolationProfile: OS sandbox + container story.
type isolationSubcommand struct{}

func (c *isolationSubcommand) Name() string      { return "isolation" }
func (c *isolationSubcommand) Aliases() []string { return []string{"iso"} }
func (c *isolationSubcommand) Description() string {
	return "isolation profile: dev|workspace|strict|container"
}

func (c *isolationSubcommand) Usage() string {
	return "/isolation [dev|workspace|strict|container|os=workspace,container=1]"
}

func (c *isolationSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if m.session == nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "No session"})
		return m, nil
	}
	if len(args) == 0 {
		p := m.session.Isolation()
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf(
			"Isolation: %s\n  OS sandbox: %s\n  Container required: %v\nPresets: dev | workspace | strict | container",
			p.String(), p.OSMode, p.ContainerRequired,
		)})
		return m, nil
	}
	raw := strings.Join(args, " ")
	p, err := engine.ParseIsolationProfile(raw)
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		return m, nil
	}
	m.session.ApplyIsolationProfile(p)
	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf(
		"Isolation → %s (os=%s, container_required=%v)\nShell wrap applies when os is workspace/strict; tools block until Docker is up when container_required.",
		p.String(), p.OSMode, p.ContainerRequired,
	)})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&isolationSubcommand{})
}
