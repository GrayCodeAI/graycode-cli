package cmd

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// versionSubcommand implements the /version slash command. It prints
// the running graycode version. Follows the SubcommandRegistry pattern
// demonstrated in chat_subcommand_branch.go.
type versionSubcommand struct{}

func (v *versionSubcommand) Name() string        { return "version" }
func (v *versionSubcommand) Aliases() []string   { return nil }
func (v *versionSubcommand) Description() string { return "print the running graycode version" }
func (v *versionSubcommand) Usage() string       { return "" }
func (v *versionSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("graycode v%s", DisplayVersion())})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&versionSubcommand{})
}
