package analytics

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLogEvent(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

	LogEvent("test_event", "session-123", map[string]interface{}{"key": "value"})

	// Verify file was created
	data, err := os.ReadFile(dir + "/.hawk/analytics/events.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected events to be logged")
	}
	if !strings.Contains(string(data), "test_event") {
		t.Fatal("expected event name in log")
	}
}

func TestSaveAndGetTraces(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

	trace := &SessionTrace{
		SessionID:    "test-session",
		StartTime:    time.Now(),
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		MessageCount: 10,
		ToolCalls:    3,
		CostUSD:      0.05,
	}

	if err := SaveTrace(trace); err != nil {
		t.Fatal(err)
	}

	traces, err := GetTraces()
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}
	if traces[0].SessionID != "test-session" {
		t.Fatalf("expected session ID 'test-session', got %q", traces[0].SessionID)
	}
	if traces[0].CostUSD != 0.05 {
		t.Fatalf("expected cost 0.05, got %f", traces[0].CostUSD)
	}
}

func TestSummary(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

	// Test empty summary
	summary := Summary()
	if summary != "No analytics data available." {
		t.Fatalf("unexpected summary: %q", summary)
	}

	// Add trace and check summary
	SaveTrace(&SessionTrace{
		SessionID:    "s1",
		Provider:     "anthropic",
		MessageCount: 5,
		ToolCalls:    2,
		CostUSD:      0.1,
	})

	summary = Summary()
	if !strings.Contains(summary, "Sessions: 1") {
		t.Fatalf("expected 'Sessions: 1' in summary, got: %q", summary)
	}
	if !strings.Contains(summary, "Total cost: $0.1000") {
		t.Fatalf("expected cost in summary, got: %q", summary)
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"", []string{}},
		{"single", []string{"single"}},
		{"a\n", []string{"a"}},
	}

	for _, tt := range tests {
		result := splitLines(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("splitLines(%q) = %v, want %v", tt.input, result, tt.expected)
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("splitLines(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
			}
		}
	}
}
