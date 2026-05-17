package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompressOldSessions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Create a session
	sess := &Session{
		ID:       "compress-test",
		Model:    "gpt-4o",
		Provider: "openai",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}
	if err := Save(sess); err != nil {
		t.Fatal(err)
	}

	// Set the file modification time to 10 days ago
	dir := sessionsDir()
	path := filepath.Join(dir, "compress-test.jsonl")
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	os.Chtimes(path, oldTime, oldTime)

	// Compress sessions older than 7 days
	count, err := CompressOldSessions(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("CompressOldSessions error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 compressed, got %d", count)
	}

	// Original should be gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("original file should be removed after compression")
	}

	// Compressed file should exist
	gzPath := path + ".gz"
	if _, err := os.Stat(gzPath); err != nil {
		t.Fatalf("compressed file should exist: %v", err)
	}
}

func TestCompressOldSessions_RecentNotCompressed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	sess := &Session{
		ID:       "recent-test",
		Model:    "gpt-4o",
		Provider: "openai",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}
	if err := Save(sess); err != nil {
		t.Fatal(err)
	}

	count, err := CompressOldSessions(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("CompressOldSessions error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 compressed (recent file), got %d", count)
	}
}

func TestDecompressSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	sess := &Session{
		ID:       "decompress-test",
		Model:    "gpt-4o",
		Provider: "openai",
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
	}
	if err := Save(sess); err != nil {
		t.Fatal(err)
	}

	// Manually compress
	dir := sessionsDir()
	path := filepath.Join(dir, "decompress-test.jsonl")
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	os.Chtimes(path, oldTime, oldTime)
	CompressOldSessions(1 * 24 * time.Hour)

	// Decompress
	loaded, err := DecompressSession("decompress-test")
	if err != nil {
		t.Fatalf("DecompressSession error: %v", err)
	}
	if loaded.Model != "gpt-4o" {
		t.Fatalf("expected model gpt-4o, got %s", loaded.Model)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "hello" {
		t.Fatalf("expected first message 'hello', got %q", loaded.Messages[0].Content)
	}
}

func TestDecompressSession_NotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := DecompressSession("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}
