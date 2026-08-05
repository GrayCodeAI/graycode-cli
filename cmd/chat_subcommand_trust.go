package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// trustSubcommand manages folder trust from the chat TUI.
type trustSubcommand struct{}

func (c *trustSubcommand) Name() string        { return "trust" }
func (c *trustSubcommand) Aliases() []string   { return nil }
func (c *trustSubcommand) Description() string { return "folder trust: status | add | remove" }
func (c *trustSubcommand) Usage() string       { return "/trust [status|add|remove]" }

func (c *trustSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	action := "status"
	if len(args) > 0 {
		action = strings.ToLower(args[0])
	}
	switch action {
	case "status", "check", "show", "":
		st := engine.ProjectTrust("")
		m.messages = append(m.messages, displayMsg{role: "system", content: st.Detail()})
	case "add", "allow", "yes":
		if err := engine.TrustProject("", "chat /trust add"); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: icons.CheckBold() + " Project trusted. Project hooks/MCP/plugins may load.\n" + engine.ProjectTrust("").Detail()})
	case "remove", "revoke", "untrust":
		if err := engine.UntrustProject(""); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: "Removed folder trust for this project."})
	default:
		m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Usage: %s", c.Usage())})
	}
	return m, nil
}

func init() {
	subcommandRegistry.Register(&trustSubcommand{})
}
