package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// mcpSubcommand implements the /mcp slash command. It prints the
// MCP server summary (status, tools).
type mcpSubcommand struct{}

func (mc *mcpSubcommand) Name() string        { return "mcp" }
func (mc *mcpSubcommand) Aliases() []string   { return nil }
func (mc *mcpSubcommand) Description() string { return "show MCP server status" }
func (mc *mcpSubcommand) Usage() string       { return "" }
func (mc *mcpSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: m.mcpSummary()})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&mcpSubcommand{})
}
