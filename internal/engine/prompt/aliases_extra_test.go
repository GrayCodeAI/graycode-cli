package prompt

import (
	"testing"
)

func TestNewOptimizer(t *testing.T) {
	opt := NewOptimizer()
	if opt == nil {
		t.Fatal("expected non-nil Optimizer")
	}
}

func TestNewABTest(t *testing.T) {
	a := DSPyVariant{Section: "section-a", Content: "content-a"}
	b := DSPyVariant{Section: "section-b", Content: "content-b"}
	test := NewABTest(a, b)
	if test == nil {
		t.Fatal("expected non-nil ABTest")
	}
	if test.A.Section != "section-a" {
		t.Errorf("A.Section = %q, want %q", test.A.Section, "section-a")
	}
	if test.B.Section != "section-b" {
		t.Errorf("B.Section = %q, want %q", test.B.Section, "section-b")
	}
}

func TestNewTuner(t *testing.T) {
	tuner := NewTuner()
	if tuner == nil {
		t.Fatal("expected non-nil Tuner")
	}
}

func TestNewABTest_EmptyVariants(t *testing.T) {
	test := NewABTest(DSPyVariant{}, DSPyVariant{})
	if test == nil {
		t.Fatal("expected non-nil ABTest")
	}
}
