package cmd

import (
	"fmt"

	"github.com/GrayCodeAI/hawk/internal/engine"
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

func autonomyTierHint(level engine.AutonomyLevel) string {
	switch level {
	case engine.AutonomyBasic:
		return "read & search only"
	case engine.AutonomySemi:
		return "edits auto · shell asks"
	case engine.AutonomyFull:
		return "shell auto · risky asks"
	case engine.AutonomyYOLO:
		return "minimal prompts"
	default:
		return ""
	}
}

func formatAutonomyTierMessage(level engine.AutonomyLevel) string {
	name := autonomyTierName(level)
	if hint := autonomyTierHint(level); hint != "" {
		return fmt.Sprintf("Autonomy → %s (%s)", name, hint)
	}
	return fmt.Sprintf("Autonomy → %s", name)
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