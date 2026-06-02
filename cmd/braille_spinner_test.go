package cmd

import (
	"strings"
	"testing"
)

func TestBrailleSpinner_Tick(t *testing.T) {
	s := NewBrailleSpinner(SpinnerBraille, "Thinking")
	f1 := s.Frame()
	f2 := s.Tick()
	if f1 == f2 {
		t.Error("expected different frames after tick")
	}
	if !strings.Contains(f1, hawkSpinnerANSI) {
		t.Error("expected colored spinner glyph")
	}
	if renderShimmer("Thinking", 0) == "" {
		t.Error("expected colored verb label")
	}
}

func TestBrailleSpinner_AllStyles(t *testing.T) {
	styles := []SpinnerStyle{
		SpinnerBraille, SpinnerBrailleWave, SpinnerHawk, SpinnerDNA,
		SpinnerScan, SpinnerPulse, SpinnerSnake, SpinnerOrbit,
	}
	for _, style := range styles {
		s := NewBrailleSpinner(style, "test")
		f := s.Frame()
		if f == "" {
			t.Errorf("style %s produced empty frame", style)
		}
	}
}

func TestBrailleSpinner_Random(t *testing.T) {
	s := NewBrailleSpinner(SpinnerRandom, "Loading")
	f := s.Frame()
	if f == "" {
		t.Error("random spinner produced empty frame")
	}
}

func TestRenderShimmer(t *testing.T) {
	result := renderShimmer("Hi", 0)
	if result == "" {
		t.Error("expected non-empty shimmer output")
	}
	if result == "Hi" {
		t.Error("expected ANSI-colored output, got plain text")
	}
	if !strings.Contains(result, "\033[") {
		t.Error("expected ANSI color codes")
	}
}

func TestHawkQuadBlock_Frames(t *testing.T) {
	if len(hawkQuadBlockGlyphs) != 4 {
		t.Fatalf("expected 4 QuadBlock glyphs, got %d", len(hawkQuadBlockGlyphs))
	}
	if hawkQuadBlockGlyphs[3] != "▙" {
		t.Fatalf("expected last QuadBlock frame ▙, got %q", hawkQuadBlockGlyphs[3])
	}
	s := NewBrailleSpinner(SpinnerHawk, "Working")
	f0 := s.Frame()
	if !strings.Contains(f0, "▛") {
		t.Fatalf("expected QuadBlock glyph, got %q", f0)
	}
	s.Tick()
	s.Tick()
	s.Tick()
	if !strings.Contains(s.Frame(), "▙") {
		t.Fatalf("expected frame cycle to reach ▙, got %q", s.Frame())
	}
}

func TestHawkRandomPalette(t *testing.T) {
	if len(hawkRandomPalette) != 20 {
		t.Fatalf("expected 20 hawk random colors, got %d", len(hawkRandomPalette))
	}
	for i, c := range hawkRandomPalette {
		if hawkColorIsOrange(c) {
			t.Fatalf("color %d %v should not be orange (reserved for hawk accent)", i, c)
		}
		if !hawkColorVisibleOnBG(c) {
			t.Fatalf("color %d %v too dim on dark background", i, c)
		}
	}
}

func hawkColorIsOrange(rgb [3]int) bool {
	r, g, b := rgb[0], rgb[1], rgb[2]
	return r > 180 && g < 160 && b < 100
}

func hawkColorVisibleOnBG(rgb [3]int) bool {
	bg := hawkSpinnerBG
	maxC := rgb[0]
	if rgb[1] > maxC {
		maxC = rgb[1]
	}
	if rgb[2] > maxC {
		maxC = rgb[2]
	}
	// Require strong channel and reasonable contrast vs bg (~30,30,40).
	if maxC < 165 {
		return false
	}
	dr := absInt(rgb[0] - bg[0])
	dg := absInt(rgb[1] - bg[1])
	db := absInt(rgb[2] - bg[2])
	return dr+dg+db >= 120
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func TestHawkRandomSolidLabel(t *testing.T) {
	s := NewBrailleSpinner(SpinnerHawk, "Crafting")
	// Default mode: solid color label (no wave).
	s.SetWave(false)
	f := s.Frame()
	// Solid label has at most two reset codes: one for glyph, one for label.
	if strings.Count(f, "\033[0m") > 2 {
		t.Errorf("expected solid label color, got mixed resets: %q", f)
	}
}

func TestHawkWaveLabel_MultipleColorsPerChar(t *testing.T) {
	s := NewBrailleSpinner(SpinnerHawk, "Crafting")
	s.SetWave(true)
	f := s.Frame()
	// Wave mode: each character of the verb gets its own foreground color,
	// so we expect N color codes for the N-char verb plus resets.
	if strings.Count(f, "\033[38;2;") < len("Crafting") {
		t.Errorf("expected per-character colors in wave mode, got %q", f)
	}
	// Wave frames must change as the spinner ticks (color phase shifts).
	f0 := f
	s.Tick()
	s.Tick()
	if s.Frame() == f0 {
		t.Error("expected wave frame to change across ticks")
	}
}

func TestHawkWaveAnimatedDots_PresentInFrame(t *testing.T) {
	s := NewBrailleSpinner(SpinnerHawk, "Crafting")
	s.SetWave(true)
	f := s.Frame()
	// Three dots (○ or ●) ride after the verb in wave mode.
	dots := strings.Count(f, "○") + strings.Count(f, "●")
	if dots != 3 {
		t.Errorf("expected 3 trailing dots in wave mode, got %d in %q", dots, f)
	}
	// Tick advances the highlighted dot position.
	idxBefore := s.dots
	s.Tick()
	if s.dots == idxBefore {
		t.Error("expected dot phase to advance on tick")
	}
}

func TestColorHawkRGB(t *testing.T) {
	got := colorHawkRGB([3]int{255, 94, 14}, "Hi")
	if strings.Contains(got, "\033[2m") {
		t.Fatal("expected full natural color, not dim")
	}
	if !strings.Contains(got, "38;2;255;94;14") {
		t.Fatalf("expected natural hawk orange, got %q", got)
	}
}
