package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// runSubcommand implements the /run slash command. It runs an
// arbitrary shell command (with safety checks) and adds the
// output to the conversation as a system message plus a
// "[Command output: ...]" user message so the model can see
// what happened.
type runSubcommand struct{}

func (r *runSubcommand) Name() string        { return "run" }
func (r *runSubcommand) Aliases() []string   { return nil }
func (r *runSubcommand) Description() string { return "run shell command, add output to context" }
func (r *runSubcommand) Usage() string       { return "/run <command>" }
func (r *runSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if len(args) < 1 {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /run <command>"})
		return m, nil
	}
	cmdStr := strings.TrimSpace(strings.TrimPrefix(text, "/run"))
	result, isErr := runSlashShellCommand(m, cmdStr)
	role := "system"
	if isErr {
		role = "error"
	}
	m.messages = append(m.messages, displayMsg{role: role, content: fmt.Sprintf("$ %s\n%s", cmdStr, result)})
	if !isErr {
		m.session.AddUser(fmt.Sprintf("[Command output: %s]\n```\n%s\n```", cmdStr, result))
	}
	return m, nil
}

func init() {
	subcommandRegistry.Register(&runSubcommand{})
}
