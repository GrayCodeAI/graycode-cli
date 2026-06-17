package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// bughunterSubcommand implements the /bughunter slash command. It
// prompts the model to find likely bugs in the current codebase.
type bughunterSubcommand struct{}

func (b *bughunterSubcommand) Name() string        { return "bughunter" }
func (b *bughunterSubcommand) Aliases() []string   { return nil }
func (b *bughunterSubcommand) Description() string { return "hunt for likely bugs in the current codebase" }
func (b *bughunterSubcommand) Usage() string       { return "" }
func (b *bughunterSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.startPromptCommand("/bughunter", "Hunt for likely bugs in the current codebase and changes. Prioritize concrete defects that can be reproduced or fixed.")
}

func init() {
	subcommandRegistry.Register(&bughunterSubcommand{})
}
