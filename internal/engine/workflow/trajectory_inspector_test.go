package workflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewTrajectoryInspector(t *testing.T) {
	ti := NewTrajectoryInspector("session-abc123")
	if ti.SessionID != "session-abc123" {
		t.Errorf("expected session ID session-abc123, got %s", ti.SessionID)
	}
	if len(ti.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(ti.Events))
	}
	if ti.StartTime.IsZero() {
		t.Error("expected non-zero start time")
	}
}

func TestTrajectoryInspectorRecord(t *testing.T) {
	ti := NewTrajectoryInspector("session-1")
	ti.Record("thought", "Need to fix the auth bug", "", 0, 100)
	ti.Record("action", "src/auth.go", "Read", 200*time.Millisecond, 50)
	ti.Record("observation", "Found nil check missing", "", 0, 75)

	if len(ti.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(ti.Events))
	}

	e := ti.Events[0]
	if e.Index != 0 {
		t.Errorf("expected index 0, got %d", e.Index)
	}
	if e.Type != "thought" {
		t.Errorf("expected type thought, got %s", e.Type)
	}
	if e.Content != "Need to fix the auth bug" {
		t.Errorf("unexpected content: %s", e.Content)
	}
	if e.Tokens != 100 {
		t.Errorf("expected 100 tokens, got %d", e.Tokens)
	}

	e2 := ti.Events[1]
	if e2.ToolName != "Read" {
		t.Errorf("expected tool name Read, got %s", e2.ToolName)
	}
	if e2.Duration != 200*time.Millisecond {
		t.Errorf("expected duration 200ms, got %v", e2.Duration)
	}
}

func TestTrajectoryInspectorRenderTimeline(t *testing.T) {
	ti := NewTrajectoryInspector("session-abc123")
	// Override start time for predictable output.
	ti.StartTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Record events with controlled timestamps.
	ti.mu.Lock()
	ti.Events = []TrajectoryEvent{
		{Index: 0, Type: "thought", Content: "Need to fix the auth bug", Timestamp: ti.StartTime},
		{Index: 1, Type: "action", Content: "src/auth.go", ToolName: "Read", Timestamp: ti.StartTime.Add(100 * time.Millisecond)},
		{Index: 2, Type: "observation", Content: "Found nil check missing at L42", Timestamp: ti.StartTime.Add(300 * time.Millisecond)},
		{Index: 3, Type: "decision", Content: "Bug fixed, tests passing", Timestamp: ti.StartTime.Add(5300 * time.Millisecond)},
	}
	ti.mu.Unlock()

	timeline := ti.RenderTimeline()

	if !strings.Contains(timeline, "session-abc123") {
		t.Error("expected session ID in timeline")
	}
	if !strings.Contains(timeline, "═══") {
		t.Error("expected separator in timeline")
	}
	if !strings.Contains(timeline, "THOUGHT") {
		t.Error("expected THOUGHT label in timeline")
	}
	if !strings.Contains(timeline, "ACTION") {
		t.Error("expected ACTION label in timeline")
	}
	if !strings.Contains(timeline, "OBSERVATION") {
		t.Error("expected OBSERVATION label in timeline")
	}
	if !strings.Contains(timeline, "DECISION") {
		t.Error("expected DECISION label in timeline")
	}
	if !strings.Contains(timeline, "Read(src/auth.go)") {
		t.Errorf("expected tool call format Read(src/auth.go), got:\n%s", timeline)
	}
	if !strings.Contains(timeline, "[0.0s]") {
		t.Error("expected [0.0s] timestamp")
	}
}

func TestRenderTimeline_Empty(t *testing.T) {
	ti := NewTrajectoryInspector("empty-session")
	timeline := ti.RenderTimeline()
	if !strings.Contains(timeline, "empty") {
		t.Errorf("expected empty indicator, got: %s", timeline)
	}
}

func TestRenderStats(t *testing.T) {
	ti := NewTrajectoryInspector("session-stats")
	ti.StartTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ti.mu.Lock()
	ti.Events = []TrajectoryEvent{
		{Index: 0, Type: "thought", Content: "Think", Tokens: 100, Timestamp: ti.StartTime},
		{Index: 1, Type: "action", Content: "file.go", ToolName: "Read", Duration: 800 * time.Millisecond, Tokens: 200, Timestamp: ti.StartTime.Add(1 * time.Second)},
		{Index: 2, Type: "action", Content: "file.go", ToolName: "Read", Duration: 600 * time.Millisecond, Tokens: 150, Timestamp: ti.StartTime.Add(2 * time.Second)},
		{Index: 3, Type: "action", Content: "file.go", ToolName: "Edit", Duration: 400 * time.Millisecond, Tokens: 300, Timestamp: ti.StartTime.Add(3 * time.Second)},
		{Index: 4, Type: "observation", Content: "Done", Tokens: 50, Timestamp: ti.StartTime.Add(4 * time.Second)},
	}
	ti.mu.Unlock()

	stats := ti.RenderStats()

	if !strings.Contains(stats, "Events: 5") {
		t.Errorf("expected 'Events: 5', got:\n%s", stats)
	}
	if !strings.Contains(stats, "Thoughts: 1") {
		t.Errorf("expected 'Thoughts: 1', got:\n%s", stats)
	}
	if !strings.Contains(stats, "Actions: 3") {
		t.Errorf("expected 'Actions: 3', got:\n%s", stats)
	}
	if !strings.Contains(stats, "Observations: 1") {
		t.Errorf("expected 'Observations: 1', got:\n%s", stats)
	}
	if !strings.Contains(stats, "Errors: 0") {
		t.Errorf("expected 'Errors: 0', got:\n%s", stats)
	}
	if !strings.Contains(stats, "Read (2x)") {
		t.Errorf("expected most used tool 'Read (2x)', got:\n%s", stats)
	}
	if !strings.Contains(stats, "Tokens:") {
		t.Errorf("expected token count, got:\n%s", stats)
	}
}

func TestFindPatterns_ReadBeforeEdit(t *testing.T) {
	ti := NewTrajectoryInspector("session-patterns")
	ti.StartTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ti.mu.Lock()
	ti.Events = []TrajectoryEvent{
		{Index: 0, Type: "action", ToolName: "Read", Content: "file.go", Timestamp: ti.StartTime},
		{Index: 1, Type: "action", ToolName: "Edit", Content: "file.go", Timestamp: ti.StartTime.Add(1 * time.Second)},
		{Index: 2, Type: "action", ToolName: "Read", Content: "other.go", Timestamp: ti.StartTime.Add(2 * time.Second)},
		{Index: 3, Type: "action", ToolName: "Edit", Content: "other.go", Timestamp: ti.StartTime.Add(3 * time.Second)},
	}
	ti.mu.Unlock()

	patterns := ti.FindPatterns()
	found := false
	for _, p := range patterns {
		if strings.Contains(p, "Read-before-edit pattern used consistently") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected read-before-edit pattern, got: %v", patterns)
	}
}

func TestFindPatterns_NoErrors(t *testing.T) {
	ti := NewTrajectoryInspector("session-clean")
	ti.StartTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ti.mu.Lock()
	ti.Events = []TrajectoryEvent{
		{Index: 0, Type: "thought", Content: "thinking", Timestamp: ti.StartTime},
		{Index: 1, Type: "action", ToolName: "Read", Content: "file.go", Timestamp: ti.StartTime.Add(1 * time.Second)},
	}
	ti.mu.Unlock()

	patterns := ti.FindPatterns()
	found := false
	for _, p := range patterns {
		if p == "No errors encountered" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'No errors encountered', got: %v", patterns)
	}
}

func TestFindPatterns_BashRetries(t *testing.T) {
	ti := NewTrajectoryInspector("session-retry")
	ti.StartTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ti.mu.Lock()
	ti.Events = []TrajectoryEvent{
		{Index: 0, Type: "action", ToolName: "Bash", Content: "go test", Timestamp: ti.StartTime},
		{Index: 1, Type: "action", ToolName: "Bash", Content: "go test", Timestamp: ti.StartTime.Add(1 * time.Second)},
		{Index: 2, Type: "action", ToolName: "Bash", Content: "go test", Timestamp: ti.StartTime.Add(2 * time.Second)},
		{Index: 3, Type: "action", ToolName: "Bash", Content: "go test", Timestamp: ti.StartTime.Add(3 * time.Second)},
	}
	ti.mu.Unlock()

	patterns := ti.FindPatterns()
	found := false
	for _, p := range patterns {
		if strings.Contains(p, "3 retries on Bash commands") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected '3 retries on Bash commands', got: %v", patterns)
	}
}

func TestTrajectoryInspectorGetByType(t *testing.T) {
	ti := NewTrajectoryInspector("session-filter")

	ti.Record("thought", "first thought", "", 0, 10)
	ti.Record("action", "file.go", "Read", 100*time.Millisecond, 20)
	ti.Record("thought", "second thought", "", 0, 15)
	ti.Record("error", "something failed", "", 0, 5)

	thoughts := ti.GetByType("thought")
	if len(thoughts) != 2 {
		t.Fatalf("expected 2 thoughts, got %d", len(thoughts))
	}
	if thoughts[0].Content != "first thought" {
		t.Errorf("expected 'first thought', got %s", thoughts[0].Content)
	}
	if thoughts[1].Content != "second thought" {
		t.Errorf("expected 'second thought', got %s", thoughts[1].Content)
	}

	errors := ti.GetByType("error")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}

	decisions := ti.GetByType("decision")
	if len(decisions) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(decisions))
	}
}

func TestGetToolUsage(t *testing.T) {
	ti := NewTrajectoryInspector("session-tools")

	ti.Record("action", "file.go", "Read", 100*time.Millisecond, 10)
	ti.Record("action", "file.go", "Read", 100*time.Millisecond, 10)
	ti.Record("action", "file.go", "Edit", 200*time.Millisecond, 20)
	ti.Record("action", "go test", "Bash", 1*time.Second, 30)
	ti.Record("thought", "thinking", "", 0, 5) // no tool

	usage := ti.GetToolUsage()
	if usage["Read"] != 2 {
		t.Errorf("expected Read: 2, got %d", usage["Read"])
	}
	if usage["Edit"] != 1 {
		t.Errorf("expected Edit: 1, got %d", usage["Edit"])
	}
	if usage["Bash"] != 1 {
		t.Errorf("expected Bash: 1, got %d", usage["Bash"])
	}
	if _, exists := usage[""]; exists {
		t.Error("empty tool name should not be in usage map")
	}
}

func TestTrajectoryInspectorExportJSON(t *testing.T) {
	ti := NewTrajectoryInspector("session-export")
	ti.Record("thought", "thinking about problem", "", 0, 100)
	ti.Record("action", "main.go", "Read", 150*time.Millisecond, 50)

	jsonStr, err := ti.ExportJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("exported JSON is invalid: %v", err)
	}

	if parsed["session_id"] != "session-export" {
		t.Errorf("expected session_id 'session-export', got %v", parsed["session_id"])
	}

	events, ok := parsed["events"].([]interface{})
	if !ok {
		t.Fatal("expected events array in JSON")
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events in JSON, got %d", len(events))
	}
}

func TestReplay(t *testing.T) {
	ti := NewTrajectoryInspector("session-replay")
	ti.StartTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ti.mu.Lock()
	ti.Events = []TrajectoryEvent{
		{Index: 0, Type: "thought", Content: "first", Timestamp: ti.StartTime},
		{Index: 1, Type: "action", Content: "second", Timestamp: ti.StartTime.Add(100 * time.Millisecond)},
		{Index: 2, Type: "observation", Content: "third", Timestamp: ti.StartTime.Add(200 * time.Millisecond)},
	}
	ti.mu.Unlock()

	// Replay at 100x speed (very fast for testing).
	ch := ti.Replay(100.0)

	var received []TrajectoryEvent
	for event := range ch {
		received = append(received, event)
	}

	if len(received) != 3 {
		t.Fatalf("expected 3 events from replay, got %d", len(received))
	}
	if received[0].Content != "first" {
		t.Errorf("expected first event content 'first', got %s", received[0].Content)
	}
	if received[2].Content != "third" {
		t.Errorf("expected third event content 'third', got %s", received[2].Content)
	}
}

func TestReplay_ZeroSpeed(t *testing.T) {
	ti := NewTrajectoryInspector("session-replay-zero")
	ti.StartTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ti.mu.Lock()
	ti.Events = []TrajectoryEvent{
		{Index: 0, Type: "thought", Content: "first", Timestamp: ti.StartTime},
		{Index: 1, Type: "action", Content: "second", Timestamp: ti.StartTime.Add(10 * time.Millisecond)},
	}
	ti.mu.Unlock()

	// Zero speed should default to 1.0, not panic.
	ch := ti.Replay(0)
	count := 0
	for range ch {
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}
}

func TestTrajectoryInspectorSummarize(t *testing.T) {
	ti := NewTrajectoryInspector("session-summary")
	ti.StartTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ti.mu.Lock()
	ti.Events = []TrajectoryEvent{
		{Index: 0, Type: "thought", Content: "thinking", Tokens: 100, Timestamp: ti.StartTime},
		{Index: 1, Type: "action", Content: "file.go", ToolName: "Read", Tokens: 200, Timestamp: ti.StartTime.Add(1 * time.Second)},
		{Index: 2, Type: "observation", Content: "found issue", Tokens: 150, Timestamp: ti.StartTime.Add(2 * time.Second)},
		{Index: 3, Type: "action", Content: "file.go", ToolName: "Edit", Tokens: 300, Timestamp: ti.StartTime.Add(3 * time.Second)},
		{Index: 4, Type: "decision", Content: "done", Tokens: 50, Timestamp: ti.StartTime.Add(4 * time.Second)},
	}
	ti.mu.Unlock()

	summary := ti.Summarize()

	if !strings.Contains(summary, "session-summary") {
		t.Errorf("expected session ID in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "5 events") {
		t.Errorf("expected '5 events' in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "2 actions") {
		t.Errorf("expected '2 actions' in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "No errors") {
		t.Errorf("expected 'No errors' in summary, got: %s", summary)
	}
}

func TestSummarize_Empty(t *testing.T) {
	ti := NewTrajectoryInspector("empty")
	summary := ti.Summarize()
	if summary != "Empty trajectory with no recorded events." {
		t.Errorf("expected empty trajectory message, got: %s", summary)
	}
}

func TestTrajectoryInspectorConcurrentAccess(t *testing.T) {
	ti := NewTrajectoryInspector("session-concurrent")

	done := make(chan struct{})

	// Concurrent writers.
	go func() {
		for i := 0; i < 100; i++ {
			ti.Record("action", "file.go", "Read", time.Millisecond, 10)
		}
		done <- struct{}{}
	}()

	// Concurrent readers.
	go func() {
		for i := 0; i < 100; i++ {
			_ = ti.RenderTimeline()
			_ = ti.RenderStats()
			_ = ti.GetByType("action")
			_ = ti.GetToolUsage()
		}
		done <- struct{}{}
	}()

	<-done
	<-done

	if len(ti.Events) != 100 {
		t.Errorf("expected 100 events after concurrent writes, got %d", len(ti.Events))
	}
}

func TestInspectorFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "500ms"},
		{5 * time.Second, "5s"},
		{65 * time.Second, "1m 5s"},
		{3*time.Minute + 45*time.Second, "3m 45s"},
	}

	for _, tt := range tests {
		got := inspectorFormatDuration(tt.d)
		if got != tt.want {
			t.Errorf("inspectorFormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestInspectorFormatTokens(t *testing.T) {
	tests := []struct {
		tokens int
		want   string
	}{
		{500, "500"},
		{1000, "1,000"},
		{4500, "4,500"},
		{12345, "12,345"},
	}

	for _, tt := range tests {
		got := inspectorFormatTokens(tt.tokens)
		if got != tt.want {
			t.Errorf("inspectorFormatTokens(%d) = %q, want %q", tt.tokens, got, tt.want)
		}
	}
}

func TestTrajectoryInspectorEventIcon(t *testing.T) {
	tests := []struct {
		eventType string
		want      string
	}{
		{"thought", "\U0001f4ad"},
		{"action", "\U0001f527"},
		{"observation", "\U0001f441"},
		{"error", "❌"},
		{"decision", "✅"},
		{"unknown", "•"},
	}

	for _, tt := range tests {
		got := eventIcon(tt.eventType)
		if got != tt.want {
			t.Errorf("eventIcon(%q) = %q, want %q", tt.eventType, got, tt.want)
		}
	}
}
