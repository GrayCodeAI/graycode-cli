package cmd

import (
	"fmt"
	"strings"
	"testing"
)

func TestSpinnerWave_AdvancesOnTick(t *testing.T) {
	s := NewBrailleSpinner(SpinnerHawk, "Hi")
	f1 := s.Frame()
	s.Tick()
	f2 := s.Frame()
	if f1 == f2 {
		t.Fatal("expected wave frame to change after tick")
	}
	if s.wavePhase == 0 {
		t.Fatal("expected wavePhase to advance")
	}
}

func TestSpinnerWave_TwentyColors(t *testing.T) {
	if len(spinnerWaveColors) != 20 {
		t.Fatalf("expected 20 wave colors, got %d", len(spinnerWaveColors))
	}
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		seen[ansiSpinnerWaveColor(i)] = true
	}
	if len(seen) != 20 {
		t.Fatalf("expected 20 distinct escapes, got %d", len(seen))
	}
}

func TestSpinnerWave_FlowsThroughGlyphVerbAndDots(t *testing.T) {
	out0 := renderSpinnerWaveLine("◐", "AB", 0, 0)
	out1 := renderSpinnerWaveLine("◐", "AB", 1, 0)
	if out0 == out1 {
		t.Fatal("expected wave head shift to change output")
	}
	if strings.Count(out0, iconDotFilled) != 1 {
		t.Fatalf("expected one filled dot, got %q", out0)
	}
	if !strings.Contains(out0, "◐") {
		t.Fatalf("expected spinner glyph in line, got %q", out0)
	}
}

func TestSpinnerWave_GlyphUsesWaveColor(t *testing.T) {
	s := NewBrailleSpinner(SpinnerHawk, "Go")
	f := s.Frame()
	if !strings.Contains(f, "◐") && !strings.Contains(f, "◓") && !strings.Contains(f, "◑") && !strings.Contains(f, "◒") {
		t.Fatalf("expected compass spinner glyph, got %q", f)
	}
	if !frameContainsSpinnerWave(f) {
		t.Fatal("expected wave color on spinner glyph")
	}
}

func fmtSpinnerWaveNeedle(index int) string {
	c := spinnerWaveColors[index%spinnerWaveLen]
	return fmt.Sprintf("38;2;%d;%d;%d", c[0], c[1], c[2])
}
