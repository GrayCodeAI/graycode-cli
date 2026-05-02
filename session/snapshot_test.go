package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSnapshotStore_TakeAndList(t *testing.T) {
	dir := t.TempDir()
	ss := &SnapshotStore{
		sessionID: "test-sess",
		dir:       filepath.Join(dir, "snapshots"),
		maxSnaps:  50,
	}

	sess := &Session{
		ID:        "test-sess",
		Model:     "gpt-4o",
		Provider:  "openai",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		},
	}

	err := ss.Take("test action", sess)
	if err != nil {
		t.Fatalf("Take error: %v", err)
	}

	snaps := ss.List()
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].ID != 1 {
		t.Errorf("expected ID 1, got %d", snaps[0].ID)
	}
	if snaps[0].Action != "test action" {
		t.Errorf("expected action 'test action', got %q", snaps[0].Action)
	}
	if snaps[0].MsgIndex != 2 {
		t.Errorf("expected MsgIndex 2, got %d", snaps[0].MsgIndex)
	}
}

func TestSnapshotStore_TakeMultiple(t *testing.T) {
	dir := t.TempDir()
	ss := &SnapshotStore{
		sessionID: "test-sess",
		dir:       filepath.Join(dir, "snapshots"),
		maxSnaps:  50,
	}

	sess := &Session{
		ID:        "test-sess",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  []Message{{Role: "user", Content: "hello"}},
	}

	ss.Take("first", sess)
	sess.Messages = append(sess.Messages, Message{Role: "assistant", Content: "hi"})
	ss.Take("second", sess)

	snaps := ss.List()
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}
	if snaps[0].ID != 1 || snaps[1].ID != 2 {
		t.Errorf("unexpected IDs: %d, %d", snaps[0].ID, snaps[1].ID)
	}
}

func TestSnapshotStore_Rewind(t *testing.T) {
	dir := t.TempDir()
	ss := &SnapshotStore{
		sessionID: "test-sess",
		dir:       filepath.Join(dir, "snapshots"),
		maxSnaps:  50,
	}

	sess := &Session{
		ID:        "test-sess",
		Model:     "gpt-4o",
		Provider:  "openai",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "hello"},
		},
	}

	ss.Take("initial", sess)

	// Add more messages
	sess.Messages = append(sess.Messages,
		Message{Role: "assistant", Content: "hi"},
		Message{Role: "user", Content: "more stuff"},
	)
	ss.Take("after changes", sess)

	// Rewind to snapshot 1
	restored, err := ss.Rewind(1)
	if err != nil {
		t.Fatalf("Rewind error: %v", err)
	}
	if len(restored.Messages) != 1 {
		t.Errorf("expected 1 message after rewind, got %d", len(restored.Messages))
	}
	if restored.Messages[0].Content != "hello" {
		t.Errorf("expected 'hello', got %q", restored.Messages[0].Content)
	}
}

func TestSnapshotStore_Rewind_NotFound(t *testing.T) {
	dir := t.TempDir()
	ss := &SnapshotStore{
		sessionID: "test-sess",
		dir:       filepath.Join(dir, "snapshots"),
		maxSnaps:  50,
	}

	_, err := ss.Rewind(999)
	if err == nil {
		t.Error("expected error for nonexistent snapshot")
	}
}

func TestSnapshotStore_Load(t *testing.T) {
	dir := t.TempDir()
	ss := &SnapshotStore{
		sessionID: "test-sess",
		dir:       filepath.Join(dir, "snapshots"),
		maxSnaps:  50,
	}

	sess := &Session{
		ID:        "test-sess",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  []Message{{Role: "user", Content: "hello"}},
	}
	ss.Take("test", sess)

	// Create a new store and load
	ss2 := &SnapshotStore{
		sessionID: "test-sess",
		dir:       filepath.Join(dir, "snapshots"),
		maxSnaps:  50,
	}
	err := ss2.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	snaps := ss2.List()
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot after load, got %d", len(snaps))
	}
}

func TestSnapshotStore_Load_NoIndex(t *testing.T) {
	dir := t.TempDir()
	ss := &SnapshotStore{
		sessionID: "test-sess",
		dir:       filepath.Join(dir, "snapshots"),
		maxSnaps:  50,
	}

	err := ss.Load()
	if err != nil {
		t.Fatalf("Load with no index should not error: %v", err)
	}
	if len(ss.snapshots) != 0 {
		t.Error("should have no snapshots")
	}
}

func TestSnapshotStore_Format(t *testing.T) {
	ss := &SnapshotStore{
		sessionID: "test-sess",
		maxSnaps:  50,
	}

	formatted := ss.Format()
	if !strings.Contains(formatted, "No snapshots") {
		t.Errorf("expected 'No snapshots', got %q", formatted)
	}

	ss.snapshots = []Snapshot{
		{ID: 1, Timestamp: time.Now(), MsgIndex: 3, Action: "edit file"},
		{ID: 2, Timestamp: time.Now(), MsgIndex: 5, Action: "add feature", Label: "checkpoint"},
	}

	formatted = ss.Format()
	if !strings.Contains(formatted, "Snapshots (2)") {
		t.Errorf("expected 'Snapshots (2)', got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "edit file") {
		t.Error("should contain action")
	}
	if !strings.Contains(formatted, "[checkpoint]") {
		t.Error("should contain label")
	}
}

func TestSnapshotStore_Cleanup(t *testing.T) {
	dir := t.TempDir()
	ss := &SnapshotStore{
		sessionID: "test-sess",
		dir:       filepath.Join(dir, "snapshots"),
		maxSnaps:  3,
	}
	os.MkdirAll(ss.dir, 0o755)

	sess := &Session{
		ID:        "test-sess",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  []Message{{Role: "user", Content: "hello"}},
	}

	// Take 5 snapshots (max is 3)
	for i := 0; i < 5; i++ {
		ss.Take("action", sess)
	}

	if len(ss.snapshots) != 3 {
		t.Errorf("expected 3 snapshots after cleanup, got %d", len(ss.snapshots))
	}

	// The remaining snapshots should be IDs 3, 4, 5
	if ss.snapshots[0].ID != 3 {
		t.Errorf("expected first remaining snapshot ID 3, got %d", ss.snapshots[0].ID)
	}

	// Old snapshot files should be deleted
	for _, id := range []int{1, 2} {
		path := filepath.Join(ss.dir, fmt.Sprintf("%d.jsonl", id))
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("snapshot %d file should be deleted", id)
		}
	}
}

