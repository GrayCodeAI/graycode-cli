package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// helpSubcommand implements the /help and /commands slash commands.
// It prints the static help table of all available slash commands.
type helpSubcommand struct{}

func (h *helpSubcommand) Name() string        { return "help" }
func (h *helpSubcommand) Aliases() []string   { return []string{"commands"} }
func (h *helpSubcommand) Description() string { return "show this help" }
func (h *helpSubcommand) Usage() string       { return "" }
func (h *helpSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: staticHelpText()})
	return m, nil
}

// staticHelpText returns the canonical help table. The list is
// curated by hand so it stays under the 70-column terminal width.
func staticHelpText() string {
	return `/add-dir <path>     — Add a directory to context
/branch             — Show git branch/status
/clear              — Clear display
/compact            — Compact conversation (LLM summary)
/commit             — Auto-commit changes
/context            — Show current context
/cost               — Token usage and cost
/diff               — Review changes
/doctor             — Run diagnostics
/env                — Show provider environment status
/files              — Show modified files
/help               — This help message
/history            — List saved sessions
/init               — Analyze project
/metrics            — Show collected metrics
/model              — Show current model
/permissions        — Show tier, sandbox, mode, rules, and effective behavior
/recover            — Recover a session
/refresh            — Refresh context files
/review             — Ask hawk to review changes
/render             — Toggle raw vs rendered output
/reset              — Reset session state
/resume <id>        — Resume session
/revert             — Revert file changes
/security-review    — Ask hawk to review security risks
/snapshot           — Snapshot session
/spec               — Generate spec from conversation
/status             — Session status
/summary            — Summarize the current session
/tag <label>        — Tag session
/think              — Toggle think mode
/theme <t>          — Set theme (dark/light/auto)
/thinkback          — Review reasoning decisions
/tools              — List enabled tools
/usage              — Token usage
/version            — Show hawk version
/vim                — Toggle vim mode
/welcome            — Show startup summary
/quit               — Exit hawk`
}

func init() {
	subcommandRegistry.Register(&helpSubcommand{})
}
