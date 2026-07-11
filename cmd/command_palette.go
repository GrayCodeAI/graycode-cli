package cmd

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
			tea "charm.land/bubbletea/v2"
		lipgloss "charm.land/lipgloss/v2"
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
	ti.SetWidth(40)

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
	commands := slashCommands()
	entries := make([]CommandPaletteEntry, 0, len(commands))
	for _, name := range commands {
		desc := slashCommandDescription(name)
		entries = append(entries, CommandPaletteEntry{
			Name:        name,
			Description: desc,
			Category:    slashCommandCategory(name),
			Action:      name,
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}

func slashCommandDescription(name string) string {
	if desc := slashDescriptions[name]; desc != "" {
		return desc
	}
	cmdName := strings.TrimPrefix(name, "/")
	if cmd, ok := subcommandRegistry.Lookup(cmdName); ok {
		return cmd.Description()
	}
	return "Run " + name
}

func slashCommandCategory(name string) string {
	switch name {
	case "/help", "/model", "/config", "/quit", "/exit", "/clear", "/compact", "/undo", "/snapshot", "/recover", "/new", "/copy", "/welcome":
		return "Core"
	case "/review", "/commit", "/test", "/lint", "/diff", "/status", "/audit", "/security-review", "/check", "/bughunter", "/hunt", "/ultrareview":
		return "Workflow"
	case "/agents", "/agents-init", "/mission", "/exec", "/research", "/loop", "/council", "/dream", "/investigate", "/vibe":
		return "Agent"
	case "/memory", "/context", "/ctx", "/search", "/history", "/session", "/sessions", "/export", "/share", "/fork", "/branches", "/branch":
		return "Memory"
	case "/tools", "/mcp", "/plugin", "/plugins", "/skills", "/files", "/image", "/render", "/yaad", "/ecosystem", "/path":
		return "Tools"
	case "/doctor", "/cost", "/usage", "/metrics", "/stats", "/integrity", "/stale", "/tokens", "/provider-status":
		return "Diagnostics"
	case "/autonomy", "/spec", "/vim", "/theme", "/color", "/mouse", "/select", "/focus", "/follow", "/output-style", "/statusline", "/keybindings", "/voice", "/remote-env", "/refresh-model-catalog":
		return "Settings"
	default:
		return "Other"
	}
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

	switch key := msg.Key(); key.Code {
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
	cp.input.SetWidth(boxWidth - 4)
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
