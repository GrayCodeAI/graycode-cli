package cmd

import (
	"testing"

		lipgloss "charm.land/lipgloss/v2"
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

func TestApplyDefaultHostAutonomy_SetsBasicWhenUnset(t *testing.T) {
	sess := engine.NewSession("", "test-model", "you are helpful", nil)
	applyDefaultHostAutonomy(sess)
	if got := sess.PermSvc().Autonomy(); got != DefaultHostAutonomy {
		t.Fatalf("got %v, want %v", got, DefaultHostAutonomy)
	}
}

func TestApplyDefaultHostAutonomy_DoesNotClobberExplicitSetting(t *testing.T) {
	sess := engine.NewSession("", "test-model", "you are helpful", nil)
	sess.PermSvc().SetAutonomy(engine.AutonomyFull)
	applyDefaultHostAutonomy(sess)
	if got := sess.PermSvc().Autonomy(); got != engine.AutonomyFull {
		t.Fatalf("got %v, want AutonomyFull preserved", got)
	}
}

func TestAutonomyTierColorsDistinct(t *testing.T) {
	levels := []engine.AutonomyLevel{
		engine.AutonomyBasic,
		engine.AutonomySemi,
		engine.AutonomyFull,
		engine.AutonomyYOLO,
	}
	seen := make(map[lipgloss.Style]bool)
	for _, l := range levels {
		c := autonomyTierColor(l)
		if seen[c] {
			t.Fatalf("duplicate color for tier %v", l)
		}
		seen[c] = true
	}
}
