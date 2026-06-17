package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/tool"
)

// testSubcommand implements the /test slash command. It runs
// `go test ./...` by default (or whatever command follows
// /test), with a destructive/suspicious safety check. Failures
// are added to the conversation as a user message asking the
// model to fix them.
type testSubcommand struct{}

func (t *testSubcommand) Name() string        { return "test" }
func (t *testSubcommand) Aliases() []string   { return nil }
func (t *testSubcommand) Description() string { return "run tests, add failures to context" }
func (t *testSubcommand) Usage() string       { return "/test [go-test-args...]" }
func (t *testSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	cmdStr := "go test ./..."
	if len(args) >= 1 {
		cmdStr = strings.TrimSpace(strings.TrimPrefix(text, "/test"))
	}
	if tool.IsDestructiveCommand(cmdStr) || tool.IsSuspicious(cmdStr) {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Blocked: command fails safety check"})
		return m, nil
	}
	out, err := exec.CommandContext(context.Background(), "sh", "-c", cmdStr).CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil {
		result += "\n" + err.Error()
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Tests failed:\n%s", result)})
		m.session.AddUser(fmt.Sprintf("[Test failures]\n```\n%s\n```\nPlease fix these test failures.", result))
	} else {
		m.messages = append(m.messages, displayMsg{role: "system", content: "All tests passed."})
	}
	return m, nil
}

func init() {
	subcommandRegistry.Register(&testSubcommand{})
}
