package session

import (
	"os"
	"testing"
	"time"
)

// TestSaveSessionFileMode verifies that the persisted session JSONL is created
// without any group/other permissions. Session files hold full conversation
// history — private user state — so they must not be readable by other local
// users (os.Create would leave them 0666 &^ umask, typically 0644).
func TestSaveSessionFileMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	sessDir := setTestSessionsDir(t, dir)
	_ = os.MkdirAll(sessDir, 0o755)

	sess := &Session{
		ID:        "perm-check",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  []Message{{Role: "user", Content: "sensitive"}},
	}
	if err := Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(jsonlPathFor(sess.ID))
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}
	// Assert the security property directly (no group/other access) so the
	// check is independent of the process umask.
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("session file perm = %04o, has group/other bits; want owner-only (0600)", perm)
	}
}
