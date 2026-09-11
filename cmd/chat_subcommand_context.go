package cmd

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine/project"
)

// contextSubcommand implements the /context slash command. It
// shows the current context, or initializes it (init), or shows
// the loaded project context files (show).
type contextSubcommand struct{}

func (c *contextSubcommand) Name() string        { return "context" }
func (c *contextSubcommand) Aliases() []string   { return nil }
func (c *contextSubcommand) Description() string { return "show or init the project context" }
func (c *contextSubcommand) Usage() string       { return "/context [init|show]" }
func (c *contextSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/context"))
	if arg == "init" {
		cwd, _ := os.Getwd()
		pc := project.NewProjectContext(cwd)
		return m.startPromptCommand("/context init", pc.InitPrompt())
	}
	if arg == "show" {
		cwd, _ := os.Getwd()
		pc := project.NewProjectContext(cwd)
		content := pc.Load()
		if content == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No project context files found. Run /context init to generate."})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: content})
		}
		return m, nil
	}
	m.messages = append(m.messages, displayMsg{role: "system", content: hawkconfig.BuildContextWithDirs(addDirs)})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&contextSubcommand{})
}
