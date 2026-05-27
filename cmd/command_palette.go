package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommandPaletteEntry represents a single command in the palette.
type CommandPaletteEntry struct {
	Name        string
	Description string
	Category    string
	Action      string // slash command to execute
}

// CommandPalette is a Ctrl+K command palette for quick command discovery.
type CommandPalette struct {
	open     bool
	input    textinput.Model
	entries  []CommandPaletteEntry
	filtered []CommandPaletteEntry
	sel      int
	width    int
}

var (
	paletteTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	paletteInputStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1)
	paletteItemStyle     = lipgloss.NewStyle().Padding(0, 1)
	paletteSelStyle      = lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("240")).Foreground(lipgloss.Color("230"))
	paletteDescStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	paletteCategoryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	paletteDimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	paletteBoxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1)
)

// NewCommandPalette creates a new command palette with all available commands.
func NewCommandPalette(width int) *CommandPalette {
	ti := textinput.New()
	ti.Placeholder = "Type to search commands..."
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 40

	cp := &CommandPalette{
		input: ti,
		width: width,
	}
	cp.entries = cp.buildEntries()
	cp.filtered = cp.entries
	return cp
}

// buildEntries builds the full list of palette entries from slash commands.
func (cp *CommandPalette) buildEntries() []CommandPaletteEntry {
	var entries []CommandPaletteEntry

	// Core commands
	core := map[string]string{
		"/help":     "Show help and available commands",
		"/model":    "Switch AI model",
		"/config":   "Open configuration panel",
		"/quit":     "Exit hawk",
		"/clear":    "Clear conversation history",
		"/compact":  "Compact conversation to save tokens",
		"/undo":     "Undo the last file change",
		"/snapshot": "Save workspace snapshot",
		"/recover":  "Recover interrupted session",
	}

	// Workflow commands
	workflow := map[string]string{
		"/plan":   "Enter planning mode",
		"/review": "Review recent changes",
		"/commit": "Create smart commit",
		"/test":   "Run project tests",
		"/lint":   "Run linter on changed files",
		"/format": "Format code",
		"/diff":   "Show working diff",
		"/status": "Show git status",
	}

	// Agent commands
	agent := map[string]string{
		"/agent":    "Agent management",
		"/mission":  "Multi-agent mission mode",
		"/exec":     "Execute task non-interactively",
		"/research": "Autonomous research loop",
		"/loop":     "Run in loop mode",
	}

	// Memory & context
	memory := map[string]string{
		"/remember": "Store information in memory",
		"/recall":   "Search memory",
		"/context":  "Export project context",
		"/search":   "Search sessions",
		"/sessions": "List saved sessions",
	}

	// Tools & ecosystem
	tools := map[string]string{
		"/tools":     "List available tools",
		"/mcp":       "Show MCP server config",
		"/plugin":    "Plugin management",
		"/skills":    "Community skills",
		"/sight":     "Code review with sight",
		"/inspect":   "Site audit",
		"/yaad":      "Memory graph operations",
		"/ecosystem": "Ecosystem panel",
	}

	// Diagnostics
	diag := map[string]string{
		"/doctor":  "Run health diagnostics",
		"/path":    "Check developer path readiness",
		"/cost":    "Show cost analysis",
		"/rules":   "Show permission rules",
		"/sandbox": "Sandbox configuration",
		"/eval":    "Run evaluations",
	}

	// Settings
	settings := map[string]string{
		"/acceptEdits":       "Toggle auto-edit mode",
		"/bypassPermissions": "Toggle full auto mode",
		"/default":           "Reset to default permissions",
		"/vim":               "Toggle vim mode",
		"/theme":             "Change color theme",
	}

	addEntries := func(category string, cmds map[string]string) {
		for name, desc := range cmds {
			entries = append(entries, CommandPaletteEntry{
				Name:        name,
				Description: desc,
				Category:    category,
				Action:      name,
			})
		}
	}

	addEntries("Core", core)
	addEntries("Workflow", workflow)
	addEntries("Agent", agent)
	addEntries("Memory", memory)
	addEntries("Tools", tools)
	addEntries("Diagnostics", diag)
	addEntries("Settings", settings)

	return entries
}

// Open opens the command palette.
func (cp *CommandPalette) Open() {
	cp.open = true
	cp.input.SetValue("")
	cp.filtered = cp.entries
	cp.sel = 0
	cp.input.Focus()
}

// Close closes the command palette.
func (cp *CommandPalette) Close() {
	cp.open = false
	cp.input.SetValue("")
	cp.sel = 0
}

// IsOpen returns whether the palette is open.
func (cp *CommandPalette) IsOpen() bool {
	return cp.open
}

// Selected returns the currently selected entry, or nil.
func (cp *CommandPalette) Selected() *CommandPaletteEntry {
	if cp.sel >= 0 && cp.sel < len(cp.filtered) {
		return &cp.filtered[cp.sel]
	}
	return nil
}

// Update handles key events for the command palette.
func (cp *CommandPalette) Update(msg tea.KeyMsg) (string, bool) {
	if !cp.open {
		return "", false
	}

	switch msg.Type {
	case tea.KeyEsc:
		cp.Close()
		return "", true
	case tea.KeyEnter:
		if sel := cp.Selected(); sel != nil {
			action := sel.Action
			cp.Close()
			return action, true
		}
		return "", true
	case tea.KeyUp:
		if len(cp.filtered) > 0 {
			cp.sel--
			if cp.sel < 0 {
				cp.sel = len(cp.filtered) - 1
			}
		}
		return "", true
	case tea.KeyDown:
		if len(cp.filtered) > 0 {
			cp.sel = (cp.sel + 1) % len(cp.filtered)
		}
		return "", true
	case tea.KeyTab:
		if sel := cp.Selected(); sel != nil {
			cp.input.SetValue(sel.Action + " ")
			cp.input.CursorEnd()
			cp.filter(cp.input.Value())
		}
		return "", true
	default:
		var cmd tea.Cmd
		cp.input, cmd = cp.input.Update(msg)
		_ = cmd
		cp.filter(cp.input.Value())
		cp.sel = 0
		return "", true
	}
}

// filter applies fuzzy search to the entries, using scored ranking for
// relevance. Entries are sorted by score so the best match is first.
func (cp *CommandPalette) filter(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		cp.filtered = cp.entries
		return
	}

	ranked := RankFuzzyResults(query, cp.entries)
	cp.filtered = make([]CommandPaletteEntry, 0, len(ranked))
	for _, r := range ranked {
		cp.filtered = append(cp.filtered, r.Entry)
	}
}

// fuzzySubsequence checks if query is a subsequence of target.
func fuzzySubsequence(query, target string) bool {
	qi := 0
	for ti := 0; ti < len(target) && qi < len(query); ti++ {
		if query[qi] == target[ti] {
			qi++
		}
	}
	return qi == len(query)
}

// Render renders the command palette as a string.
func (cp *CommandPalette) Render(viewWidth int) string {
	if !cp.open {
		return ""
	}

	maxVisible := 10
	if viewWidth < 60 {
		viewWidth = 60
	}
	boxWidth := viewWidth - 4
	if boxWidth > 70 {
		boxWidth = 70
	}

	var b strings.Builder

	// Title
	b.WriteString(paletteTitleStyle.Render("  Command Palette"))
	b.WriteString(paletteDimStyle.Render("  (Esc to close, Enter to run, Tab to edit)"))
	b.WriteString("\n\n")

	// Input
	cp.input.Width = boxWidth - 4
	b.WriteString(paletteInputStyle.Width(boxWidth - 2).Render(cp.input.View()))
	b.WriteString("\n\n")

	// Results
	if len(cp.filtered) == 0 {
		b.WriteString(paletteDimStyle.Render("  No matching commands"))
	} else {
		start := 0
		if cp.sel >= maxVisible {
			start = cp.sel - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(cp.filtered) {
			end = len(cp.filtered)
		}

		// Group by category
		currentCat := ""
		for i := start; i < end; i++ {
			e := cp.filtered[i]
			if e.Category != currentCat {
				currentCat = e.Category
				b.WriteString(paletteCategoryStyle.Render("  "+currentCat) + "\n")
			}

			line := fmt.Sprintf("  %-18s %s", e.Name, paletteDescStyle.Render(e.Description))
			if i == cp.sel {
				b.WriteString(paletteSelStyle.Width(boxWidth).Render(line))
			} else {
				b.WriteString(paletteItemStyle.Width(boxWidth).Render(line))
			}
			b.WriteString("\n")
		}

		// Scroll indicator
		if len(cp.filtered) > maxVisible {
			b.WriteString(paletteDimStyle.Render(fmt.Sprintf("  %d/%d results", cp.sel+1, len(cp.filtered))))
		}
	}

	return paletteBoxStyle.Width(boxWidth).Render(b.String())
}
