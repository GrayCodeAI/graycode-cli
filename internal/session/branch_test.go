package session

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
)

// mustJSON encodes v to json.RawMessage, failing the test on error.
func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustJSON: %v", err)
	}
	return b
}

func TestFork(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	original := &Session{
		ID:       "fork-origin",
		Model:    "gpt-4o",
		Provider: "openai",
		Messages: []Message{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "response 1"},
			{Role: "user", Content: "second"},
			{Role: "assistant", Content: "response 2"},
			{Role: "user", Content: "third"},
		},
	}
	if err := Save(original); err != nil {
		t.Fatal(err)
	}

	forked, err := Fork("fork-origin", 2)
	if err != nil {
		t.Fatalf("Fork error: %v", err)
	}

	if forked.ID == "fork-origin" {
		t.Fatal("forked session should have a new ID")
	}
	if len(forked.Messages) != 3 {
		t.Fatalf("expected 3 messages in fork, got %d", len(forked.Messages))
	}
	if forked.Messages[2].Content != "second" {
		t.Fatalf("expected last message 'second', got %q", forked.Messages[2].Content)
	}
	if forked.Model != "gpt-4o" {
		t.Fatalf("expected model gpt-4o, got %s", forked.Model)
	}

	// Verify the fork was saved
	loaded, err := Load(forked.ID)
	if err != nil {
		t.Fatalf("could not load forked session: %v", err)
	}
	if len(loaded.Messages) != 3 {
		t.Fatalf("loaded fork should have 3 messages, got %d", len(loaded.Messages))
	}
}

func TestFork_InvalidIndex(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	original := &Session{
		ID:       "fork-invalid",
		Model:    "gpt-4o",
		Provider: "openai",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}
	if err := Save(original); err != nil {
		t.Fatal(err)
	}

	if _, err := Fork("fork-invalid", -1); err == nil {
		t.Fatal("expected error for negative index")
	}
	if _, err := Fork("fork-invalid", 5); err == nil {
		t.Fatal("expected error for out-of-bounds index")
	}
}

func TestFork_NonexistentSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := Fork("nonexistent-id", 0); err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

// TestForkAtEvent_ForksMessageIndexAndEvents verifies that ForkAtEvent
// truncates both the event spine and message list to the correct point,
// and adds a SessionEndSeed boundary to the forked session.
func TestForkAtEvent_ForksMessageIndexAndEvents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	original := &Session{
		ID:       "fork-event-origin",
		Model:    "gpt-4o",
		Provider: "openai",
		Messages: []Message{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "response 1"},
			{Role: "user", Content: "second"},
			{Role: "assistant", Content: "response 2"},
		},
		Events: []eventlog.WireEvent{
			{Type: eventlog.UserMessage, Seq: 1, Data: mustJSON(t, Message{Content: "first"})},
			{Type: eventlog.ToolCall, Seq: 2, Data: mustJSON(t, eventlog.ToolCallPayload{Name: "Read"})},
			{Type: eventlog.AssistantMsg, Seq: 3, Data: mustJSON(t, Message{Content: "response 1"})},
			{Type: eventlog.UserMessage, Seq: 4, Data: mustJSON(t, Message{Content: "second"})},
			{Type: eventlog.AssistantMsg, Seq: 5, Data: mustJSON(t, Message{Content: "response 2"})},
		},
	}
	if err := Save(original); err != nil {
		t.Fatal(err)
	}

	// Fork at event seq 3 (= assistant response 1)
	forked, err := ForkAtEvent("fork-event-origin", 3)
	if err != nil {
		t.Fatalf("ForkAtEvent error: %v", err)
	}

	if len(forked.Messages) != 2 {
		t.Fatalf("expected 2 messages in fork, got %d", len(forked.Messages))
	}
	if forked.Messages[1].Content != "response 1" {
		t.Fatalf("expected last message 'response 1', got %q", forked.Messages[1].Content)
	}

	// Events should be truncated to seq <= 3 (3 events)
	if len(forked.Events) != 4 { // 3 original events + 1 SessionEndSeed marker
		t.Fatalf("expected 4 events in fork (3 + seed), got %d", len(forked.Events))
	}
	if forked.Events[2].Seq != 3 {
		t.Fatalf("expected last pre-seed event seq 3, got %d", forked.Events[2].Seq)
	}
	// Last event should be the seed
	if forked.Events[3].Type != eventlog.SessionEndSeed {
		t.Fatalf("expected SessionEndSeed marker as last event, got %s", forked.Events[3].Type)
	}
}

// TestForkAtEvent_NoEvents verifies graceful fallback when parent has no
// event spine (version-0 session).
func TestForkAtEvent_NoEvents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	original := &Session{
		ID: "fork-no-events",
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		},
	}
	if err := Save(original); err != nil {
		t.Fatal(err)
	}

	forked, err := ForkAtEvent("fork-no-events", 1)
	if err != nil {
		t.Fatalf("ForkAtEvent error: %v", err)
	}

	if len(forked.Messages) != 2 {
		t.Fatalf("expected 2 messages (all), got %d", len(forked.Messages))
	}
	if len(forked.Events) != 0 {
		t.Fatalf("expected 0 events for version-0 fork, got %d", len(forked.Events))
	}
}

// TestForkAtEvent_NonexistentSession verifies error on missing parent.
func TestForkAtEvent_NonexistentSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := ForkAtEvent("missing-session-id", 1); err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

// TestRepairJournal_ValidSpine verifies no repair needed when spine is valid.
func TestRepairJournal_ValidSpine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	original := &Session{
		ID: "repair-valid",
		Events: []eventlog.WireEvent{
			{Type: eventlog.SessionMeta, Seq: 1, Data: mustJSON(t, eventlog.Meta{ID: "s1"})},
			{Type: eventlog.UserMessage, Seq: 2, Data: mustJSON(t, Message{Content: "hi"})},
			{Type: eventlog.AssistantMsg, Seq: 3, Data: mustJSON(t, Message{Content: "hello"})},
		},
	}
	if err := Save(original); err != nil {
		t.Fatal(err)
	}

	pruned, err := RepairJournal("repair-valid")
	if err != nil {
		t.Fatalf("RepairJournal error: %v", err)
	}
	if pruned != 0 {
		t.Fatalf("expected 0 pruned for valid spine, got %d", pruned)
	}
}

// TestRepairJournal_CorruptSpine verifies truncation at the last valid event.
func TestRepairJournal_CorruptSpine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	original := &Session{
		ID: "repair-corrupt",
		Events: []eventlog.WireEvent{
			{Type: eventlog.SessionMeta, Seq: 1, Data: mustJSON(t, eventlog.Meta{ID: "s1"})},
			{Type: eventlog.UserMessage, Seq: 2, Data: mustJSON(t, Message{Content: "hi"})},
			{Type: eventlog.AssistantMsg, Seq: 3, Data: mustJSON(t, Message{Content: "hello"})},
		},
		Messages: []Message{{Role: "user", Content: "hi"}},
	}
	if err := Save(original); err != nil {
		t.Fatal(err)
	}

	// Corrupt the file on disk by appending an event line with a known type
	// but a Data payload that won't unmarshal into the expected struct.
	// scanJSONLLines captures this as a WireEvent (type is known), but
	// DecodeWire rejects it when the payload doesn't match.
	path := jsonlPathFor("repair-corrupt")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n" + `{"type":"message.assistant","seq":4,"at":"2024-01-01T00:00:00Z","data":{"role":"assistant","content":12345}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	pruned, err := RepairJournal("repair-corrupt")
	if err != nil {
		t.Fatalf("RepairJournal error: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 event pruned, got %d", pruned)
	}

	// Verify the repaired session still loads and has 3 valid events
	loaded, err := Load("repair-corrupt")
	if err != nil {
		t.Fatalf("could not load repaired session: %v", err)
	}
	if len(loaded.Events) != 3 {
		t.Fatalf("expected 3 events after repair, got %d", len(loaded.Events))
	}
	// Verify the repaired spine validates
	if _, err := eventlog.DecodeWire(loaded.Events); err != nil {
		t.Fatalf("repaired spine should validate: %v", err)
	}
}

// TestRepairJournal_NoEvents verifies no-op when spine is empty.
func TestRepairJournal_NoEvents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	original := &Session{
		ID:       "repair-none",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}
	if err := Save(original); err != nil {
		t.Fatal(err)
	}

	pruned, err := RepairJournal("repair-none")
	if err != nil {
		t.Fatalf("RepairJournal error: %v", err)
	}
	if pruned != 0 {
		t.Fatalf("expected 0 pruned for empty spine, got %d", pruned)
	}
}

// TestFork_PreservesEvents verifies the standard Fork now also preserves
// the event spine.
func TestFork_PreservesEvents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	original := &Session{
		ID: "fork-events-origin",
		Events: []eventlog.WireEvent{
			{Type: eventlog.UserMessage, Seq: 1, Data: mustJSON(t, Message{Content: "first"})},
			{Type: eventlog.AssistantMsg, Seq: 2, Data: mustJSON(t, Message{Content: "response 1"})},
			{Type: eventlog.UserMessage, Seq: 3, Data: mustJSON(t, Message{Content: "second"})},
		},
		Messages: []Message{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "response 1"},
			{Role: "user", Content: "second"},
		},
	}
	if err := Save(original); err != nil {
		t.Fatal(err)
	}

	forked, err := Fork("fork-events-origin", 1)
	if err != nil {
		t.Fatalf("Fork error: %v", err)
	}

	// Fork at message index 1 → should include events up to seq of the 2nd
	// model-visible event (AssistantMsg at seq 2).
	if len(forked.Events) != 2 {
		t.Fatalf("expected 2 events in fork, got %d", len(forked.Events))
	}
	if forked.Events[1].Seq != 2 {
		t.Fatalf("expected last event seq 2, got %d", forked.Events[1].Seq)
	}
}
