package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// init() registers a large batch of simple /slash commands via
// the SubcommandRegistry. Each command's body is taken directly
// from the original case block in chat_commands.go; this file
// is the canonical location for those simple commands.

func init() {
	// /copy — copy chat or input
	subcommandRegistry.Register(&delegatingCommand{
		name:        "copy",
		description: "copy chat or input (all|input|last|assistant)",
		usage:       "/copy <all|input|last|assistant>",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			// handleCopyCommand expects parts[0] to be the command
			// name. The registry dispatcher strips it, so re-add it.
			return m.handleCopyCommand(append([]string{"/copy"}, args...))
		},
	})

	// /select — pause TUI for native text selection
	subcommandRegistry.Register(&delegatingCommand{
		name:        "select",
		description: "pause TUI for native text selection",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			return m, enterSelectionMode(m.ref, m.copyableTranscript(), m.mouseEnabled())
		},
	})

	// /mouse — toggle mouse capture
	subcommandRegistry.Register(&delegatingCommand{
		name:        "mouse",
		description: "toggle mouse capture (off = click-drag copy)",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			// handleMouseCommand expects parts[0] to be the command
			// name. The registry dispatcher strips it, so re-add it.
			m.handleMouseCommand(append([]string{"/mouse"}, args...))
			return m, nil
		},
	})

	// /undo — undo last exchange (file edits via tool.UndoLatest)
	subcommandRegistry.Register(&delegatingCommand{
		name:        "undo",
		description: "undo the last exchange (file edits)",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			restored, err := tool.UndoLatest()
			if err != nil {
				m.messages = append(m.messages, displayMsg{role: "system", content: "No file changes to undo"})
			} else {
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Undid %s", restored)})
			}
			return m, nil
		},
	})

	// /theme <t> — set theme (dark|light|auto)
	subcommandRegistry.Register(&delegatingCommand{
		name:        "theme",
		description: "set theme (dark|light|auto)",
		usage:       "/theme <dark|light|auto>",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			if len(args) < 1 {
				m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /theme <dark|light|auto>"})
				return m, nil
			}
			if err := hawkconfig.SetGlobalSetting("theme", args[0]); err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			} else {
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Theme set to: %s (restart to apply)", args[0])})
			}
			return m, nil
		},
	})

	// /color <hex> — set agent color
	subcommandRegistry.Register(&delegatingCommand{
		name:        "color",
		description: "set agent color (hex value)",
		usage:       "/color <hex-color>",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			if len(args) < 1 {
				m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /color <hex-color>"})
				return m, nil
			}
			if err := hawkconfig.SetGlobalSetting("agentColor", args[0]); err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			} else {
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Agent color set to: %s", args[0])})
			}
			return m, nil
		},
	})

	// /fast — toggle fast mode
	subcommandRegistry.Register(&delegatingCommand{
		name:        "fast",
		description: "toggle fast mode (cheapest model for this provider)",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			savedModel := hawkconfig.ActiveModel(context.Background())
			if m.session.Model() == savedModel {
				norm := hawkconfig.NormalizeProviderForEngine(m.session.Provider())
				fastModel := hawkconfig.CheapestModelForProvider(norm, m.session.Model())
				if strings.TrimSpace(fastModel) == "" {
					fastModel = hawkconfig.DefaultModelForProvider(norm)
				}
				if strings.TrimSpace(fastModel) == "" {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Fast mode: no catalog model resolved for this provider"})
					return m, nil
				}
				m.session.SetModel(fastModel)
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Fast mode on → %s", fastModel)})
			} else {
				m.session.SetModel(savedModel)
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Fast mode off → %s", savedModel)})
			}
			return m, nil
		},
	})

	// /effort <level> — set reasoning effort
	subcommandRegistry.Register(&delegatingCommand{
		name:        "effort",
		description: "set reasoning effort (low|medium|high)",
		usage:       "/effort <low|medium|high>",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			if len(args) < 1 {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /effort <low|medium|high>"})
				return m, nil
			}
			level := strings.ToLower(args[0])
			switch level {
			case "low", "medium", "high":
				_ = hawkconfig.SetGlobalSetting("reasoningEffort", level)
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Reasoning effort → %s", level)})
			default:
				m.messages = append(m.messages, displayMsg{role: "error", content: "Valid levels: low, medium, high"})
			}
			return m, nil
		},
	})

	// /agents — list active agents/teammates
	subcommandRegistry.Register(&delegatingCommand{
		name:        "agents",
		description: "list active agents/teammates",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			return m.startPromptCommand("/agents", "List all active agents and teammates in the current session. Show their status and assigned tasks.")
		},
	})
}

// delegatingCommand is a ChatSubcommand implementation that wraps
// a handler function. Used for the many simple /slash commands
// that just dispatch to a small body. Avoids the boilerplate of
// defining a struct per command.
type delegatingCommand struct {
	name        string
	aliases     []string
	description string
	usage       string
	handler     func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd)
}

func (d *delegatingCommand) Name() string             { return d.name }
func (d *delegatingCommand) Aliases() []string        { return d.aliases }
func (d *delegatingCommand) Description() string      { return d.description }
func (d *delegatingCommand) Usage() string            { return d.usage }
func (d *delegatingCommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return d.handler(m, args, text)
}
