package cmd

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/mattn/go-runewidth"
)

// TestPadRight_MultibyteDisplayWidth verifies that padRight pads to a target
// DISPLAY width (via runewidth), not byte length — so multi-byte names align
// correctly in the picker columns.
func TestPadRight_MultibyteDisplayWidth(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
	}{
		{"ascii", "hello", 10},
		{"cjk", "你好", 10}, // 2 CJK chars = 4 display cells
		{"mixed", "a你b", 12},
		{"exact", "hello", 5},
		{"overflow", "hello world", 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := padRight(tt.input, tt.width)
			if runewidth.StringWidth(tt.input) >= tt.width {
				if got != tt.input {
					t.Errorf("padRight(%q, %d) = %q, want unchanged", tt.input, tt.width, got)
				}
				return
			}
			if gotW := runewidth.StringWidth(got); gotW != tt.width {
				t.Errorf("padRight(%q, %d) display width = %d, want %d", tt.input, tt.width, gotW, tt.width)
			}
		})
	}
}

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
