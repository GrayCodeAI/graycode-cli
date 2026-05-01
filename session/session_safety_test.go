package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAtomicSave(t *testing.T) {
	// Setup temp dir
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	sess := &Session{
		ID:        "test-atomic-1",
		Model:     "claude-4",
		Provider:  "anthropic",
		CWD:       "/tmp/test",
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
		CreatedAt: time.Now(),
	}

	// Save should succeed
	if err := Save(sess); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// File should exist (no .tmp leftover)
	path := filepath.Join(tmp, ".hawk", "sessions", "test-atomic-1.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session file not found: %v", err)
	}

	// No temp file should remain
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); err == nil {
		t.Error("temp file should not exist after successful save")
	}

	// Load it back
	loaded, err := Load("test-atomic-1")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.Model != "claude-4" {
		t.Errorf("expected model claude-4, got %s", loaded.Model)
	}
	if len(loaded.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(loaded.Messages))
	}
}

func TestAtomicSave_OverwritesCleanly(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	sess := &Session{
		ID:       "test-overwrite",
		Model:    "model-1",
		Messages: []Message{{Role: "user", Content: "first"}},
	}
	Save(sess)

	// Overwrite with more messages
	sess.Model = "model-2"
	sess.Messages = append(sess.Messages, Message{Role: "assistant", Content: "second"})
	Save(sess)

	loaded, _ := Load("test-overwrite")
	if loaded.Model != "model-2" {
		t.Errorf("expected model-2, got %s", loaded.Model)
	}
	if len(loaded.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(loaded.Messages))
	}
}

func TestWAL_BasicAppend(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	os.MkdirAll(filepath.Join(tmp, ".hawk", "sessions"), 0o755)

	wal, err := NewWAL("test-wal-1")
	if err != nil {
		t.Fatalf("NewWAL error: %v", err)
	}

	// Append messages
	wal.AppendMeta("claude-4", "anthropic", "/tmp")
	wal.Append(Message{Role: "user", Content: "hello"})
	wal.Append(Message{Role: "assistant", Content: "hi"})
	wal.Append(Message{Role: "user", Content: "how are you?"})
	wal.Close()

	// Recover from WAL
	recovered, err := RecoverFromWAL("test-wal-1")
	if err != nil {
		t.Fatalf("RecoverFromWAL error: %v", err)
	}
	if recovered == nil {
		t.Fatal("expected recovered session")
	}
	if recovered.Model != "claude-4" {
		t.Errorf("expected model claude-4, got %s", recovered.Model)
	}
	if len(recovered.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(recovered.Messages))
	}
	if recovered.Messages[2].Content != "how are you?" {
		t.Errorf("expected last message 'how are you?', got %q", recovered.Messages[2].Content)
	}
}

func TestWAL_Remove(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	os.MkdirAll(filepath.Join(tmp, ".hawk", "sessions"), 0o755)

	wal, _ := NewWAL("test-wal-remove")
	wal.Append(Message{Role: "user", Content: "test"})
	wal.Remove()

	// WAL file should be gone
	path := filepath.Join(tmp, ".hawk", "sessions", "test-wal-remove.wal")
	if _, err := os.Stat(path); err == nil {
		t.Error("WAL file should be removed")
	}

	// Recovery should return nil
	recovered, _ := RecoverFromWAL("test-wal-remove")
	if recovered != nil {
		t.Error("should not recover from removed WAL")
	}
}

func TestWAL_NoRecoveryIfNoWAL(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	recovered, err := RecoverFromWAL("nonexistent")
	if err != nil {
		t.Errorf("should not error for missing WAL, got %v", err)
	}
	if recovered != nil {
		t.Error("should return nil for missing WAL")
	}
}

func TestCheckForRecovery(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	dir := filepath.Join(tmp, ".hawk", "sessions")
	os.MkdirAll(dir, 0o755)

	// Create some WAL files
	os.WriteFile(filepath.Join(dir, "session-a.wal"), []byte("data"), 0o644)
	os.WriteFile(filepath.Join(dir, "session-b.wal"), []byte("data"), 0o644)
	os.WriteFile(filepath.Join(dir, "session-c.jsonl"), []byte("not a wal"), 0o644)

	ids := CheckForRecovery()
	if len(ids) != 2 {
		t.Errorf("expected 2 WAL files, got %d", len(ids))
	}
}

func TestLoadPreview(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.jsonl")

	content := `{"type":"session_meta","id":"x","model":"m"}
{"role":"user","content":"What is the meaning of life?"}
{"role":"assistant","content":"42"}
`
	os.WriteFile(path, []byte(content), 0o644)

	preview := loadPreview(path)
	if preview != "What is the meaning of life?" {
		t.Errorf("expected user message preview, got %q", preview)
	}
}

func TestList_UsesFileModTime(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	dir := filepath.Join(tmp, ".hawk", "sessions")
	os.MkdirAll(dir, 0o755)

	// Create two session files with different mod times
	content := `{"type":"session_meta","id":"s1"}
{"role":"user","content":"first session"}
`
	os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(content), 0o644)
	time.Sleep(10 * time.Millisecond)
	content2 := `{"type":"session_meta","id":"s2"}
{"role":"user","content":"second session"}
`
	os.WriteFile(filepath.Join(dir, "s2.jsonl"), []byte(content2), 0o644)

	entries, err := List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Most recent should be first
	if entries[0].ID != "s2" {
		t.Errorf("expected s2 first (newest), got %s", entries[0].ID)
	}
}

func TestCorruptedLineSkipped(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	dir := filepath.Join(tmp, ".hawk", "sessions")
	os.MkdirAll(dir, 0o755)

	// Write a file with one corrupted line
	content := `{"type":"session_meta","id":"corrupt-test","model":"m"}
{"role":"user","content":"good message"}
this is not valid json at all!!!
{"role":"assistant","content":"also good"}
`
	os.WriteFile(filepath.Join(dir, "corrupt-test.jsonl"), []byte(content), 0o644)

	loaded, err := Load("corrupt-test")
	if err != nil {
		t.Fatalf("Load should not fail on corrupted lines: %v", err)
	}
	if len(loaded.Messages) != 2 {
		t.Errorf("expected 2 valid messages (skipping corrupted), got %d", len(loaded.Messages))
	}
}
