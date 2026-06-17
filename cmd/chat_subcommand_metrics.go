package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// metricsSubcommand implements the /metrics slash command. It prints
// the formatted session metrics (counters, latencies).
type metricsSubcommand struct{}

func (mt *metricsSubcommand) Name() string        { return "metrics" }
func (mt *metricsSubcommand) Aliases() []string   { return nil }
func (mt *metricsSubcommand) Description() string { return "print session metrics (counters, latencies)" }
func (mt *metricsSubcommand) Usage() string       { return "" }
func (mt *metricsSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: m.session.Metrics().Format()})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&metricsSubcommand{})
}
