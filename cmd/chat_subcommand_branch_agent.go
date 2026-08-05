package cmd

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// branchAgentSubcommand creates a hawk/agent-* branch from main/master.
type branchAgentSubcommand struct{}

func (c *branchAgentSubcommand) Name() string      { return "branch-agent" }
func (c *branchAgentSubcommand) Aliases() []string { return []string{"agent-branch"} }
func (c *branchAgentSubcommand) Description() string {
	return "create hawk/agent-* branch if on main/master"
}
func (c *branchAgentSubcommand) Usage() string { return "/branch-agent" }

func (c *branchAgentSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	info := engine.InspectGitBranch("")
	if !info.HasRepo {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Not a git repository."})
		return m, nil
	}
	if !info.OnDefault {
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf(
			"Already on `%s` (not a default branch). No change.", info.Branch,
		)})
		return m, nil
	}
	name, err := engine.EnsureAgentBranch("")
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		return m, nil
	}
	m.refreshStatusBarLeft(true)
	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf(
		"✓ Checked out `%s` — agent edits stay off %s.\nTip: `/commit` when ready.",
		name, info.Branch,
	)})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&branchAgentSubcommand{})
}
