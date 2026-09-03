package cmd

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

// soulSubcommand implements the /soul slash command. It shows the
// loaded coding soul (style + preferences), or initializes one.
type soulSubcommand struct{}

func (s *soulSubcommand) Name() string        { return "soul" }
func (s *soulSubcommand) Aliases() []string   { return nil }
func (s *soulSubcommand) Description() string { return "show or initialize the coding soul" }
func (s *soulSubcommand) Usage() string       { return "/soul [init]" }
func (s *soulSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/soul"))
	if arg == "init" {
		return m.startPromptCommand("/soul init", engine.InitSoulPrompt())
	}
	soul := engine.LoadCodingSoul()
	if soul.Style == "" && soul.Preferences == "" {
		m.messages = append(m.messages, displayMsg{role: "system", content: "No soul file found. Run /soul init to generate one."})
	} else {
		m.messages = append(m.messages, displayMsg{role: "system", content: soul.ForPrompt()})
	}
	return m, nil
}

func init() {
	subcommandRegistry.Register(&soulSubcommand{})
}
