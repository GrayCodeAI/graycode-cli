package session

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSmartCheckpointer_ShouldCheckpoint(t *testing.T) {
	dir := t.TempDir()
	store := &SnapshotStore{
		sessionID: "test-sess",
		dir:       filepath.Join(dir, "snapshots"),
		maxSnaps:  50,
	}

	sc := NewSmartCheckpointer(store)

	sess := &Session{
		ID:        "test-sess",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  []Message{{Role: "user", Content: "hello"}},
	}

	// First call: state is new, should checkpoint.
	if !sc.ShouldCheckpoint(TriggerFileWrite, sess) {
		t.Error("expected ShouldCheckpoint to return true for first event")
	}

	// Manually simulate that a checkpoint was taken.
	sc.OnEvent(TriggerFileWrite, sess, "initial")

	// Same state: should NOT checkpoint.
	if sc.ShouldCheckpoint(TriggerFileWrite, sess) {
		t.Error("expected ShouldCheckpoint to return false for unchanged state")
	}

	// Change state.
	sess.Messages = append(sess.Messages, Message{Role: "assistant", Content: "hi there"})
	if !sc.ShouldCheckpoint(TriggerFileWrite, sess) {
		t.Error("expected ShouldCheckpoint to return true after state change")
	}
}

func TestSmartCheckpointer_DisabledTrigger(t *testing.T) {
	sc := NewSmartCheckpointer(nil)

	sess := &Session{
		ID:       "test-sess",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}

	// Disable TriggerToolError.
	sc.SetTrigger(TriggerToolError, false)

	if sc.ShouldCheckpoint(TriggerToolError, sess) {
		t.Error("expected ShouldCheckpoint to return false for disabled trigger")
	}

	// Enabled trigger should still work.
	if !sc.ShouldCheckpoint(TriggerFileWrite, sess) {
		t.Error("expected ShouldCheckpoint to return true for enabled trigger")
	}
}

func TestSmartCheckpointer_OnEventFiltering(t *testing.T) {
	dir := t.TempDir()
	store := &SnapshotStore{
		sessionID: "test-sess",
		dir:       filepath.Join(dir, "snapshots"),
		maxSnaps:  50,
	}

	sc := NewSmartCheckpointer(store)

	sess := &Session{
		ID:        "test-sess",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  []Message{{Role: "user", Content: "hello"}},
	}

	// Fire multiple events with the same state.
	sc.OnEvent(TriggerFileWrite, sess, "first write")
	sc.OnEvent(TriggerFileWrite, sess, "same write")
	sc.OnEvent(TriggerFileWrite, sess, "same write again")

	// Only the first should result in a checkpoint.
	if sc.CheckpointsTaken() != 1 {
		t.Errorf("expected 1 checkpoint, got %d", sc.CheckpointsTaken())
	}
	if sc.EventsSeen() != 3 {
		t.Errorf("expected 3 events seen, got %d", sc.EventsSeen())
	}

	// Now change state and fire again.
	sess.Messages = append(sess.Messages, Message{Role: "assistant", Content: "world"})
	sc.OnEvent(TriggerUserFeedback, sess, "new content")

	if sc.CheckpointsTaken() != 2 {
		t.Errorf("expected 2 checkpoints, got %d", sc.CheckpointsTaken())
	}
}

func TestSmartCheckpointer_Stats(t *testing.T) {
	sc := NewSmartCheckpointer(nil)

	sess := &Session{
		ID:       "test-sess",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}

	sc.OnEvent(TriggerFileWrite, sess, "write")
	sc.OnEvent(TriggerFileWrite, sess, "same") // filtered
	sc.OnEvent(TriggerFileWrite, sess, "same") // filtered

	sess.Messages = append(sess.Messages, Message{Role: "assistant", Content: "response"})
	sc.OnEvent(TriggerPlanChange, sess, "plan changed")

	stats := sc.Stats()
	if !strings.Contains(stats, "4 events seen") {
		t.Errorf("expected '4 events seen' in stats, got: %s", stats)
	}
	if !strings.Contains(stats, "2 checkpoints taken") {
		t.Errorf("expected '2 checkpoints taken' in stats, got: %s", stats)
	}
	if !strings.Contains(stats, "filtered") {
		t.Errorf("expected 'filtered' in stats, got: %s", stats)
	}
}
