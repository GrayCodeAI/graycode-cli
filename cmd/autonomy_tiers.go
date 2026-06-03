package cmd

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/charmbracelet/lipgloss"
)

// Four container autonomy tiers (Inspect → Edit → Run → Trust).
var containerAutonomyTiers = []engine.AutonomyLevel{
	engine.AutonomyBasic,
	engine.AutonomySemi,
	engine.AutonomyFull,
	engine.AutonomyYOLO,
}

var containerAutonomyTierNames = []string{
	"Inspect",
	"Edit",
	"Run",
	"Trust",
}

// DefaultContainerAutonomy is the tier applied when the sandbox becomes ready.
const DefaultContainerAutonomy = engine.AutonomySemi

func autonomyTierName(level engine.AutonomyLevel) string {
	for i, l := range containerAutonomyTiers {
		if l == level {
			return containerAutonomyTierNames[i]
		}
	}
	return "Inspect"
}

func autonomyTierIndex(level engine.AutonomyLevel) int {
	for i, l := range containerAutonomyTiers {
		if l == level {
			return i
		}
	}
	return 1 // default Edit
}

func nextAutonomyTier(level engine.AutonomyLevel) engine.AutonomyLevel {
	return containerAutonomyTiers[(autonomyTierIndex(level)+1)%len(containerAutonomyTiers)]
}

// autonomyTierDescription is plain-language copy shown when the user changes tier (ctrl+L).
func autonomyTierDescription(level engine.AutonomyLevel) string {
	switch level {
	case engine.AutonomyBasic:
		return "Read and search the codebase only. File changes and terminal commands need your approval."
	case engine.AutonomySemi:
		return "Read and edit files automatically. Terminal commands need your approval."
	case engine.AutonomyFull:
		return "Read, edit, and run most terminal commands in the container. Risky commands still need your approval."
	case engine.AutonomyYOLO:
		return "Almost no approval prompts. Use only when you fully trust this session."
	default:
		return "Read and edit files automatically. Terminal commands need your approval."
	}
}

func autonomyTierColor(level engine.AutonomyLevel) lipgloss.Color {
	switch level {
	case engine.AutonomyBasic:
		return tierInspect
	case engine.AutonomySemi:
		return tierEdit
	case engine.AutonomyFull:
		return tierRun
	case engine.AutonomyYOLO:
		return tierTrust
	default:
		return tierEdit
	}
}

func autonomyTierStyle(level engine.AutonomyLevel) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(autonomyTierColor(level)).Bold(true).Inline(true)
}

func renderAutonomyTierLabel(level engine.AutonomyLevel) string {
	return autonomyTierStyle(level).Render(autonomyTierName(level))
}

func formatAutonomyTierMessage(level engine.AutonomyLevel) string {
	return fmt.Sprintf("Autonomy %s — %s", renderAutonomyTierLabel(level), autonomyTierDescription(level))
}

func formatSandboxReadyAutonomyMessage(level engine.AutonomyLevel) string {
	return fmt.Sprintf("Sandbox ready. Default tier %s — %s Press ctrl+L to switch Inspect · Edit · Run · Trust.",
		renderAutonomyTierLabel(level), autonomyTierDescription(level))
}

func autonomyLevelForTierName(name string) engine.AutonomyLevel {
	switch strings.TrimSpace(name) {
	case "Inspect":
		return engine.AutonomyBasic
	case "Edit":
		return engine.AutonomySemi
	case "Run":
		return engine.AutonomyFull
	case "Trust":
		return engine.AutonomyYOLO
	default:
		return DefaultContainerAutonomy
	}
}

// autonomyFromSettings maps settings.json autonomy (1–4) to a tier level.
// 0 or unset leaves session default until the container is ready.
func autonomyFromSettings(n int) engine.AutonomyLevel {
	switch n {
	case 1:
		return engine.AutonomyBasic
	case 2:
		return engine.AutonomySemi
	case 3:
		return engine.AutonomyFull
	case 4:
		return engine.AutonomyYOLO
	default:
		return 0
	}
}