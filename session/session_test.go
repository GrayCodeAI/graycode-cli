package session

import (
	"os"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	// Use temp dir for sessions
	tmp := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", orig)

	s := &Session{
		ID:       "test123",
		Model:    "claude-sonnet",
		Provider: "anthropic",
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
	}
	if err := Save(s); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load("test123")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != "test123" {
		t.Fatalf("got ID %q", loaded.ID)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("got %d messages", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "hello" {
		t.Fatalf("got %q", loaded.Messages[0].Content)
	}
}

func TestList(t *testing.T) {
	tmp := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", orig)

	Save(&Session{ID: "a", Messages: []Message{{Role: "user", Content: "first"}}})
	Save(&Session{ID: "b", Messages: []Message{{Role: "user", Content: "second"}}})

	entries, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries", len(entries))
	}
}

func TestLoadMissing(t *testing.T) {
	tmp := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", orig)

	_, err := Load("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}
