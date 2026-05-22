package cmd

import (
	"strings"
	"testing"
	"time"
)

func TestRenderStatusBar_SignatureExists(t *testing.T) {
	var _ func(*chatModel, int) string = renderStatusBar
}

func TestShortenHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := shortenHomePath(home + "/project/hawk")
	if got != "~/project/hawk" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatSessionDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{3*time.Minute + 2*time.Second, "3m 2s"},
		{90 * time.Minute, "1h 30m"},
	}
	for _, tt := range tests {
		if got := formatSessionDuration(tt.d); got != tt.expected {
			t.Errorf("duration %v: got %q want %q", tt.d, got, tt.expected)
		}
	}
}

func TestFormatTokenCountWithCommas(t *testing.T) {
	if got := formatTokenCountWithCommas(14442); got != "14,442 tokens" {
		t.Fatalf("got %q", got)
	}
}

func TestPadStatusBarLine(t *testing.T) {
	left := "left"
	right := "right"
	result := padStatusBarLine(left, right, len(left), len(right), 20)
	if len(result) != 20 {
		t.Fatalf("expected width 20, got %d (%q)", len(result), result)
	}
	if !strings.HasPrefix(result, left) || !strings.HasSuffix(result, right) {
		t.Fatalf("unexpected layout: %q", result)
	}
}
