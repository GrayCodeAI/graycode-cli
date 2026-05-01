package session

import (
	"testing"
)

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
