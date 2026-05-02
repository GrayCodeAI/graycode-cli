package engine

import (
	"fmt"
	"testing"
)

func TestLoopDetectorNoLoop(t *testing.T) {
	ld := NewLoopDetector(10, 4)
	ld.RecordStep([]string{"Read"}, []string{`{"path":"a.go"}`}, []string{"content a"})
	ld.RecordStep([]string{"Read"}, []string{`{"path":"b.go"}`}, []string{"content b"})
	ld.RecordStep([]string{"Edit"}, []string{`{"path":"a.go"}`}, []string{"ok"})
	if ld.IsLooping() {
		t.Error("should not detect loop with different steps")
	}
}

func TestLoopDetectorDetectsLoop(t *testing.T) {
	ld := NewLoopDetector(10, 3)
	for i := 0; i < 5; i++ {
		ld.RecordStep([]string{"Read"}, []string{`{"path":"a.go"}`}, []string{"same content"})
	}
	if !ld.IsLooping() {
		t.Error("should detect loop with 5 identical steps (threshold 3)")
	}
}

func TestLoopDetectorWindowSliding(t *testing.T) {
	ld := NewLoopDetector(5, 4)
	// Fill window with identical steps.
	for i := 0; i < 4; i++ {
		ld.RecordStep([]string{"Read"}, []string{`{"path":"a.go"}`}, []string{"same"})
	}
	if !ld.IsLooping() {
		t.Error("should detect loop at threshold")
	}
	// Push old entries out of window with different steps.
	for i := 0; i < 5; i++ {
		ld.RecordStep([]string{"Write"}, []string{fmt.Sprintf(`{"path":"file%d.go"}`, i)}, []string{fmt.Sprintf("output%d", i)})
	}
	if ld.IsLooping() {
		t.Error("should not detect loop after window slides past")
	}
}

func TestLoopDetectorWarning(t *testing.T) {
	ld := NewLoopDetector(10, 4)
	msg := ld.LoopWarning()
	if msg == "" {
		t.Error("expected non-empty warning")
	}
}
