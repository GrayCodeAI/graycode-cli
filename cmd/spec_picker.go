package cmd

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// specPickerAction identifies what a SpecPicker entry does when selected.
type specPickerAction int

const (
	specActionStart specPickerAction = iota
	specActionStatus
	specActionEdit
	specActionResume
	specActionArchive
	specActionConfigure
	specActionReset
)

// specPickerEntry is one selectable row in the picker.
type specPickerEntry struct {
	Action      specPickerAction
	Name        string
	Description string
}

var specPickerEntries = []specPickerEntry{
	{specActionStart, "Start", "Begin the workflow — writes spec.md, then plan.md, then tasks.md"},
	{specActionStatus, "Status", "Show the current stage"},
	{specActionEdit, "Edit", "Edit the active spec's artifacts (spec.md, plan.md, tasks.md)"},
	{specActionResume, "Resume", "Resume from the current stage — continue where you left off"},
	{specActionArchive, "Archive", "Archive a completed spec when implementation is done"},
	{specActionConfigure, "Configure", "Set language, framework, methodology, architecture preferences"},
	{specActionReset, "Reset", "Drop back to None — Write/Edit/Bash follow the trust tier again"},
}

// SpecPicker is a /spec quick-select overlay, modeled on AutonomyPicker's
// interaction pattern (arrow keys navigate, Enter selects, Esc dismisses,
// type to filter). Unlike AutonomyPicker, entries are actions rather than
// tiers — the spec workflow is a linear stage the model advances through
// via tool calls, not something the user jumps to directly.
type SpecPicker struct {
	open     bool
	input    textinput.Model
	entries  []specPickerEntry
	filtered []specPickerEntry
	sel      int
	width    int
	stage    engine.SpecStage
}

// NewSpecPicker creates a new spec workflow picker.
func NewSpecPicker(width int) *SpecPicker {
	ti := textinput.New()
	ti.Placeholder = "Type to filter…"
	ti.Focus()
	ti.CharLimit = 40
	ti.Width = 40

	return &SpecPicker{
		input:    ti,
		width:    width,
		entries:  specPickerEntries,
		filtered: specPickerEntries,
	}
}

// Open opens the picker, recording the current stage for the header line.
func (sp *SpecPicker) Open(stage engine.SpecStage) {
	sp.open = true
	sp.input.SetValue("")
	sp.input.Focus()
	sp.filtered = sp.entries
	sp.sel = 0
	sp.stage = stage
}

// Close closes the picker.
func (sp *SpecPicker) Close() {
	sp.open = false
	sp.input.SetValue("")
	sp.sel = 0
}

// IsOpen returns whether the picker is open.
func (sp *SpecPicker) IsOpen() bool {
	return sp.open
}

// Selected returns the currently highlighted entry, or nil.
func (sp *SpecPicker) Selected() *specPickerEntry {
	if sp.sel >= 0 && sp.sel < len(sp.filtered) {
		return &sp.filtered[sp.sel]
	}
	return nil
}

// Update handles key events. Returns (chosen, handled) — chosen is non-nil
// only on the keypress that commits a selection.
func (sp *SpecPicker) Update(msg tea.KeyMsg) (*specPickerEntry, bool) {
	if !sp.open {
		return nil, false
	}

	switch msg.Type {
	case tea.KeyEsc:
		sp.Close()
		return nil, true
	case tea.KeyEnter:
		sel := sp.Selected()
		sp.Close()
		return sel, true
	case tea.KeyUp:
		if len(sp.filtered) > 0 {
			sp.sel--
			if sp.sel < 0 {
				sp.sel = len(sp.filtered) - 1
			}
		}
		return nil, true
	case tea.KeyDown:
		if len(sp.filtered) > 0 {
			sp.sel = (sp.sel + 1) % len(sp.filtered)
		}
		return nil, true
	default:
		var cmd tea.Cmd
		sp.input, cmd = sp.input.Update(msg)
		_ = cmd
		sp.filter(sp.input.Value())
		sp.sel = 0
		return nil, true
	}
}

func (sp *SpecPicker) filter(query string) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		sp.filtered = sp.entries
		return
	}
	filtered := make([]specPickerEntry, 0, len(sp.entries))
	for _, e := range sp.entries {
		if strings.Contains(strings.ToLower(e.Name), query) || strings.Contains(strings.ToLower(e.Description), query) {
			filtered = append(filtered, e)
		}
	}
	sp.filtered = filtered
}

// Render renders the picker as a string, styled to match AutonomyPicker/CommandPalette.
func (sp *SpecPicker) Render(viewWidth int) string {
	if !sp.open {
		return ""
	}

	if viewWidth < 60 {
		viewWidth = 60
	}
	boxWidth := viewWidth - 4
	if boxWidth > 78 {
		boxWidth = 78
	}

	var b strings.Builder
	b.WriteString(paletteTitleStyle.Render("  Spec"))
	b.WriteString(paletteDimStyle.Render("  (↑↓ navigate · Enter select · Esc dismiss)"))
	b.WriteString("\n")
	b.WriteString(paletteDimStyle.Render(fmt.Sprintf("  Current stage: %s", specStageDisplayName(sp.stage))))
	b.WriteString("\n\n")

	sp.input.Width = boxWidth - 4
	b.WriteString(paletteInputStyle.Width(boxWidth - 2).Render(sp.input.View()))
	b.WriteString("\n\n")

	if len(sp.filtered) == 0 {
		b.WriteString(paletteDimStyle.Render("  No matching actions"))
	} else {
		nameWidth := 0
		for _, e := range sp.filtered {
			if len(e.Name) > nameWidth {
				nameWidth = len(e.Name)
			}
		}
		for i, e := range sp.filtered {
			line := "  " + lipgloss.NewStyle().Bold(true).Render(padRight(e.Name, nameWidth)) + "  " + e.Description
			if i == sp.sel {
				b.WriteString(paletteSelStyle.Width(boxWidth).Render("  " + padRight(e.Name, nameWidth) + "  " + e.Description))
			} else {
				b.WriteString(paletteItemStyle.Width(boxWidth).Render(line))
			}
			b.WriteString("\n")
		}
	}

	return paletteBoxStyle.Width(boxWidth).Render(strings.TrimRight(b.String(), "\n"))
}

// specStageDisplayName mirrors specStageLabel but takes a stage value
// directly, since the picker tracks the stage snapshot from when it opened
// rather than a *engine.Session.
func specStageDisplayName(stage engine.SpecStage) string {
	switch stage {
	case engine.SpecStageSpecify:
		return "Specify"
	case engine.SpecStagePlan:
		return "Plan"
	case engine.SpecStageTasks:
		return "Tasks"
	case engine.SpecStageImplementing:
		return "Implementing"
	default:
		return "None"
	}
}
