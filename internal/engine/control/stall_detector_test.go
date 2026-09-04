package control

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

func TestNewStallDetector(t *testing.T) {
	sd := NewStallDetector()
	if sd.WindowSize != 10 {
		t.Errorf("expected WindowSize=10, got %d", sd.WindowSize)
	}
	if sd.SoftThreshold != 3 {
		t.Errorf("expected SoftThreshold=3, got %d", sd.SoftThreshold)
	}
	if sd.HardThreshold != 5 {
		t.Errorf("expected HardThreshold=5, got %d", sd.HardThreshold)
	}
	if len(sd.Window) != 0 {
		t.Errorf("expected empty window, got %d entries", len(sd.Window))
	}
}

func TestStallDetectorNoStall(t *testing.T) {
	sd := NewStallDetector()
	sd.Record("Read", map[string]interface{}{"path": "a.go"}, "content a")
	sd.Record("Read", map[string]interface{}{"path": "b.go"}, "content b")
	sd.Record("Edit", map[string]interface{}{"path": "a.go", "content": "new"}, "ok")

	result := sd.Check()
	if result.IsStalled {
		t.Error("should not detect stall with different tool calls")
	}
	if result.Level != "none" {
		t.Errorf("expected level 'none', got %q", result.Level)
	}
}

func TestStallDetectorSoftThreshold(t *testing.T) {
	sd := NewStallDetector()
	args := map[string]interface{}{"path": "auth.go"}

	// Record same tool+args 3 times (soft threshold).
	for i := 0; i < 3; i++ {
		sd.Record("Edit", args, "error: permission denied")
	}

	result := sd.Check()
	if !result.IsStalled {
		t.Error("expected stall at soft threshold")
	}
	if result.Level != "soft" {
		t.Errorf("expected level 'soft', got %q", result.Level)
	}
	if result.RepeatCount < 3 {
		t.Errorf("expected RepeatCount >= 3, got %d", result.RepeatCount)
	}
}

func TestStallDetectorHardThreshold(t *testing.T) {
	sd := NewStallDetector()
	args := map[string]interface{}{"path": "auth.go"}

	// Record same tool+args 5 times (hard threshold).
	for i := 0; i < 5; i++ {
		sd.Record("Edit", args, "error: permission denied")
	}

	result := sd.Check()
	if !result.IsStalled {
		t.Error("expected stall at hard threshold")
	}
	if result.Level != "hard" {
		t.Errorf("expected level 'hard', got %q", result.Level)
	}
	if result.RepeatCount < 5 {
		t.Errorf("expected RepeatCount >= 5, got %d", result.RepeatCount)
	}
}

func TestStallDetectorOscillation(t *testing.T) {
	sd := NewStallDetector()
	argsA := map[string]interface{}{"path": "a.go"}
	argsB := map[string]interface{}{"path": "b.go"}

	// A→B→A→B pattern.
	sd.Record("Edit", argsA, "ok")
	sd.Record("Read", argsB, "content")
	sd.Record("Edit", argsA, "ok")
	sd.Record("Read", argsB, "content")

	result := sd.Check()
	if !result.IsStalled {
		t.Error("expected oscillation detection")
	}
	if !strings.Contains(result.Pattern, "oscillation") {
		t.Errorf("expected pattern to mention oscillation, got %q", result.Pattern)
	}
}

func TestStallDetectorOscillationNotTriggeredForSame(t *testing.T) {
	sd := NewStallDetector()
	args := map[string]interface{}{"path": "a.go"}

	// Same call repeated is repetition, not oscillation.
	sd.Record("Edit", args, "ok")
	sd.Record("Edit", args, "ok")
	sd.Record("Edit", args, "ok")
	sd.Record("Edit", args, "ok")

	if sd.DetectOscillation() {
		t.Error("same repeated call should not be detected as oscillation")
	}
}

func TestStallDetectorErrorLoop(t *testing.T) {
	sd := NewStallDetector()
	sameError := "error: file not found"

	// Same output 3 times from different tools.
	sd.Record("Read", map[string]interface{}{"path": "x.go"}, sameError)
	sd.Record("Edit", map[string]interface{}{"path": "x.go", "content": "a"}, sameError)
	sd.Record("Write", map[string]interface{}{"path": "x.go", "data": "b"}, sameError)

	if !sd.DetectErrorLoop() {
		t.Error("expected error loop detection when same output appears 3 times")
	}
}

func TestStallDetectorErrorLoopNotTriggeredBelowThreshold(t *testing.T) {
	sd := NewStallDetector()

	sd.Record("Read", map[string]interface{}{"path": "x.go"}, "error: not found")
	sd.Record("Edit", map[string]interface{}{"path": "x.go"}, "error: not found")

	if sd.DetectErrorLoop() {
		t.Error("should not detect error loop with only 2 identical outputs")
	}
}

func TestStallDetectorWindowSliding(t *testing.T) {
	sd := NewStallDetector()
	sd.WindowSize = 5
	args := map[string]interface{}{"path": "a.go"}

	// Fill with 3 repeats (triggers soft).
	for i := 0; i < 3; i++ {
		sd.Record("Edit", args, "error")
	}
	result := sd.Check()
	if !result.IsStalled {
		t.Error("expected stall with 3 repeats")
	}

	// Push old entries out with different calls.
	for i := 0; i < 5; i++ {
		sd.Record("Read", map[string]interface{}{"path": strings.Repeat("x", i+1)}, fmt.Sprintf("output %d", i))
	}

	result = sd.Check()
	if result.IsStalled {
		t.Error("should not detect stall after window slides past repeated entries")
	}
}

func TestStallDetectorReset(t *testing.T) {
	sd := NewStallDetector()
	args := map[string]interface{}{"path": "a.go"}

	for i := 0; i < 5; i++ {
		sd.Record("Edit", args, "error")
	}

	result := sd.Check()
	if !result.IsStalled {
		t.Fatal("precondition: should be stalled")
	}

	sd.Reset()

	result = sd.Check()
	if result.IsStalled {
		t.Error("should not be stalled after reset")
	}
	if len(sd.Window) != 0 {
		t.Errorf("expected empty window after reset, got %d entries", len(sd.Window))
	}
}

func TestStallDetectorBuildEscalation(t *testing.T) {
	sd := NewStallDetector()

	// No stall.
	result := &StallResult{IsStalled: false, Level: "none"}
	msg := sd.BuildEscalation(result)
	if msg != "" {
		t.Errorf("expected empty message for no stall, got %q", msg)
	}

	// Soft stall.
	result = &StallResult{
		IsStalled:   true,
		Level:       "soft",
		RepeatCount: 3,
		Pattern:     "Edit (same args) repeated 3 times consecutively",
		Suggestion:  "Try reading the file first",
	}
	msg = sd.BuildEscalation(result)
	if !strings.Contains(msg, icons.Alert()+" Stall detected") {
		t.Errorf("expected soft warning marker, got %q", msg)
	}
	if !strings.Contains(msg, "repeated 3 times") {
		t.Errorf("expected repeat count in message, got %q", msg)
	}
	if !strings.Contains(msg, "Suggestions:") {
		t.Errorf("expected suggestions section, got %q", msg)
	}

	// Hard stall.
	result = &StallResult{
		IsStalled:   true,
		Level:       "hard",
		RepeatCount: 5,
		Pattern:     "Edit (same args) repeated 5 times consecutively",
		Suggestion:  "Stop and ask user",
	}
	msg = sd.BuildEscalation(result)
	if !strings.Contains(msg, "HARD STALL DETECTED") {
		t.Errorf("expected hard stall marker, got %q", msg)
	}
}

func TestStallDetectorBuildEscalationNil(t *testing.T) {
	sd := NewStallDetector()
	msg := sd.BuildEscalation(nil)
	if msg != "" {
		t.Errorf("expected empty for nil result, got %q", msg)
	}
}

func TestStallDetectorFormatWindow(t *testing.T) {
	sd := NewStallDetector()

	// Empty window.
	out := sd.FormatWindow()
	if out != "(empty window)" {
		t.Errorf("expected empty marker, got %q", out)
	}

	// With entries.
	sd.Record("Read", map[string]interface{}{"path": "a.go"}, "content")
	sd.Record("Edit", map[string]interface{}{"path": "b.go"}, "ok")

	out = sd.FormatWindow()
	if !strings.Contains(out, "Stall Window (2/10 entries)") {
		t.Errorf("expected window header, got %q", out)
	}
	if !strings.Contains(out, "Read") {
		t.Errorf("expected Read tool name in output, got %q", out)
	}
	if !strings.Contains(out, "Edit") {
		t.Errorf("expected Edit tool name in output, got %q", out)
	}
}

func TestStallDetectorConcurrency(t *testing.T) {
	sd := NewStallDetector()
	done := make(chan struct{})

	// Concurrent writers.
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			args := map[string]interface{}{"i": n}
			sd.Record("Tool", args, "output")
		}(i)
	}

	// Concurrent readers.
	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			sd.Check()
			sd.FormatWindow()
		}()
	}

	// Wait for all goroutines.
	for i := 0; i < 15; i++ {
		<-done
	}

	// Verify no panic and window is bounded.
	if len(sd.Window) > sd.WindowSize {
		t.Errorf("window exceeded max size: %d > %d", len(sd.Window), sd.WindowSize)
	}
}

func TestStallDetectorDetectRepetitionNonConsecutive(t *testing.T) {
	sd := NewStallDetector()
	sd.WindowSize = 6
	args := map[string]interface{}{"path": "a.go"}
	otherArgs := map[string]interface{}{"path": "b.go"}

	// Interleave same call with a different one: A, B, A, B, A (3 of A in window of 5).
	sd.Record("Edit", args, "error")
	sd.Record("Read", otherArgs, "content")
	sd.Record("Edit", args, "error")
	sd.Record("Read", otherArgs, "content2")
	sd.Record("Edit", args, "error")

	count, pattern := sd.DetectRepetition()
	if count < 3 {
		t.Errorf("expected count >= 3 for non-consecutive repeats, got %d (pattern: %s)", count, pattern)
	}
}

func TestHashArgsDeterministic(t *testing.T) {
	args := map[string]interface{}{
		"path":    "file.go",
		"content": "hello",
	}
	h1 := hashArgs(args)
	h2 := hashArgs(args)
	if h1 != h2 {
		t.Error("hashArgs should be deterministic for the same input")
	}
}

func TestHashArgsNil(t *testing.T) {
	h := hashArgs(nil)
	if h == "" {
		t.Error("hashArgs(nil) should return a valid hash")
	}
}
