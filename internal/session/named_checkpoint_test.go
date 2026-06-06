package session

import (
	"testing"
)

// TestNamedCheckpointRoundTrip verifies a labeled session snapshot saves and
// restores with full message history, model, and provider intact.
func TestNamedCheckpointRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	orig := &Session{
		ID:       "sess-1",
		Model:    "claude-opus-4-8",
		Provider: "anthropic",
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
	}

	cp, err := SaveNamedCheckpoint("my-feature", orig)
	if err != nil {
		t.Fatalf("SaveNamedCheckpoint: %v", err)
	}
	if cp.Name != "my-feature" {
		t.Errorf("name = %q, want my-feature", cp.Name)
	}

	got, err := LoadNamedCheckpoint("my-feature")
	if err != nil {
		t.Fatalf("LoadNamedCheckpoint: %v", err)
	}
	if got.Session == nil {
		t.Fatal("restored session is nil")
	}
	if got.Session.ID != orig.ID {
		t.Errorf("ID = %q, want %q", got.Session.ID, orig.ID)
	}
	if got.Session.Model != orig.Model || got.Session.Provider != orig.Provider {
		t.Errorf("model/provider = %s/%s, want %s/%s",
			got.Session.Model, got.Session.Provider, orig.Model, orig.Provider)
	}
	if len(got.Session.Messages) != len(orig.Messages) {
		t.Fatalf("message count = %d, want %d", len(got.Session.Messages), len(orig.Messages))
	}
	for i := range orig.Messages {
		if got.Session.Messages[i].Role != orig.Messages[i].Role ||
			got.Session.Messages[i].Content != orig.Messages[i].Content {
			t.Errorf("message %d = %+v, want %+v", i, got.Session.Messages[i], orig.Messages[i])
		}
	}
}

// TestNamedCheckpointSnapshotIsImmutable verifies the snapshot is a deep copy:
// mutating the live session after saving does not change the saved checkpoint.
func TestNamedCheckpointSnapshotIsImmutable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s := &Session{ID: "s", Messages: []Message{{Role: "user", Content: "v1"}}}
	if _, err := SaveNamedCheckpoint("snap", s); err != nil {
		t.Fatal(err)
	}
	// Mutate the live session.
	s.Messages[0].Content = "MUTATED"
	s.Messages = append(s.Messages, Message{Role: "user", Content: "extra"})

	got, err := LoadNamedCheckpoint("snap")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Session.Messages) != 1 || got.Session.Messages[0].Content != "v1" {
		t.Errorf("snapshot was not immutable: %+v", got.Session.Messages)
	}
}

// TestNamedCheckpointOverwriteAndList verifies re-saving a label overwrites it
// and that List/Delete behave.
func TestNamedCheckpointOverwriteAndList(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := SaveNamedCheckpoint("a", &Session{ID: "a1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveNamedCheckpoint("a", &Session{ID: "a2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveNamedCheckpoint("b", &Session{ID: "b1"}); err != nil {
		t.Fatal(err)
	}

	got, err := LoadNamedCheckpoint("a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Session.ID != "a2" {
		t.Errorf("overwrite failed: got session %q, want a2", got.Session.ID)
	}

	list, err := ListNamedCheckpoints()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(list))
	}

	if err := DeleteNamedCheckpoint("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadNamedCheckpoint("a"); err == nil {
		t.Error("expected error loading deleted checkpoint")
	}
}

// TestNamedCheckpointErrors covers nil/empty inputs and missing checkpoints.
func TestNamedCheckpointErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := SaveNamedCheckpoint("", &Session{}); err == nil {
		t.Error("expected error for empty name")
	}
	if _, err := SaveNamedCheckpoint("x", nil); err == nil {
		t.Error("expected error for nil session")
	}
	if _, err := LoadNamedCheckpoint("missing"); err == nil {
		t.Error("expected error loading missing checkpoint")
	}
}
