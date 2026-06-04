package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/charmbracelet/lipgloss"
)

func TestRenderStatusBar_SignatureExists(t *testing.T) {
	var _ func(*chatModel, int) string = renderStatusBar
}

func TestRenderStatusBarRight_IncludesTokensLabel(t *testing.T) {
	m := &chatModel{session: &engine.Session{}}
	m.session.Cost.PromptTokens = 1200
	m.session.Cost.CompletionTokens = 300
	got := renderStatusBarRight(m)
	if !strings.Contains(got, "tokens") {
		t.Fatalf("expected tokens label in footer right, got %q", got)
	}
	if !strings.Contains(got, "●") {
		t.Fatalf("expected bullet prefix, got %q", got)
	}
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

func TestLayoutFooterRow_NoWrap(t *testing.T) {
	left := "left"
	right := "right"
	result := layoutFooterRow(left, right, 20)
	if lipgloss.Width(result) > 20 {
		t.Fatalf("expected visual width <= 20, got %q", result)
	}
	if !strings.HasPrefix(result, left) || !strings.HasSuffix(result, right) {
		t.Fatalf("unexpected layout: %q", result)
	}
}

func TestLayoutFooterRow_TruncatesOverflow(t *testing.T) {
	long := strings.Repeat("x", 80)
	result := layoutFooterRow("ok", long, 40)
	if lipgloss.Width(result) > 40 {
		t.Fatalf("footer row overflow: width=%d", lipgloss.Width(result))
	}
}
