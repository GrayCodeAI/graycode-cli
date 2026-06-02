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
	// Glyph is hawk brand orange.
	if !strings.Contains(f1, ansiOrange) {
		t.Error("expected orange spinner glyph")
	}
	// Label is bright green.
	if !strings.Contains(f1, ansiGreen) {
		t.Error("expected green verb label")
	}
	// Filled dot is bright yellow.
	if !strings.Contains(f1, ansiYellow) {
		t.Error("expected yellow filled dot")
	}
}

func TestBrailleSpinner_AllStyles(t *testing.T) {
	styles := []SpinnerStyle{
		SpinnerBraille, SpinnerBrailleWave, SpinnerHawk, SpinnerDNA,
		SpinnerScan, SpinnerPulse, SpinnerSnake, SpinnerOrbit,
		SpinnerWing, SpinnerTalons,
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

func TestHawkAnimatedDots_PresentInFrame(t *testing.T) {
	s := NewBrailleSpinner(SpinnerHawk, "Crafting")
	f := s.Frame()
	// Three plain circles ride after the verb: one bold cyan, two dim.
	total := strings.Count(f, "●") + strings.Count(f, "○")
	if total != hawkTypingDots {
		t.Errorf("expected %d trailing circle-dots, got %d in %q", hawkTypingDots, total, f)
	}
	// Tick advances the highlighted dot position.
	idxBefore := s.dots
	s.Tick()
	if s.dots == idxBefore {
		t.Error("expected dot phase to advance on tick")
	}
}
