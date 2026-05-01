package cmd

import (
	"strings"
	"testing"
	"time"
)

func TestAwaySummary_RecentActivity(t *testing.T) {
	msgs := make([]displayMsg, 10)
	for i := range msgs {
		msgs[i] = displayMsg{role: "user", content: "message"}
	}
	// Last activity was just now — should return empty
	result := awaySummary(msgs, time.Now())
	if result != "" {
		t.Fatalf("expected empty for recent activity, got %q", result)
	}
}

func TestAwaySummary_TooFewMessages(t *testing.T) {
	msgs := []displayMsg{
		{role: "user", content: "hello"},
		{role: "assistant", content: "hi"},
	}
	// Enough idle time but too few messages
	result := awaySummary(msgs, time.Now().Add(-10*time.Minute))
	if result != "" {
		t.Fatalf("expected empty for too few messages, got %q", result)
	}
}

func TestAwaySummary_ValidSummary(t *testing.T) {
	msgs := []displayMsg{
		{role: "user", content: "fix the build"},
		{role: "assistant", content: "I'll look into that"},
		{role: "tool_use", content: "Bash"},
		{role: "tool_result", content: "build succeeded"},
		{role: "user", content: "now add tests"},
		{role: "assistant", content: "Adding tests for the new module"},
	}
	result := awaySummary(msgs, time.Now().Add(-15*time.Minute))
	if result == "" {
		t.Fatal("expected non-empty summary")
	}
	if !strings.Contains(result, "away for") {
		t.Fatalf("expected 'away for' in summary, got %q", result)
	}
}

func TestUniqueStrings(t *testing.T) {
	input := []string{"Bash", "FileRead", "Bash", "Grep", "Bash"}
	got := uniqueStrings(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 unique strings, got %d: %v", len(got), got)
	}
}
