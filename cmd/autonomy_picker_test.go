package cmd

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

func TestAutonomyPicker_HasAllFiveTiers(t *testing.T) {
	ap := NewAutonomyPicker(80)
	if len(ap.entries) != 5 {
		t.Fatalf("expected 5 tiers, got %d", len(ap.entries))
	}
	wantOrder := []engine.AutonomyLevel{
		engine.AutonomySupervised, engine.AutonomyBasic, engine.AutonomySemi,
		engine.AutonomyFull, engine.AutonomyYOLO,
	}
	for i, level := range wantOrder {
		if ap.entries[i].Level != level {
			t.Errorf("entry %d = %v, want %v", i, ap.entries[i].Level, level)
		}
	}
}

func TestAutonomyPicker_OpenPreselectsCurrentTier(t *testing.T) {
	ap := NewAutonomyPicker(80)
	ap.Open(engine.AutonomyFull)
	if !ap.IsOpen() {
		t.Fatal("expected picker to be open")
	}
	sel := ap.Selected()
	if sel == nil || sel.Level != engine.AutonomyFull {
		t.Fatalf("expected preselected tier AutonomyFull, got %+v", sel)
	}
}

func TestAutonomyPicker_EnterSelectsAndCloses(t *testing.T) {
	ap := NewAutonomyPicker(80)
	ap.Open(engine.AutonomySupervised)

	chosen, handled := ap.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if !handled || chosen != nil {
		t.Fatalf("KeyDown should navigate, not select: chosen=%v handled=%v", chosen, handled)
	}
	if ap.Selected().Level != engine.AutonomyBasic {
		t.Fatalf("expected selection to move to AutonomyBasic, got %v", ap.Selected().Level)
	}

	chosen, handled = ap.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("expected Enter to be handled")
	}
	if chosen == nil || chosen.Level != engine.AutonomyBasic {
		t.Fatalf("expected chosen AutonomyBasic, got %v", chosen)
	}
	if ap.IsOpen() {
		t.Error("expected picker to close after Enter")
	}
}

func TestAutonomyPicker_EscClosesWithoutSelecting(t *testing.T) {
	ap := NewAutonomyPicker(80)
	ap.Open(engine.AutonomyBasic)

	chosen, handled := ap.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled || chosen != nil {
		t.Fatalf("expected Esc to close without a selection, got chosen=%v", chosen)
	}
	if ap.IsOpen() {
		t.Error("expected picker to be closed after Esc")
	}
}

func TestAutonomyPicker_FilterByName(t *testing.T) {
	ap := NewAutonomyPicker(80)
	ap.Open(engine.AutonomyBasic)

	for _, r := range "scout" {
		ap.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if len(ap.filtered) != 1 || ap.filtered[0].Level != engine.AutonomyBasic {
		t.Fatalf("expected filter 'scout' to match only Basic tier, got %+v", ap.filtered)
	}
}
