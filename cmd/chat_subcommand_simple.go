package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/plugin"
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

	// /parallel — run commands in parallel
	subcommandRegistry.Register(&delegatingCommand{
		name:        "parallel",
		description: "run commands in parallel (delegates to handleParallelCommand)",
		usage:       "/parallel [args...]",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			return m.handleParallelCommand(append([]string{"/parallel"}, args...), text)
		},
	})

	// /skills — list, search, install, remove skills
	subcommandRegistry.Register(&delegatingCommand{
		name:        "skills",
		description: "list, search, install, remove skills",
		usage:       "/skills [subcommand]",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			return m.handleSkillsCommand(append([]string{"/skills"}, args...), text)
		},
	})

	// /tasks — show task list
	subcommandRegistry.Register(&delegatingCommand{
		name:        "tasks",
		description: "show the current task list",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			tasks := tool.GetTaskStore().List()
			if len(tasks) == 0 {
				m.messages = append(m.messages, displayMsg{role: "system", content: "No tasks."})
				return m, nil
			}
			var b strings.Builder
			for _, t := range tasks {
				status := string(t.Status)
				icon := "○"
				if t.Status == tool.TaskStatusCompleted {
					icon = "●"
				} else if t.Status == tool.TaskStatusInProgress {
					icon = "◐"
				}
				b.WriteString(fmt.Sprintf("  %s %s [%s] %s\n", icon, t.ID, status, t.Subject))
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
			return m, nil
		},
	})

	// /vibe — enter vibe coding mode
	subcommandRegistry.Register(&delegatingCommand{
		name:        "vibe",
		description: "enter vibe coding mode (auto-apply all changes)",
		usage:       "/vibe [additional prompt]",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			prompt := "Enter vibe coding mode. Auto-apply all changes, run tests after each edit, and iterate until tests pass. Start by reading the project structure."
			if len(args) > 0 {
				prompt = strings.TrimSpace(strings.TrimPrefix(text, "/vibe"))
			}
			return m.startPromptCommand("/vibe", prompt)
		},
	})

	// /learn — LLM-powered skill advisor
	subcommandRegistry.Register(&delegatingCommand{
		name:        "learn",
		description: "LLM-powered skill advisor (deep, update)",
		usage:       "/learn [deep|update]",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			cwd, _ := os.Getwd()
			deep := len(args) >= 1 && args[0] == "deep"
			update := len(args) >= 1 && args[0] == "update"
			ctx := plugin.GatherLearnContext(cwd)
			if deep || update {
				ctx.SourceInfo = plugin.GatherDeepSourceInfo(cwd)
			}
			if update {
				summary := plugin.FormatLearnSummary(ctx, true)
				prompt := plugin.BuildLearnUpdatePrompt(ctx)
				return m.startPromptCommand(summary, prompt)
			}
			summary := plugin.FormatLearnSummary(ctx, deep)
			prompt := plugin.BuildLearnPrompt(ctx)
			return m.startPromptCommand(summary, prompt)
		},
	})

	// /cron — list scheduled cron jobs
	subcommandRegistry.Register(&delegatingCommand{
		name:        "cron",
		description: "list scheduled cron jobs",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			jobs := tool.GetCronScheduler().List()
			if len(jobs) == 0 {
				m.messages = append(m.messages, displayMsg{role: "system", content: "No scheduled jobs."})
				return m, nil
			}
			var b strings.Builder
			for _, j := range jobs {
				jtype := "recurring"
				if !j.Recurring {
					jtype = "one-shot"
				}
				b.WriteString(fmt.Sprintf("  %s [%s] %s next: %s\n", j.ID, jtype, j.Schedule, j.NextRun.Format("Jan 02 15:04")))
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
			return m, nil
		},
	})

	// /glm <on|off|default> — toggle GLM/Z.ai extended reasoning
	subcommandRegistry.Register(&delegatingCommand{
		name:        "glm",
		description: "toggle GLM/Z.ai extended reasoning",
		usage:       "/glm <on|off|default>",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			if len(args) < 1 {
				cur, _ := hawkconfig.SettingValue(hawkconfig.LoadSettings(), "glmthinking")
				m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /glm <on|off|default> — toggle GLM/Z.ai extended reasoning\nCurrent: " + cur})
				return m, nil
			}
			switch strings.ToLower(args[0]) {
			case "on":
				_ = hawkconfig.SetGlobalSetting("glmthinking", "true")
				enabled := true
				m.session.GLMThinkingEnabled = &enabled
				m.messages = append(m.messages, displayMsg{role: "system", content: "GLM thinking → enabled"})
			case "off":
				_ = hawkconfig.SetGlobalSetting("glmthinking", "false")
				disabled := false
				m.session.GLMThinkingEnabled = &disabled
				m.messages = append(m.messages, displayMsg{role: "system", content: "GLM thinking → disabled"})
			case "default":
				_ = hawkconfig.SetGlobalSetting("glmthinking", "default")
				m.session.GLMThinkingEnabled = nil
				m.messages = append(m.messages, displayMsg{role: "system", content: "GLM thinking → default (model decides)"})
			default:
				m.messages = append(m.messages, displayMsg{role: "error", content: "Valid options: on, off, default"})
			}
			return m, nil
		},
	})

	// /vim — toggle vim mode
	subcommandRegistry.Register(&delegatingCommand{
		name:        "vim",
		description: "toggle vim mode",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			if m.vim == nil {
				m.vim = NewVimState()
			}
			m.vim.SetEnabled(!m.vim.IsEnabled())
			state := "disabled"
			if m.vim.IsEnabled() {
				state = "enabled (press Esc for NORMAL mode)"
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: "Vim mode " + state})
			return m, nil
		},
	})

	// /hooks — show configured hooks
	subcommandRegistry.Register(&delegatingCommand{
		name:        "hooks",
		description: "show configured hooks",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			m.messages = append(m.messages, displayMsg{role: "system", content: hooksSummary()})
			return m, nil
		},
	})

	// /plugins — list installed plugins
	subcommandRegistry.Register(&delegatingCommand{
		name:        "plugins",
		description: "list installed plugins",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			m.messages = append(m.messages, displayMsg{role: "system", content: pluginsSummary(m.pluginRuntime)})
			return m, nil
		},
	})

	// /plugin — alias for /plugins
	subcommandRegistry.Register(&delegatingCommand{
		name:        "plugin",
		description: "list installed plugins (alias for /plugins)",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			m.messages = append(m.messages, displayMsg{role: "system", content: pluginsSummary(m.pluginRuntime)})
			return m, nil
		},
	})

	// /upgrade — check for updates
	subcommandRegistry.Register(&delegatingCommand{
		name:        "upgrade",
		description: "check for hawk updates",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			return m.startPromptCommand("/upgrade", "Check for hawk updates and show the latest available version.")
		},
	})

	// /keybindings — show keybindings
	subcommandRegistry.Register(&delegatingCommand{
		name:        "keybindings",
		description: "show keybindings",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Keybindings:\n  Enter           — Submit\n  Ctrl+C          — Cancel/Exit\n  Ctrl+Shift+C    — Copy (input draft or chat)\n  Ctrl+\\          — Native text selection\n  Ctrl+L          — Clear\n  Up/Down         — History\n  Tab             — Complete\n  /mouse off      — Enable click-drag copy"})
			return m, nil
		},
	})

	// /statusline — print compact status line
	subcommandRegistry.Register(&delegatingCommand{
		name:        "statusline",
		description: "print a compact status line",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			m.messages = append(m.messages, displayMsg{role: "system", content: statusLineSummary(m)})
			return m, nil
		},
	})

	// /remote-env — show remote env summary
	subcommandRegistry.Register(&delegatingCommand{
		name:        "remote-env",
		description: "show the remote env summary",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			m.messages = append(m.messages, displayMsg{role: "system", content: envSummary(m.session.Provider(), m.session.Model())})
			return m, nil
		},
	})

	// /thinkback, /think-back, /thinkback-play — review reasoning
	subcommandRegistry.Register(&delegatingCommand{
		name:        "thinkback",
		aliases:     []string{"think-back", "thinkback-play"},
		description: "review reasoning decisions (thinkback/think-back/thinkback-play)",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			summary := "Review the thinking/reasoning from this conversation and highlight key decision points and alternatives considered."
			return m.startPromptCommand("/thinkback", summary)
		},
	})

	// /ultrareview — adversarial code review
	subcommandRegistry.Register(&delegatingCommand{
		name:        "ultrareview",
		description: "perform a deep, adversarial code review",
		usage:       "",
		handler: func(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
			return m.startPromptCommand("/ultrareview", "Perform a deep, adversarial code review of this change set. Prioritize correctness, security, regressions, and missing tests.")
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
