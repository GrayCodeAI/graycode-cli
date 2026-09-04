package engine

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
)

func TestSession_SanitizeTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: ` "Fix authentication bug" `, want: "Fix authentication bug"},
		{input: `Title: Refactor database schema.`, want: "Refactor database schema"},
		{input: `title: Implement dark mode`, want: "Implement dark mode"},
		{input: "`Add telemetry exporter`", want: "Add telemetry exporter"},
		{input: "A very long title that exceeds the maximum allowable title length and should be truncated properly so it does not overflow UI headers or metadata fields", want: "A very long title that exceeds the maximum allowable title length and should be "},
	}

	for _, tc := range tests {
		got := sanitizeTitle(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeTitle(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSession_GenerateTitle_DeterministicFallback(t *testing.T) {
	reg := tool.NewRegistry()
	sess := NewSession("", "", "System prompt", reg)

	sess.AddUser("Fix memory leak in websocket listener")

	title, err := sess.GenerateTitle(context.Background())
	if err != nil {
		t.Fatalf("GenerateTitle failed: %v", err)
	}

	if title != "Fix memory leak in websocket listener" {
		t.Errorf("GenerateTitle() = %q, want 'Fix memory leak in websocket listener'", title)
	}

	// Verify title event in journal
	j := sess.Persistence().Journal()
	if j != nil {
		var found bool
		for _, ev := range j.Snapshot() {
			if ev.Type == eventlog.SessionTitle {
				if f, ok := ev.Data.(eventlog.SessionTitleFact); ok && f.Title == title {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("expected SessionTitleFact with %q in journal", title)
		}
	}
}
