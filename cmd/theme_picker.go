package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	internaltheme "github.com/GrayCodeAI/hawk/internal/theme"
)

// detectAutoIsDark returns whether "auto" theme currently resolves to dark.
func detectAutoIsDark() bool {
	settings := hawkconfig.LoadGlobalSettings()
	return settings.Theme == "auto" || settings.Theme == "system" || internaltheme.DetectOSTheme() != "light"
}

// ThemeChoice represents a visual theme option.
type ThemeChoice struct {
	Name   string
	Desc   string
	IsDark bool
}

// buildThemeChoices pulls the full theme list from the internal registry so
// theme_picker.go never needs its own separate hardcoded slice.
func buildThemeChoices() []ThemeChoice {
	entries := internaltheme.ThemeByName()
	names := internaltheme.ThemeNames() // ordered by registry
	out := make([]ThemeChoice, 0, len(names)+1)
	for _, name := range names {
		e, ok := entries[name]
		if !ok {
			continue
		}
		kind := "dark"
		if !e.IsDark {
			kind = "light"
		}
		out = append(out, ThemeChoice{
			Name:   name,
			Desc:   fmt.Sprintf("%s (%s)", e.Label, kind),
			IsDark: e.IsDark,
		})
	}
	// Add "auto" at the end — follows OS appearance (system dark/light preference)
	out = append(out, ThemeChoice{Name: "auto", Desc: "Follow system appearance (auto dark/light)", IsDark: detectAutoIsDark()})
	return out
}

// ThemePicker is a lightweight overlay for choosing a theme.
// It pulls entries from the internal/theme registry so it always reflects the
// full set of registered palettes.
type ThemePicker struct {
	open    bool
	entries []ThemeChoice
	sel     int
}

// NewThemePicker creates a new theme picker backed by the internal registry.
func NewThemePicker() *ThemePicker {
	return &ThemePicker{
		entries: buildThemeChoices(),
	}
}

// Open opens the picker, pre-selecting the given theme name (defaults to 0).
func (tp *ThemePicker) Open() {
	tp.open = true
	tp.sel = 0
}

// OpenWithCurrent opens the picker pre-selecting the currently active theme.
func (tp *ThemePicker) OpenWithCurrent(current string) {
	tp.open = true
	tp.sel = 0
	for i, e := range tp.entries {
		if e.Name == current {
			tp.sel = i
			break
		}
	}
}

// Close closes the picker.
func (tp *ThemePicker) Close() {
	tp.open = false
}

// IsOpen returns whether the picker is visible.
func (tp *ThemePicker) IsOpen() bool {
	return tp.open
}

// Selected returns the currently highlighted entry, or nil.
func (tp *ThemePicker) Selected() *ThemeChoice {
	if tp.sel >= 0 && tp.sel < len(tp.entries) {
		return &tp.entries[tp.sel]
	}
	return nil
}

// Update handles key events. Returns (chosen, handled).
// chosen is non-nil only on the Enter keypress that commits the selection.
func (tp *ThemePicker) Update(msg tea.KeyMsg) (*ThemeChoice, bool) {
	if !tp.open {
		return nil, false
	}

	switch key := msg.Key(); key.Code {
	case tea.KeyEsc:
		tp.Close()
		return nil, true
	case 'c':
		// Ctrl+C closes the picker; other C keys fall through to text input.
		if key.Mod == tea.ModCtrl {
			tp.Close()
			return nil, true
		}
		return nil, false
	case tea.KeyEnter:
		sel := tp.Selected()
		tp.Close()
		return sel, true
	case tea.KeyUp:
		if len(tp.entries) > 0 {
			tp.sel--
			if tp.sel < 0 {
				tp.sel = len(tp.entries) - 1
			}
		}
		return nil, true
	case tea.KeyDown:
		if len(tp.entries) > 0 {
			tp.sel = (tp.sel + 1) % len(tp.entries)
		}
		return nil, true
	default:
		if msg.String() == "q" {
			tp.Close()
		}
		return nil, true
	}
}

// View renders the theme picker overlay.
func (tp *ThemePicker) View() tea.View {
	if !tp.open {
		return tea.NewView("")
	}

	titleStyle := lipgloss.NewStyle().
		Background(hawkColor).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))

	var b strings.Builder
	b.WriteString(titleStyle.Render(" Select Theme ") + "\n")
	b.WriteString(hintStyle.Render("  ↑/↓ navigate  •  Enter select  •  Esc cancel") + "\n\n")

	for i, e := range tp.entries {
		if i == tp.sel {
			rowStyle := lipgloss.NewStyle().Foreground(hawkColor).Bold(true)
			b.WriteString(fmt.Sprintf("  ▶ %s\n", rowStyle.Render(e.Name)))
			b.WriteString(fmt.Sprintf("    %s\n", lipgloss.NewStyle().Foreground(hawkColor).Render(e.Desc)))
		} else {
			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D0D0D0"))
			b.WriteString(fmt.Sprintf("    %s\n", nameStyle.Render(e.Name)))
			b.WriteString(fmt.Sprintf("    %s\n", dimStyle.Render(e.Desc)))
		}
	}

	// Add live preview for selected theme
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("  Preview:") + "\n")
	b.WriteString(renderThemePreview(tp.entries[tp.sel].Name) + "\n")

	v := tea.View{Content: b.String()}
	v.AltScreen = true
	return v
}

// renderThemePreview renders a visual preview of the selected theme.
func renderThemePreview(themeName string) string {
	var preview strings.Builder

	// Handle auto theme specially
	if themeName == "auto" {
		preview.WriteString(fmt.Sprintf("  Panel:   %s dark\n", lipgloss.NewStyle().Background(lipgloss.Color("#0e0e10")).Render("    ")))
		preview.WriteString(fmt.Sprintf("  Accent:  %s orange\n", lipgloss.NewStyle().Background(lipgloss.Color("#FF5E0E")).Render("    ")))
		return preview.String()
	}

	entry, ok := internaltheme.LookupTheme(themeName)
	if !ok {
		return "  Theme not found"
	}
	p := entry.Palette

	if p.Panel != "" {
		preview.WriteString(fmt.Sprintf("  Panel:   %s\n", lipgloss.NewStyle().Background(lipgloss.Color(p.Panel)).Render("    ")))
	}
	if p.PromptBg != "" {
		preview.WriteString(fmt.Sprintf("  Prompt:  %s\n", lipgloss.NewStyle().Background(lipgloss.Color(p.PromptBg)).Render("    ")))
	}
	if p.Accent != "" {
		preview.WriteString(fmt.Sprintf("  Accent:  %s\n", lipgloss.NewStyle().Background(lipgloss.Color(p.Accent)).Render("    ")))
	}
	if p.Ink != "" {
		preview.WriteString(fmt.Sprintf("  Text:    %s\n", lipgloss.NewStyle().Background(lipgloss.Color(p.Ink)).Render("    ")))
	}
	if p.Green != "" {
		preview.WriteString(fmt.Sprintf("  Green:   %s\n", lipgloss.NewStyle().Background(lipgloss.Color(p.Green)).Render("    ")))
	}
	if p.Red != "" {
		preview.WriteString(fmt.Sprintf("  Red:     %s\n", lipgloss.NewStyle().Background(lipgloss.Color(p.Red)).Render("    ")))
	}

	return preview.String()
}
