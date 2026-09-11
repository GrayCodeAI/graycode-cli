package cmd

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
)

// dreamSubcommand implements the /dream slash command. It runs
// memory consolidation: takes the loaded harrier memories and asks
// the model to summarize and clean them up.
type dreamSubcommand struct{}

func (d *dreamSubcommand) Name() string      { return "dream" }
func (d *dreamSubcommand) Aliases() []string { return nil }
func (d *dreamSubcommand) Description() string {
	return "consolidate harrier memories into a coherent summary"
}
func (d *dreamSubcommand) Usage() string { return "" }
func (d *dreamSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	projectDir, _ := os.Getwd()
	status := memory.HarrierStatus()
	if strings.Contains(status, "not initialized") || strings.Contains(status, "no memories") {
		m.messages = append(m.messages, displayMsg{role: "system", content: status + "\nRun 'harrier' to start storing memories, or use /memory to view AGENTS.md."})
		return m, nil
	}
	harrierCtx := memory.LoadHarrierContext(projectDir)
	if harrierCtx == "" {
		m.messages = append(m.messages, displayMsg{role: "system", content: "No harrier memories found to consolidate."})
		return m, nil
	}
	dreamPrompt := `Review the harrier memories below and consolidate them into a coherent summary.
Focus on: recurring patterns, user preferences learned, project context that should persist,
and any corrections or feedback. Remove redundant or outdated entries.
Write the consolidated result as clear, organized harrier memory nodes.

` + harrierCtx
	m.messages = append(m.messages, displayMsg{role: "system", content: "Running memory consolidation...\n" + status})
	return m.startPromptCommand("/dream", dreamPrompt)
}

func init() {
	subcommandRegistry.Register(&dreamSubcommand{})
}
