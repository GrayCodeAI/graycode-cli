package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// addDirSubcommand implements the /add-dir slash command. It adds a
// directory to the read-only context (system context + allowed dirs).
type addDirSubcommand struct{}

func (a *addDirSubcommand) Name() string        { return "add-dir" }
func (a *addDirSubcommand) Aliases() []string   { return nil }
func (a *addDirSubcommand) Description() string { return "add a directory to the agent's read context" }
func (a *addDirSubcommand) Usage() string       { return "/add-dir <path>" }
func (a *addDirSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if len(args) < 1 {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /add-dir <path>"})
		return m, nil
	}
	dirArg := strings.TrimSpace(strings.TrimPrefix(text, "/add-dir"))
	abs, contextBlock, err := additionalDirContext(dirArg)
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		return m, nil
	}
	if !hasString(addDirs, abs) {
		addDirs = append(addDirs, abs)
		m.session.AppendSystemContext(contextBlock)
		m.session.SetAllowedDirs(addDirs)
	}
	m.messages = append(m.messages, displayMsg{role: "system", content: "Added directory to context: " + abs})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&addDirSubcommand{})
}
