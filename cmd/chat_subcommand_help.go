package cmd

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// helpSubcommand implements the /help and /commands slash commands.
// It prints the dynamically-generated help table of all registered
// subcommands (sorted by name, with description).
type helpSubcommand struct{}

func (h *helpSubcommand) Name() string        { return "help" }
func (h *helpSubcommand) Aliases() []string   { return []string{"commands"} }
func (h *helpSubcommand) Description() string { return "show this help" }
func (h *helpSubcommand) Usage() string       { return "" }
func (h *helpSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "system", content: dynamicHelpText()})
	return m, nil
}

// dynamicHelpText generates the help table from the live
// SubcommandRegistry. Each entry is formatted as
// `/<name> <args>     — <description>`.
//
// The longest name (including args) determines the column
// width, capped at 40 columns to fit a 70-column terminal.
func dynamicHelpText() string {
	all := subcommandRegistry.All()
	type entry struct {
		cmd  string
		desc string
	}
	entries := make([]entry, 0, len(all))
	maxLen := 0
	for _, sub := range all {
		usage := sub.Usage()
		// Usage strings may or may not start with a space;
		// normalize so the column separator lands on the same
		// place for every line.
		if usage != "" && !strings.HasPrefix(usage, " ") {
			usage = " " + usage
		}
		cmd := "/" + sub.Name() + usage
		entries = append(entries, entry{cmd: cmd, desc: sub.Description()})
		if len(cmd) > maxLen {
			maxLen = len(cmd)
		}
	}
	if maxLen > 40 {
		maxLen = 40
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].cmd < entries[j].cmd
	})
	var b strings.Builder
	for _, e := range entries {
		pad := maxLen - len(e.cmd) + 1
		if pad < 1 {
			pad = 1
		}
		b.WriteString(e.cmd)
		for i := 0; i < pad; i++ {
			b.WriteByte(' ')
		}
		b.WriteString("— ")
		b.WriteString(e.desc)
		b.WriteByte('\n')
	}
	return b.String()
}

func init() {
	subcommandRegistry.Register(&helpSubcommand{})
}

