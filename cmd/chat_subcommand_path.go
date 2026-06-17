package cmd

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

// pathSubcommand implements the /path slash command. It prints
// the developer-path report (HOME, GOROOT, etc.).
type pathSubcommand struct{}

func (p *pathSubcommand) Name() string        { return "path" }
func (p *pathSubcommand) Aliases() []string   { return nil }
func (p *pathSubcommand) Description() string { return "show developer path (HOME, GOROOT, etc.)" }
func (p *pathSubcommand) Usage() string       { return "" }
func (p *pathSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: hawkconfig.FormatDeveloperPathReport(context.Background())})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&pathSubcommand{})
}
