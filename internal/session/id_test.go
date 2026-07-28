package session

import (
	"strings"
	"testing"
)

func TestValidateID(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"abc123", "session-name", "session_name", "session.name"} {
		if err := ValidateID(id); err != nil {
			t.Errorf("ValidateID(%q) returned error: %v", id, err)
		}
	}

	invalid := []string{"", ".", "..", "../escape", `..\escape`, "/absolute", "has space", strings.Repeat("a", maxSessionIDLength+1)}
	for _, id := range invalid {
		if err := ValidateID(id); err == nil {
			t.Errorf("ValidateID(%q) returned nil", id)
		}
	}
}

func TestSessionFileOperationsRejectTraversal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setTestSessionsDir(t, t.TempDir())
	const traversal = "../../escaped"

	if err := Save(&Session{ID: traversal}); err == nil {
		t.Error("Save accepted a traversal session ID")
	}
	if _, err := Load(traversal); err == nil {
		t.Error("Load accepted a traversal session ID")
	}
	if _, err := NewWAL(traversal); err == nil {
		t.Error("NewWAL accepted a traversal session ID")
	}
	if _, err := RecoverFromWAL(traversal); err == nil {
		t.Error("RecoverFromWAL accepted a traversal session ID")
	}
	if _, err := AcquireLock(traversal); err == nil {
		t.Error("AcquireLock accepted a traversal session ID")
	}
	if _, err := DecompressSession(traversal); err == nil {
		t.Error("DecompressSession accepted a traversal session ID")
	}

	store := NewSnapshotStore(traversal)
	if err := store.Load(); err == nil {
		t.Error("SnapshotStore accepted a traversal session ID")
	}
	if got := SessionPath(t.TempDir(), traversal); got != "" {
		t.Errorf("SessionPath returned unsafe path %q", got)
	}
}
