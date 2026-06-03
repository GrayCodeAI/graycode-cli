package cmd

import (
	"testing"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/charmbracelet/lipgloss"
)

func TestAutonomyTierNames(t *testing.T) {
	if got := autonomyTierName(engine.AutonomyBasic); got != "Inspect" {
		t.Fatalf("Basic = %q, want Inspect", got)
	}
	if got := autonomyTierName(engine.AutonomySemi); got != "Edit" {
		t.Fatalf("Semi = %q, want Edit", got)
	}
	if got := autonomyTierName(engine.AutonomyFull); got != "Run" {
		t.Fatalf("Full = %q, want Run", got)
	}
	if got := autonomyTierName(engine.AutonomyYOLO); got != "Trust" {
		t.Fatalf("YOLO = %q, want Trust", got)
	}
}

func TestNextAutonomyTier(t *testing.T) {
	if nextAutonomyTier(engine.AutonomyYOLO) != engine.AutonomyBasic {
		t.Fatal("expected Trust → Inspect wrap")
	}
	if nextAutonomyTier(engine.AutonomySemi) != engine.AutonomyFull {
		t.Fatal("expected Edit → Run")
	}
}

func TestAutonomyFromSettings(t *testing.T) {
	if autonomyFromSettings(2) != engine.AutonomySemi {
		t.Fatal("settings autonomy 2 should map to Edit/Semi")
	}
	if autonomyFromSettings(0) != 0 {
		t.Fatal("settings 0 should leave unset")
	}
}

func TestAutonomyTierColorsDistinct(t *testing.T) {
	levels := []engine.AutonomyLevel{
		engine.AutonomyBasic,
		engine.AutonomySemi,
		engine.AutonomyFull,
		engine.AutonomyYOLO,
	}
	seen := make(map[lipgloss.Color]bool)
	for _, l := range levels {
		c := autonomyTierColor(l)
		if seen[c] {
			t.Fatalf("duplicate color for tier %v", l)
		}
		seen[c] = true
	}
}