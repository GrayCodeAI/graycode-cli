package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/tool"
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
	if tool.IsDestructiveCommand(cmdStr) || tool.IsSuspicious(cmdStr) {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Blocked: command fails safety check"})
		return m, nil
	}
	out, err := exec.CommandContext(context.Background(), "sh", "-c", cmdStr).CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil {
		result += "\n" + err.Error()
	}
	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("$ %s\n%s", cmdStr, result)})
	m.session.AddUser(fmt.Sprintf("[Command output: %s]\n```\n%s\n```", cmdStr, result))
	return m, nil
}

func init() {
	subcommandRegistry.Register(&runSubcommand{})
}
