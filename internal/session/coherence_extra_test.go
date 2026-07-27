package session

import (
	"strings"
	"testing"
)

func TestCoherenceTracker_RecordPivot(t *testing.T) {
	ct := NewCoherenceTracker(10, 5)
	ct.RecordPivot(1, "task1", "task2")

	state := ct.GetState()
	if len(state.Pivots) != 1 {
		t.Fatalf("expected 1 pivot, got %d", len(state.Pivots))
	}
	if state.Pivots[0].Turn != 1 {
		t.Errorf("Turn = %d, want 1", state.Pivots[0].Turn)
	}
	if state.Pivots[0].From != "task1" {
		t.Errorf("From = %q, want %q", state.Pivots[0].From, "task1")
	}
	if state.Pivots[0].To != "task2" {
		t.Errorf("To = %q, want %q", state.Pivots[0].To, "task2")
	}
}

func TestCoherenceTracker_RecordPivot_TruncatesToFive(t *testing.T) {
	ct := NewCoherenceTracker(10, 5)
	for i := 0; i < 10; i++ {
		ct.RecordPivot(i, "from", "to")
	}

	state := ct.GetState()
	if len(state.Pivots) != 5 {
		t.Errorf("expected 5 pivots (truncated), got %d", len(state.Pivots))
	}
}

func TestCoherenceTracker_GetState(t *testing.T) {
	ct := NewCoherenceTracker(10, 5)
	state := ct.GetState()
	if len(state.Threads) != 0 {
		t.Error("expected empty threads")
	}
}

func TestCoherenceTracker_FormatForPrompt_Empty(t *testing.T) {
	ct := NewCoherenceTracker(10, 5)
	result := ct.FormatForPrompt()
	if result != "" {
		t.Errorf("FormatForPrompt() on empty tracker = %q, want empty", result)
	}
}

func TestCoherenceTracker_FormatForPrompt_WithIntentSummary(t *testing.T) {
	ct := NewCoherenceTracker(10, 5)
	ct.state.IntentSummary = "doing something"
	result := ct.FormatForPrompt()
	if result == "" {
		t.Error("expected non-empty result with intent summary")
	}
	if !strings.Contains(result, "Session context:") {
		t.Errorf("result should contain 'Session context:', got %q", result)
	}
}

func TestCoherenceTracker_FormatForPrompt_WithActiveThread(t *testing.T) {
	ct := NewCoherenceTracker(10, 5)
	ct.state.Threads = []*SessionThread{
		{ID: "thread1", Status: "active", Topic: "coding"},
	}
	result := ct.FormatForPrompt()
	if result == "" {
		t.Error("expected non-empty result with active thread")
	}
	if !strings.Contains(result, "Session context:") {
		t.Errorf("result should contain 'Session context:', got %q", result)
	}
}

func TestCoherenceTracker_FormatForPrompt_WithInactiveThread(t *testing.T) {
	ct := NewCoherenceTracker(10, 5)
	ct.state.Threads = []*SessionThread{
		{ID: "thread1", Status: "inactive", Topic: "coding"},
	}
	result := ct.FormatForPrompt()
	if result != "" {
		t.Errorf("FormatForPrompt() with only inactive threads = %q, want empty", result)
	}
}

func TestMatchesAny(t *testing.T) {
	tests := []struct {
		text     string
		pattern  string
		expected bool
	}{
		{"hello world", "hello", true},
		{"hello world", "goodbye", false},
		{"test123", `\d+`, true},
		{"no numbers", `\d+`, false},
		{"case insensitive", `(?i)CASE`, true},
	}

	for _, tt := range tests {
		result := matchesAny(tt.text, tt.pattern)
		if result != tt.expected {
			t.Errorf("matchesAny(%q, %q) = %v, want %v", tt.text, tt.pattern, result, tt.expected)
		}
	}
}

func TestMatchesAny_InvalidRegex(t *testing.T) {
	result := matchesAny("test", "[invalid")
	if result {
		t.Error("matchesAny with invalid regex should return false")
	}
}
