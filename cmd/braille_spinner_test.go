package cmd

import "testing"

func TestBrailleSpinner_Tick(t *testing.T) {
	s := NewBrailleSpinner(SpinnerBraille, "Thinking")
	f1 := s.Frame()
	f2 := s.Tick()
	if f1 == f2 {
		t.Error("expected different frames after tick")
	}
}

func TestBrailleSpinner_AllStyles(t *testing.T) {
	styles := []SpinnerStyle{
		SpinnerBraille, SpinnerBrailleWave, SpinnerDNA,
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
}
