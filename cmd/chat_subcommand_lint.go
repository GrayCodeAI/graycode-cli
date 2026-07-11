package cmd

import (
	"fmt"
	"strings"

			tea "charm.land/bubbletea/v2"
)

// lintSubcommand implements the /lint slash command. It runs
// `golangci-lint run ./...` by default (or whatever command
// follows /lint), with a destructive/suspicious safety check.
// Non-empty output is added to the conversation as a user
// message asking the model to fix the issues.
type lintSubcommand struct{}

func (l *lintSubcommand) Name() string        { return "lint" }
func (l *lintSubcommand) Aliases() []string   { return nil }
func (l *lintSubcommand) Description() string { return "run linter, add issues to context" }
func (l *lintSubcommand) Usage() string       { return "/lint [linter-args...]" }
func (l *lintSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	cmdStr := "golangci-lint run ./..."
	if len(args) >= 1 {
		cmdStr = strings.TrimSpace(strings.TrimPrefix(text, "/lint"))
	}
	result, isErr := runSlashShellCommand(m, cmdStr)
	if isErr {
		m.messages = append(m.messages, displayMsg{role: "error", content: result})
		return m, nil
	}
	if result == "" {
		m.messages = append(m.messages, displayMsg{role: "system", content: "No lint issues."})
	} else {
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Lint issues:\n%s", result)})
		m.session.AddUser(fmt.Sprintf("[Lint output]\n```\n%s\n```\nPlease fix these lint issues.", result))
	}
	return m, nil
}

func init() {
	subcommandRegistry.Register(&lintSubcommand{})
}
