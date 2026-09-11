package cmd

import (
	"fmt"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

func TestAutonomyTierNames(t *testing.T) {
	if got := autonomyTierName(engine.AutonomyBasic); got != "Scout" {
		t.Fatalf("Basic = %q, want Scout", got)
	}
	if got := autonomyTierName(engine.AutonomySemi); got != "Builder" {
		t.Fatalf("Semi = %q, want Builder", got)
	}
	if got := autonomyTierName(engine.AutonomyFull); got != "Operator" {
		t.Fatalf("Full = %q, want Operator", got)
	}
	if got := autonomyTierName(engine.AutonomyYOLO); got != "Autonomous" {
		t.Fatalf("YOLO = %q, want Autonomous", got)
	}
}

func TestNextAutonomyTier(t *testing.T) {
	if nextAutonomyTier(engine.AutonomyYOLO) != engine.AutonomyBasic {
		t.Fatal("expected Autonomous -> Scout wrap")
	}
	if nextAutonomyTier(engine.AutonomySemi) != engine.AutonomyFull {
		t.Fatal("expected Builder -> Operator")
	}
}

func TestAutonomyFromSettings(t *testing.T) {
	if autonomyFromSettings(2) != engine.AutonomySemi {
		t.Fatal("settings autonomy 2 should map to Builder/Semi")
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
	seen := make(map[string]bool)
	for _, l := range levels {
		c := fmt.Sprint(autonomyTierColor(l))
		if seen[c] {
			t.Fatalf("duplicate color for tier %v", l)
		}
		seen[c] = true
	}
}
