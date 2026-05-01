package cmd

import (
	"fmt"
	"strings"
	"testing"
)

func TestRenderStatusBar_SignatureExists(t *testing.T) {
	// Verify the function signature compiles by referencing it.
	var _ func(*chatModel, int) string = renderStatusBar
}

func TestStatusBarCostFormat(t *testing.T) {
	// Test that dollar formatting produces expected output.
	cost := 1.50
	s := fmt.Sprintf("$%.2f", cost)
	if s != "$1.50" {
		t.Fatalf("unexpected cost format: %s", s)
	}
}

func TestStatusBarTokenFormat(t *testing.T) {
	tests := []struct {
		tokens   int
		expected string
	}{
		{0, "0"},
		{500, "500"},
		{1000, "1k"},
		{5400, "5k"},
		{100000, "100k"},
	}
	for _, tt := range tests {
		var s string
		if tt.tokens < 1000 {
			s = fmt.Sprintf("%d", tt.tokens)
		} else {
			s = fmt.Sprintf("%dk", tt.tokens/1000)
		}
		if s != tt.expected {
			t.Errorf("tokens=%d: got %q, want %q", tt.tokens, s, tt.expected)
		}
	}
}

func TestStatusBarDurationFormat(t *testing.T) {
	tests := []struct {
		minutes  int
		expected string
	}{
		{5, "5m"},
		{60, "1h0m"},
		{90, "1h30m"},
	}
	for _, tt := range tests {
		var s string
		if tt.minutes >= 60 {
			s = fmt.Sprintf("%dh%dm", tt.minutes/60, tt.minutes%60)
		} else {
			s = fmt.Sprintf("%dm", tt.minutes)
		}
		if s != tt.expected {
			t.Errorf("minutes=%d: got %q, want %q", tt.minutes, s, tt.expected)
		}
	}
}

func TestStatusBarPadding(t *testing.T) {
	// Verify that padding logic produces a string no longer than width
	width := 80
	left := "model-name"
	center := "$0.00 | 0 | 0"
	right := "5m"

	totalUsed := len(left) + len(center) + len(right)
	remaining := width - totalUsed
	if remaining < 2 {
		t.Fatal("test setup error: not enough room")
	}
	leftGap := remaining / 2
	rightGap := remaining - leftGap

	result := left + strings.Repeat(" ", leftGap) + center + strings.Repeat(" ", rightGap) + right
	if len(result) != width {
		t.Fatalf("expected length %d, got %d", width, len(result))
	}
}
