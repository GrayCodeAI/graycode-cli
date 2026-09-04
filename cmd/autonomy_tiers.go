package cmd

import (
	"fmt"
	"image/color"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

// Five container autonomy tiers (Scout → Builder → Operator → Autonomous → Always Ask).
// Supervised ("Always Ask") is included in the Ctrl+L cycle but requires a
// deliberate double-press to land on (see chat_update.go ctrl+l handling) so
// repeated key-presses can't accidentally drop the user into max-friction mode.
var containerAutonomyTiers = []engine.AutonomyLevel{
	engine.AutonomyBasic,
	engine.AutonomySemi,
	engine.AutonomyFull,
	engine.AutonomyYOLO,
	engine.AutonomySupervised,
}

var containerAutonomyTierNames = []string{
	"Scout",
	"Builder",
	"Operator",
	"Autonomous",
	"Always Ask",
}

// DefaultContainerAutonomy is the tier applied when the Docker container becomes ready.
const DefaultContainerAutonomy = engine.AutonomySemi

// yoloConfirmToken is the exact string a user must type (case-insensitive) to
// confirm entry into YOLO ("Autonomous") unattended mode via the picker.
const yoloConfirmToken = "continue"

func autonomyTierName(level engine.AutonomyLevel) string {
	if level == engine.AutonomySupervised {
		return "Always Ask"
	}
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

// nextAutonomyTier returns the next tier in the Ctrl+L cycle. It skips
// Supervised ("Always Ask") — repeated Ctrl+L wraps YOLO → Basic. Use
// nextAutonomyTierIncludingSupervised when the user explicitly confirms they
// want the cautious tier.
func nextAutonomyTier(level engine.AutonomyLevel) engine.AutonomyLevel {
	idx := autonomyTierIndex(level)
	for {
		idx = (idx + 1) % len(containerAutonomyTiers)
		if containerAutonomyTiers[idx] != engine.AutonomySupervised {
			return containerAutonomyTiers[idx]
		}
	}
}

// nextAutonomyTierIncludingSupervised returns the next tier with Supervised
// included in the cycle (used after the user confirms via double-press).
func nextAutonomyTierIncludingSupervised(level engine.AutonomyLevel) engine.AutonomyLevel {
	return containerAutonomyTiers[(autonomyTierIndex(level)+1)%len(containerAutonomyTiers)]
}

// isSupervisedPending reports whether the next regular cycle step would land
// on Supervised (i.e. the current tier is YOLO). The UI uses this to prompt
// for confirmation.
func isSupervisedPending(level engine.AutonomyLevel) bool {
	return level == engine.AutonomyYOLO
}

// autonomyTierDescription is short copy shown when the user changes tier (ctrl+L).
func autonomyTierDescription(level engine.AutonomyLevel) string {
	switch level {
	case engine.AutonomySupervised:
		return "Prompts for permission on every tool call"
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

func autonomyTierColor(level engine.AutonomyLevel) color.Color {
	switch level {
	case engine.AutonomySupervised:
		return lipgloss.Color("#9E9E9E") // matches textMuted's dark value
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
