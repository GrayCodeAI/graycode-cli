package cmd

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	reviewcontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/review"
	contracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/types"
)

// captureStdout runs fn with stdout redirected to a pipe and returns what was
// written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

func TestPrintReviewSummary_NoFindings(t *testing.T) {
	out := captureStdout(t, func() {
		printReviewSummary("0123456789abcdef", &reviewcontracts.Result{
			Stats: reviewcontracts.Stats{FilesReviewed: 3},
		})
	})
	if !strings.Contains(out, "no issues found") {
		t.Errorf("expected 'no issues found' summary, got: %q", out)
	}
	if !strings.Contains(out, "01234567") {
		t.Errorf("expected short SHA in summary, got: %q", out)
	}
}

func TestPrintReviewSummary_WithFindings(t *testing.T) {
	out := captureStdout(t, func() {
		printReviewSummary("0123456789abcdef", &reviewcontracts.Result{
			Findings: []reviewcontracts.Finding{
				{Severity: contracts.SeverityHigh, File: "main.go", Line: 10, Message: "SQL injection"},
				{Severity: contracts.SeverityLow, File: "config.go", Line: 2, Message: "naming"},
			},
		})
	})
	if !strings.Contains(out, "2 findings") {
		t.Errorf("expected '2 findings' in summary, got: %q", out)
	}
	if !strings.Contains(out, "main.go:10") {
		t.Errorf("expected file:line detail, got: %q", out)
	}
	if !strings.Contains(out, "high") {
		t.Errorf("expected max severity in summary, got: %q", out)
	}
}

func TestSilentErr_BackgroundSuppresses(t *testing.T) {
	old := reviewRunBackground
	reviewRunBackground = true
	defer func() { reviewRunBackground = old }()

	if err := silentErr(errors.New("boom"), "ctx"); err != nil {
		t.Errorf("silentErr in background mode = %v, want nil", err)
	}
}

func TestSilentErr_ForegroundWraps(t *testing.T) {
	old := reviewRunBackground
	reviewRunBackground = false
	defer func() { reviewRunBackground = old }()

	err := silentErr(errors.New("boom"), "context label")
	if err == nil {
		t.Fatal("silentErr in foreground mode returned nil")
	}
	if !strings.Contains(err.Error(), "context label") {
		t.Errorf("error should include context label, got: %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should preserve underlying message, got: %v", err)
	}
}
