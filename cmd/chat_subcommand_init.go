package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// initSubcommand implements the /init slash command. It prompts the
// model to analyze the project structure and propose an AGENTS.md
// scaffold if one is missing.
type initSubcommand struct{}

func (i *initSubcommand) Name() string      { return "init" }
func (i *initSubcommand) Aliases() []string { return nil }
func (i *initSubcommand) Description() string {
	return "analyze project structure and propose AGENTS.md scaffold"
}
func (i *initSubcommand) Usage() string { return "" }
func (i *initSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	initPrompt := "Analyze this project: read the README, check the directory structure, identify the language/framework, build system, and test runner. Report progress as you go (e.g., 'Analyzing file 5/20...'). Give me a brief summary."
	if _, err := os.Stat("AGENTS.md"); os.IsNotExist(err) {
		pt := detectAgentsProjectType()
		initPrompt += fmt.Sprintf("\n\nNote: No AGENTS.md found. I detected project type %q. After your analysis, suggest running /agents-init to generate one.", pt)
	}
	return m.startPromptCommand("/init", initPrompt)
}

func init() {
	subcommandRegistry.Register(&initSubcommand{})
}
