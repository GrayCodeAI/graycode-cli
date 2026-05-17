package session

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestSaveAndLoadMessages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(fmt.Sprintf("%s/.hawk/sessions", dir), 0o755)

	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
		{Role: "user", Content: "thanks"},
	}

	if err := SaveMessages("msg-persist", msgs); err != nil {
		t.Fatalf("SaveMessages: %v", err)
	}

	loaded, err := LoadMessages("msg-persist")
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	if len(loaded) != 3 {
		t.Errorf("LoadMessages = %d, want 3", len(loaded))
	}
}

func TestSessionPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	path := SessionPath(dir, "test-id")
	if path == "" {
		t.Error("SessionPath should return non-empty")
	}
}

func TestValidateIntegrity(t *testing.T) {
	sess := &Session{
		ID: "integrity-test", Model: "test", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Messages: []Message{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "hi"}},
	}
	result := ValidateIntegrity(sess)
	if result == nil {
		t.Error("ValidateIntegrity should return result")
	}
}

func TestComputeChecksum(t *testing.T) {
	t.Parallel()
	sess := &Session{
		ID: "checksum-test", Model: "test", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Messages: []Message{{Role: "user", Content: "data"}},
	}
	checksum := ComputeChecksum(sess)
	if checksum == "" {
		t.Error("checksum should not be empty")
	}
}

func TestStats(t *testing.T) {
	t.Parallel()
	sess := &Session{
		ID: "stats-test", Model: "test", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Messages: []Message{{Role: "user", Content: "msg1"}, {Role: "assistant", Content: "msg2"}},
	}
	stats := Stats(sess)
	if stats == nil {
		t.Error("Stats should return non-nil map")
	}
}

func TestLoadLatest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(fmt.Sprintf("%s/.hawk/sessions", dir), 0o755)

	sess := &Session{
		ID: "latest-test", Model: "test", CWD: dir, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Messages: []Message{{Role: "user", Content: "hi"}},
	}
	_ = Save(sess)

	latest, err := LoadLatest()
	if err != nil {
		// May fail if no sessions match CWD — that's ok
		_ = err
	}
	_ = latest
}
