package cmd

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/charmbracelet/lipgloss"
)

// Four container autonomy tiers (Scout → Builder → Operator → Autonomous).
var containerAutonomyTiers = []engine.AutonomyLevel{
	engine.AutonomyBasic,
	engine.AutonomySemi,
	engine.AutonomyFull,
	engine.AutonomyYOLO,
}

var containerAutonomyTierNames = []string{
	"Scout",
	"Builder",
	"Operator",
	"Autonomous",
}

// DefaultContainerAutonomy is the tier applied when the sandbox becomes ready.
const DefaultContainerAutonomy = engine.AutonomySemi

func autonomyTierName(level engine.AutonomyLevel) string {
	for i, l := range containerAutonomyTiers {
		if l == level {
			return containerAutonomyTierNames[i]
		}
	}
	return "Builder"
}

func autonomyTierIndex(level engine.AutonomyLevel) int {
	for i, l := range containerAutonomyTiers {
		if l == level {
			return i
		}
	}
	return 1 // default Builder
}

func nextAutonomyTier(level engine.AutonomyLevel) engine.AutonomyLevel {
	return containerAutonomyTiers[(autonomyTierIndex(level)+1)%len(containerAutonomyTiers)]
}

// autonomyTierDescription is short copy shown when the user changes tier (ctrl+L).
func autonomyTierDescription(level engine.AutonomyLevel) string {
	switch level {
	case engine.AutonomyBasic:
		return "Explore only — edits and commands ask first"
	case engine.AutonomySemi:
		return "File changes auto-approve — commands ask first"
	case engine.AutonomyFull:
		return "Commands auto-run — risky actions ask first"
	case engine.AutonomyYOLO:
		return "Minimal prompts — only the highest-risk actions stop"
	default:
		return "File changes auto-approve — commands ask first"
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

func autonomyLevelForTierName(name string) engine.AutonomyLevel {
	switch strings.TrimSpace(name) {
	case "Scout":
		return engine.AutonomyBasic
	case "Builder":
		return engine.AutonomySemi
	case "Operator":
		return engine.AutonomyFull
	case "Autonomous":
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
