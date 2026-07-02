package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	result, isErr := runSlashShellCommand(m, cmdStr)
	if isErr {
		m.messages = append(m.messages, displayMsg{role: "error", content: result})
		return m, nil
	}
	if shellCommandFailed(result, isErr) {
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
