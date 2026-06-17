package cmd

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// agentsInitSubcommand implements the /agents-init slash command.
// It generates an AGENTS.md file using the detected project type
// as a template, but only if one doesn't already exist.
type agentsInitSubcommand struct{}

func (a *agentsInitSubcommand) Name() string        { return "agents-init" }
func (a *agentsInitSubcommand) Aliases() []string   { return nil }
func (a *agentsInitSubcommand) Description() string { return "generate AGENTS.md from project-type template" }
func (a *agentsInitSubcommand) Usage() string       { return "" }
func (a *agentsInitSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if _, err := os.Stat("AGENTS.md"); err == nil {
		m.messages = append(m.messages, displayMsg{role: "system", content: "AGENTS.md already exists. Remove it first to regenerate."})
		return m, nil
	}
	pt := detectAgentsProjectType()
	content := GenerateAgentsTemplate(pt)
	if err := os.WriteFile("AGENTS.md", []byte(content), 0o644); err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Failed to write AGENTS.md: " + err.Error()})
		return m, nil
	}
	m.messages = append(m.messages, displayMsg{role: "system", content: "Wrote AGENTS.md (project type: " + pt + ")"})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&agentsInitSubcommand{})
}
