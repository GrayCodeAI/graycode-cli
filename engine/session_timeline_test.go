package engine

import (
	"strings"
	"testing"
	"time"
)

func TestNewTimeline(t *testing.T) {
	tl := NewTimeline("sess-123")
	if tl.SessionID != "sess-123" {
		t.Fatalf("expected session ID 'sess-123', got %q", tl.SessionID)
	}
	if len(tl.Events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(tl.Events))
	}
	if tl.StartTime.IsZero() {
		t.Fatal("expected non-zero start time")
	}
}

func TestAddEvent(t *testing.T) {
	tl := NewTimeline("test")
	meta := map[string]string{"key": "value"}
	tl.AddEvent("user_input", "Fix the auth bug", meta)

	if len(tl.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tl.Events))
	}
	e := tl.Events[0]
	if e.Type != "user_input" {
		t.Errorf("expected type 'user_input', got %q", e.Type)
	}
	if e.Content != "Fix the auth bug" {
		t.Errorf("expected content 'Fix the auth bug', got %q", e.Content)
	}
	if e.Metadata["key"] != "value" {
		t.Errorf("expected metadata key=value, got %q", e.Metadata["key"])
	}
	if e.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestAddAction(t *testing.T) {
	tl := NewTimeline("test")
	tl.AddAction("Read", "src/auth/token.go", 100*time.Millisecond)

	if len(tl.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tl.Events))
	}
	e := tl.Events[0]
	if e.Type != "action" {
		t.Errorf("expected type 'action', got %q", e.Type)
	}
	if e.Metadata["tool"] != "Read" {
		t.Errorf("expected tool 'Read', got %q", e.Metadata["tool"])
	}
	if e.Metadata["target"] != "src/auth/token.go" {
		t.Errorf("expected target 'src/auth/token.go', got %q", e.Metadata["target"])
	}
	if e.Duration != 100*time.Millisecond {
		t.Errorf("expected 100ms duration, got %v", e.Duration)
	}
}

func TestAddDecision(t *testing.T) {
	tl := NewTimeline("test")
	tl.AddDecision("nil check missing at L42", "ValidateToken returns nil on empty input")

	if len(tl.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tl.Events))
	}
	e := tl.Events[0]
	if e.Type != "decision" {
		t.Errorf("expected type 'decision', got %q", e.Type)
	}
	if e.Content != "nil check missing at L42" {
		t.Errorf("unexpected content: %q", e.Content)
	}
	if e.Metadata["reason"] != "ValidateToken returns nil on empty input" {
		t.Errorf("unexpected reason: %q", e.Metadata["reason"])
	}
}

func TestAddMilestone(t *testing.T) {
	tl := NewTimeline("test")
	tl.AddMilestone("All tests passing")

	if len(tl.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tl.Events))
	}
	e := tl.Events[0]
	if e.Type != "milestone" {
		t.Errorf("expected type 'milestone', got %q", e.Type)
	}
	if e.Content != "All tests passing" {
		t.Errorf("unexpected content: %q", e.Content)
	}
}

func TestAddError(t *testing.T) {
	tl := NewTimeline("test")
	tl.AddError("connection timeout")

	if len(tl.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tl.Events))
	}
	e := tl.Events[0]
	if e.Type != "error" {
		t.Errorf("expected type 'error', got %q", e.Type)
	}
	if e.Content != "connection timeout" {
		t.Errorf("unexpected content: %q", e.Content)
	}
}

func TestAddFileChange(t *testing.T) {
	tl := NewTimeline("test")
	tl.AddFileChange("src/auth/token.go", "modify")

	if len(tl.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tl.Events))
	}
	e := tl.Events[0]
	if e.Type != "file_change" {
		t.Errorf("expected type 'file_change', got %q", e.Type)
	}
	if e.Metadata["path"] != "src/auth/token.go" {
		t.Errorf("unexpected path: %q", e.Metadata["path"])
	}
	if e.Metadata["action"] != "modify" {
		t.Errorf("unexpected action: %q", e.Metadata["action"])
	}
}

func TestGetByType(t *testing.T) {
	tl := NewTimeline("test")
	tl.AddAction("Read", "a.go", time.Millisecond)
	tl.AddAction("Grep", "pattern", 2*time.Millisecond)
	tl.AddDecision("fix needed", "reason")
	tl.AddMilestone("done")
	tl.AddError("oops")

	actions := tl.GetByType("action")
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}

	decisions := tl.GetByType("decision")
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}

	milestones := tl.GetByType("milestone")
	if len(milestones) != 1 {
		t.Fatalf("expected 1 milestone, got %d", len(milestones))
	}

	errors := tl.GetByType("error")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}

	empty := tl.GetByType("nonexistent")
	if len(empty) != 0 {
		t.Fatalf("expected 0 events for unknown type, got %d", len(empty))
	}
}

func TestGetBetween(t *testing.T) {
	tl := NewTimeline("test")
	now := time.Now()
	tl.StartTime = now

	// Manually add events with controlled timestamps.
	tl.mu.Lock()
	tl.Events = append(tl.Events, TimelineEvent{
		ID:        "e1",
		Timestamp: now.Add(1 * time.Second),
		Type:      "action",
		Content:   "first",
	})
	tl.Events = append(tl.Events, TimelineEvent{
		ID:        "e2",
		Timestamp: now.Add(5 * time.Second),
		Type:      "action",
		Content:   "second",
	})
	tl.Events = append(tl.Events, TimelineEvent{
		ID:        "e3",
		Timestamp: now.Add(10 * time.Second),
		Type:      "action",
		Content:   "third",
	})
	tl.mu.Unlock()

	// Query for events between 2s and 7s.
	results := tl.GetBetween(now.Add(2*time.Second), now.Add(7*time.Second))
	if len(results) != 1 {
		t.Fatalf("expected 1 event between 2s-7s, got %d", len(results))
	}
	if results[0].Content != "second" {
		t.Errorf("expected 'second', got %q", results[0].Content)
	}

	// Query for all events.
	all := tl.GetBetween(now, now.Add(11*time.Second))
	if len(all) != 3 {
		t.Fatalf("expected 3 events, got %d", len(all))
	}

	// Exact boundary match.
	exact := tl.GetBetween(now.Add(5*time.Second), now.Add(5*time.Second))
	if len(exact) != 1 {
		t.Fatalf("expected 1 event at exact boundary, got %d", len(exact))
	}
}

func TestGetMilestones(t *testing.T) {
	tl := NewTimeline("test")
	tl.AddAction("Read", "a.go", time.Millisecond)
	tl.AddMilestone("first milestone")
	tl.AddAction("Write", "b.go", time.Millisecond)
	tl.AddMilestone("second milestone")

	milestones := tl.GetMilestones()
	if len(milestones) != 2 {
		t.Fatalf("expected 2 milestones, got %d", len(milestones))
	}
	if milestones[0].Content != "first milestone" {
		t.Errorf("unexpected first milestone: %q", milestones[0].Content)
	}
	if milestones[1].Content != "second milestone" {
		t.Errorf("unexpected second milestone: %q", milestones[1].Content)
	}
}

func TestDuration(t *testing.T) {
	tl := NewTimeline("test")
	now := time.Now()
	tl.StartTime = now

	// Empty timeline returns time since start.
	d := tl.Duration()
	if d < 0 {
		t.Errorf("expected non-negative duration, got %v", d)
	}

	// With events, duration is last event minus start.
	tl.mu.Lock()
	tl.Events = append(tl.Events, TimelineEvent{
		ID:        "e1",
		Timestamp: now.Add(5 * time.Second),
		Type:      "action",
		Content:   "something",
	})
	tl.mu.Unlock()

	d = tl.Duration()
	if d != 5*time.Second {
		t.Errorf("expected 5s duration, got %v", d)
	}
}

func TestRenderTimeline(t *testing.T) {
	tl := NewTimeline("test-render")
	now := time.Now()
	tl.StartTime = now

	// Manually populate events with controlled timestamps.
	tl.mu.Lock()
	tl.Events = []TimelineEvent{
		{
			ID:        "e1",
			Timestamp: now,
			Type:      "user_input",
			Content:   "Fix the auth bug",
		},
		{
			ID:        "e2",
			Timestamp: now.Add(2 * time.Second),
			Type:      "action",
			Content:   "Read src/auth/token.go",
			Duration:  100 * time.Millisecond,
			Metadata:  map[string]string{"tool": "Read", "target": "src/auth/token.go"},
		},
		{
			ID:        "e3",
			Timestamp: now.Add(5 * time.Second),
			Type:      "decision",
			Content:   "nil check missing at L42",
			Metadata:  map[string]string{"reason": "returns nil"},
		},
		{
			ID:        "e4",
			Timestamp: now.Add(8 * time.Second),
			Type:      "milestone",
			Content:   "All tests passing",
		},
	}
	tl.mu.Unlock()

	rendered := tl.RenderTimeline(80)

	// Verify key elements are present.
	if !strings.Contains(rendered, "Session Timeline") {
		t.Error("missing 'Session Timeline' header")
	}
	if !strings.Contains(rendered, "Fix the auth bug") {
		t.Error("missing user input content")
	}
	if !strings.Contains(rendered, "Read") {
		t.Error("missing action tool")
	}
	if !strings.Contains(rendered, "Decision:") {
		t.Error("missing decision")
	}
	if !strings.Contains(rendered, "Milestone:") {
		t.Error("missing milestone")
	}
	if !strings.Contains(rendered, "Actions: 1") {
		t.Error("missing action count")
	}
	if !strings.Contains(rendered, "Decisions: 1") {
		t.Error("missing decision count")
	}
	if !strings.Contains(rendered, "0.1s") {
		t.Error("missing duration annotation for action")
	}
	if !strings.Contains(rendered, "═") {
		t.Error("missing separator")
	}
}

func TestRenderTimelineMinWidth(t *testing.T) {
	tl := NewTimeline("test")
	tl.AddMilestone("done")

	// Even with very small maxWidth, it should not panic.
	rendered := tl.RenderTimeline(10)
	if rendered == "" {
		t.Error("expected non-empty render output")
	}
}

func TestSummarize(t *testing.T) {
	tl := NewTimeline("sess-abc")
	now := time.Now()
	tl.StartTime = now

	tl.mu.Lock()
	tl.Events = []TimelineEvent{
		{ID: "e1", Timestamp: now, Type: "user_input", Content: "fix bug"},
		{ID: "e2", Timestamp: now.Add(time.Second), Type: "action", Content: "Read a.go"},
		{ID: "e3", Timestamp: now.Add(2 * time.Second), Type: "action", Content: "Grep pattern"},
		{ID: "e4", Timestamp: now.Add(3 * time.Second), Type: "decision", Content: "found issue"},
		{ID: "e5", Timestamp: now.Add(4 * time.Second), Type: "file_change", Content: "modify a.go"},
		{ID: "e6", Timestamp: now.Add(5 * time.Second), Type: "milestone", Content: "Bug fixed"},
	}
	tl.mu.Unlock()

	summary := tl.Summarize()

	if !strings.Contains(summary, "sess-abc") {
		t.Error("summary should contain session ID")
	}
	if !strings.Contains(summary, "6 total events") {
		t.Error("summary should contain event count")
	}
	if !strings.Contains(summary, "2 actions") {
		t.Error("summary should mention actions")
	}
	if !strings.Contains(summary, "1 decisions") {
		t.Error("summary should mention decisions")
	}
	if !strings.Contains(summary, "1 files were changed") {
		t.Error("summary should mention file changes")
	}
	if !strings.Contains(summary, "Bug fixed") {
		t.Error("summary should contain final milestone")
	}
}

func TestSummarizeEmpty(t *testing.T) {
	tl := NewTimeline("empty")
	summary := tl.Summarize()
	if summary != "Empty session with no recorded events." {
		t.Errorf("unexpected empty summary: %q", summary)
	}
}

func TestConcurrentAccess(t *testing.T) {
	tl := NewTimeline("concurrent")
	done := make(chan struct{})

	// Writer goroutine.
	go func() {
		for i := 0; i < 100; i++ {
			tl.AddAction("Read", "file.go", time.Millisecond)
			tl.AddDecision("decision", "reason")
			tl.AddMilestone("milestone")
			tl.AddError("error")
			tl.AddFileChange("file.go", "modify")
			tl.AddEvent("user_input", "test", nil)
		}
		close(done)
	}()

	// Reader goroutine reads concurrently.
	for i := 0; i < 50; i++ {
		_ = tl.GetByType("action")
		_ = tl.GetMilestones()
		_ = tl.Duration()
		_ = tl.RenderTimeline(80)
		_ = tl.Summarize()
	}

	<-done

	// Verify all events were recorded.
	if len(tl.Events) != 600 {
		t.Errorf("expected 600 events, got %d", len(tl.Events))
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{500 * time.Millisecond, "0s"},
		{5 * time.Second, "5s"},
		{60 * time.Second, "1m"},
		{90 * time.Second, "1m 30s"},
		{12*time.Minute + 30*time.Second, "12m 30s"},
	}

	for _, tt := range tests {
		got := timelineFmtDuration(tt.d)
		if got != tt.want {
			t.Errorf("timelineFmtDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatShortDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{50 * time.Millisecond, "50ms"},
		{100 * time.Millisecond, "0.1s"},
		{3200 * time.Millisecond, "3.2s"},
	}

	for _, tt := range tests {
		got := timelineFmtShortDuration(tt.d)
		if got != tt.want {
			t.Errorf("timelineFmtShortDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestCapitalizeFirst(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"create", "Create"},
		{"modify", "Modify"},
		{"delete", "Delete"},
		{"A", "A"},
	}

	for _, tt := range tests {
		got := timelineCapitalizeFirst(tt.input)
		if got != tt.want {
			t.Errorf("timelineCapitalizeFirst(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEventIcon(t *testing.T) {
	icons := map[string]string{
		"action":      "\U0001f50d",
		"decision":    "\U0001f4a1",
		"milestone":   "✅",
		"error":       "❌",
		"user_input":  "\U0001f4dd",
		"file_change": "✏️",
		"unknown":     "•",
	}

	for eventType, expected := range icons {
		got := timelineEventIcon(eventType)
		if got != expected {
			t.Errorf("timelineEventIcon(%q) = %q, want %q", eventType, got, expected)
		}
	}
}
