package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

func TestRenderContextUsageBar(t *testing.T) {
	bar := renderContextUsageBar(20, 50)
	if !strings.Contains(bar, "█") {
		t.Fatalf("expected filled segment, got %q", bar)
	}
	if !strings.Contains(bar, "░") {
		t.Fatalf("expected empty segment, got %q", bar)
	}
}

func TestContextUsagePercentForBar(t *testing.T) {
	sess := engine.NewSession("test", "test-model", "sys", nil)
	for i := 0; i < 20; i++ {
		sess.AddUser(strings.Repeat("word ", 200))
	}
	m := chatModel{session: sess}
	_, _, pct := m.contextUsagePercentForBar()
	if pct <= 0 {
		t.Fatalf("expected positive context bar fill, got %d", pct)
	}
}

func TestRenderCompactProgressPanel(t *testing.T) {
	m := chatModel{
		session:          &engine.Session{},
		brailleSpinner:   NewBrailleSpinner(SpinnerHawk, "Compacting conversation"),
		manualCompacting: true,
	}
	out := m.renderCompactProgressPanel(80)
	if !strings.Contains(out, "Summarizing conversation") {
		t.Fatalf("missing title: %q", out)
	}
	if !strings.Contains(out, "context ") {
		t.Fatalf("missing context label: %q", out)
	}
	if !strings.Contains(out, "esc cancel") {
		t.Fatalf("missing cancel hint: %q", out)
	}
	if !strings.Contains(out, "%") {
		t.Fatalf("missing percent: %q", out)
	}
}

func TestFormatContextPercentLabel(t *testing.T) {
	if got := formatContextPercentLabel(56, 128_000); got != "<1%" {
		t.Fatalf("small usage = %q, want <1%%", got)
	}
	if got := formatContextPercentLabel(50_000, 100_000); got != "50%" {
		t.Fatalf("half usage = %q, want 50%%", got)
	}
}