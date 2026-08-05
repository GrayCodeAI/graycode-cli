package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// autoCommitSubcommand toggles git auto-commit after Write/Edit.
type autoCommitSubcommand struct{}

func (c *autoCommitSubcommand) Name() string      { return "auto-commit" }
func (c *autoCommitSubcommand) Aliases() []string { return []string{"autocommit"} }
func (c *autoCommitSubcommand) Description() string {
	return "auto-commit after Write/Edit: on|off|status"
}
func (c *autoCommitSubcommand) Usage() string { return "/auto-commit [on|off|status]" }

func (c *autoCommitSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if m.session == nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "No session"})
		return m, nil
	}
	action := "status"
	if len(args) > 0 {
		action = strings.ToLower(args[0])
	}
	switch action {
	case "status", "show", "":
		state := "off"
		if m.session.AutoCommit() {
			state = "on"
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf(
			"Auto-commit: %s\nWhen on, successful Write/Edit/StructuredEdit run `git add` + `git commit` for that file.\nTip: use /branch-agent first so commits stay off main.",
			state,
		)})
	case "on", "enable", "true", "1":
		m.session.SetAutoCommit(true)
		m.messages = append(m.messages, displayMsg{role: "system", content: "Auto-commit → on (Write/Edit will commit)"})
	case "off", "disable", "false", "0":
		m.session.SetAutoCommit(false)
		m.messages = append(m.messages, displayMsg{role: "system", content: "Auto-commit → off"})
	default:
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /auto-commit [on|off|status]"})
	}
	return m, nil
}

func init() {
	subcommandRegistry.Register(&autoCommitSubcommand{})
}
