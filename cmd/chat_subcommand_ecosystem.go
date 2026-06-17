package cmd

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

// ecosystemSubcommand implements the /ecosystem slash command. It
// shows the formatted ecosystem panel (provider, model, env).
type ecosystemSubcommand struct{}

func (e *ecosystemSubcommand) Name() string        { return "ecosystem" }
func (e *ecosystemSubcommand) Aliases() []string   { return nil }
func (e *ecosystemSubcommand) Description() string { return "show the ecosystem panel (provider, model, env)" }
func (e *ecosystemSubcommand) Usage() string       { return "" }
func (e *ecosystemSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	settings, err := loadEffectiveSettings()
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		return m, nil
	}
	modelName, providerName := effectiveModelAndProvider(settings)
	if providerName == "" {
		providerName = "auto"
	}
	m.messages = append(m.messages, displayMsg{role: "system", content: hawkconfig.FormatEcosystemPanel(context.Background(), providerName, modelName)})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&ecosystemSubcommand{})
}
