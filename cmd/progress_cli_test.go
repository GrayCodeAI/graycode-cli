package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// TestCLIProgressNonTTY asserts that piped/CI output is clean: one static line
// per completed step, no ANSI escapes, no carriage-return animation.
func TestCLIProgressNonTTY(t *testing.T) {
	var buf bytes.Buffer
	p := newCLIProgress("Review", []string{"Build", "Review", "Save"}, &buf, false)

	p.StartStep(0)
	p.CompleteStep(0)
	p.StartStep(1)
	p.CompleteStep(1)
	p.StartStep(2)
	p.CompleteStep(2)
	p.Done()

	out := buf.String()
	if strings.Contains(out, "\x1b") {
		t.Errorf("non-TTY output must not contain ANSI escapes, got: %q", out)
	}
	if strings.Contains(out, "\r") {
		t.Errorf("non-TTY output must not use carriage returns, got: %q", out)
	}
	for _, name := range []string{"Build", "Review", "Save"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected step %q in output, got: %q", name, out)
		}
	}
	// Each completed step should appear as its own line.
	if got := strings.Count(out, "\n"); got < 3 {
		t.Errorf("expected at least 3 completed-step lines, got %d in %q", got, out)
	}
}

// TestCLIProgressTTY asserts the terminal path redraws the current line with a
// leading carriage return and clears trailing glyphs before finalizing.
func TestCLIProgressTTY(t *testing.T) {
	var buf bytes.Buffer
	p := newCLIProgress("Review", []string{"Build", "Review"}, &buf, true)

	p.StartStep(0)
	p.CompleteStep(0)
	p.Done()

	out := buf.String()
	if !strings.Contains(out, "\r") {
		t.Errorf("TTY output should redraw with carriage returns, got: %q", out)
	}
	if !strings.Contains(out, "Build") {
		t.Errorf("expected step name in TTY output, got: %q", out)
	}
}

// TestCLIProgressFailStep marks a step failed and keeps other steps pending.
func TestCLIProgressFailStep(t *testing.T) {
	var buf bytes.Buffer
	p := newCLIProgress("Review", []string{"Build", "Review"}, &buf, false)

	p.StartStep(0)
	p.FailStep(0, "boom")

	if p.pt.Steps[0].Status != "failed" {
		t.Errorf("expected step 0 failed, got %q", p.pt.Steps[0].Status)
	}
	if p.pt.Steps[1].Status != "pending" {
		t.Errorf("expected step 1 still pending, got %q", p.pt.Steps[1].Status)
	}
	if !strings.Contains(buf.String(), "boom") {
		t.Errorf("expected fail reason in output, got: %q", buf.String())
	}
}

// TestCLIProgressTTYMultiStep runs multiple Start/Complete cycles on a TTY.
// Regression: the spinner is one-shot, so each step must get a fresh spinner
// rather than restarting a stopped one (which would panic on a double close).
func TestCLIProgressTTYMultiStep(t *testing.T) {
	var buf bytes.Buffer
	p := newCLIProgress("Review", []string{"Build", "Review", "Save"}, &buf, true)

	p.StartStep(0)
	p.CompleteStep(0)
	p.StartStep(1)
	p.CompleteStep(1)
	p.StartStep(2)
	p.CompleteStep(2)
	p.Done()

	out := buf.String()
	for _, name := range []string{"Build", "Review", "Save"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected step %q in TTY output, got: %q", name, out)
		}
	}
}
