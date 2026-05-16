package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// CheckpointManager tests
// ─────────────────────────────────────────────────────────────────────────────

func TestNewCheckpointManager(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	if cm.MaxCheckpoints != 20 {
		t.Errorf("expected MaxCheckpoints=20, got %d", cm.MaxCheckpoints)
	}
	if cm.Dir != dir {
		t.Errorf("expected Dir=%s, got %s", dir, cm.Dir)
	}
	if len(cm.Checkpoints) != 0 {
		t.Errorf("expected empty checkpoints, got %d", len(cm.Checkpoints))
	}
}

func TestCheckpointManager_Create(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	// Create a test file to track
	testFile := filepath.Join(dir, "test.go")
	os.WriteFile(testFile, []byte("package main\n"), 0o644)

	messages := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}

	cp, err := cm.Create("before refactor", "About to refactor auth", messages, []string{testFile})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if cp.Name != "before refactor" {
		t.Errorf("expected name 'before refactor', got %q", cp.Name)
	}
	if cp.Description != "About to refactor auth" {
		t.Errorf("expected description 'About to refactor auth', got %q", cp.Description)
	}
	if cp.MessageCount != 2 {
		t.Errorf("expected 2 messages, got %d", cp.MessageCount)
	}
	if cp.Auto {
		t.Error("expected Auto=false for named checkpoint")
	}
	if len(cp.FilesState) != 1 {
		t.Errorf("expected 1 file in state, got %d", len(cp.FilesState))
	}
	if _, ok := cp.FilesState[testFile]; !ok {
		t.Error("expected test file in FilesState")
	}
	if cp.ID == "" {
		t.Error("expected non-empty ID")
	}
	if cp.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestCheckpointManager_AutoCheckpoint(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	messages := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "do something"},
	}

	cp, err := cm.AutoCheckpoint(messages, nil)
	if err != nil {
		t.Fatalf("AutoCheckpoint failed: %v", err)
	}

	if !cp.Auto {
		t.Error("expected Auto=true for auto-checkpoint")
	}
	if !strings.HasPrefix(cp.Name, "auto-checkpoint-") {
		t.Errorf("expected auto-checkpoint prefix, got %q", cp.Name)
	}
	if cp.MessageCount != 3 {
		t.Errorf("expected 3 messages, got %d", cp.MessageCount)
	}
}

func TestCheckpointManager_Restore(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	// Create a file that we will modify after checkpointing
	testFile := filepath.Join(dir, "restore_target.txt")
	os.WriteFile(testFile, []byte("original content"), 0o644)

	messages := []Message{
		{Role: "user", Content: "first message"},
		{Role: "assistant", Content: "first response"},
	}

	cp, err := cm.Create("restore point", "testing restore", messages, []string{testFile})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Modify the file
	os.WriteFile(testFile, []byte("modified content"), 0o644)

	// Restore
	restored, err := cm.Restore(cp.ID)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	if len(restored) != 2 {
		t.Errorf("expected 2 messages restored, got %d", len(restored))
	}
	if restored[0].Content != "first message" {
		t.Errorf("expected 'first message', got %q", restored[0].Content)
	}

	// Verify file was restored
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(content) != "original content" {
		t.Errorf("expected 'original content', got %q", string(content))
	}
}

func TestCheckpointManager_Restore_NotFound(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	_, err := cm.Restore("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent checkpoint")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestCheckpointManager_List(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	msgs := []Message{{Role: "user", Content: "hi"}}

	cm.Create("first", "first checkpoint", msgs, nil)
	cm.Create("second", "second checkpoint", msgs, nil)
	cm.Create("third", "third checkpoint", msgs, nil)

	list := cm.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 checkpoints, got %d", len(list))
	}
	if list[0].Name != "first" {
		t.Errorf("expected first checkpoint name 'first', got %q", list[0].Name)
	}
	if list[2].Name != "third" {
		t.Errorf("expected third checkpoint name 'third', got %q", list[2].Name)
	}
}

func TestCheckpointManager_Get(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	msgs := []Message{{Role: "user", Content: "hi"}}
	cp, _ := cm.Create("findme", "should be findable", msgs, nil)

	found := cm.Get(cp.ID)
	if found == nil {
		t.Fatal("expected to find checkpoint")
	}
	if found.Name != "findme" {
		t.Errorf("expected name 'findme', got %q", found.Name)
	}

	notFound := cm.Get("nonexistent")
	if notFound != nil {
		t.Error("expected nil for nonexistent checkpoint")
	}
}

func TestCheckpointManager_Delete(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	msgs := []Message{{Role: "user", Content: "hi"}}
	cp, _ := cm.Create("deleteme", "will be deleted", msgs, nil)

	err := cm.Delete(cp.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if cm.Get(cp.ID) != nil {
		t.Error("checkpoint still exists after deletion")
	}
	if len(cm.List()) != 0 {
		t.Errorf("expected 0 checkpoints after delete, got %d", len(cm.List()))
	}

	// Delete nonexistent
	err = cm.Delete("nonexistent")
	if err == nil {
		t.Error("expected error deleting nonexistent checkpoint")
	}
}

func TestCheckpointManager_ShouldAutoCheckpoint(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	// No checkpoints yet: should trigger at 10 messages
	if cm.ShouldAutoCheckpoint(5, 0) {
		t.Error("should not checkpoint at 5 messages with no prior checkpoints")
	}
	if !cm.ShouldAutoCheckpoint(10, 0) {
		t.Error("should checkpoint at 10 messages")
	}
	if !cm.ShouldAutoCheckpoint(3, 5) {
		t.Error("should checkpoint at 5 tool calls")
	}

	// After a checkpoint at 10 messages
	msgs := make([]Message, 10)
	for i := range msgs {
		msgs[i] = Message{Role: "user", Content: "msg"}
	}
	cm.Create("at10", "at 10 messages", msgs, nil)

	// 15 messages: only 5 since last, should not trigger
	if cm.ShouldAutoCheckpoint(15, 0) {
		t.Error("should not checkpoint at 15 (only 5 since last)")
	}
	// 20 messages: 10 since last, should trigger
	if !cm.ShouldAutoCheckpoint(20, 0) {
		t.Error("should checkpoint at 20 (10 since last)")
	}
	// Tool calls always trigger
	if !cm.ShouldAutoCheckpoint(12, 5) {
		t.Error("should checkpoint with 5 tool calls")
	}
}

func TestCheckpointManager_DiffFromCheckpoint(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	// Create files
	file1 := filepath.Join(dir, "unchanged.txt")
	file2 := filepath.Join(dir, "changed.txt")
	file3 := filepath.Join(dir, "new.txt")
	os.WriteFile(file1, []byte("same"), 0o644)
	os.WriteFile(file2, []byte("before"), 0o644)

	msgs := []Message{{Role: "user", Content: "hi"}}
	cp, _ := cm.Create("baseline", "baseline state", msgs, []string{file1, file2})

	// Modify file2, add file3
	os.WriteFile(file2, []byte("after"), 0o644)
	os.WriteFile(file3, []byte("brand new"), 0o644)

	diff := cm.DiffFromCheckpoint(cp.ID, []string{file1, file2, file3})

	// file2 changed, file3 is new
	hasChanged := false
	hasNew := false
	for _, d := range diff {
		if d == file2 {
			hasChanged = true
		}
		if d == file3 {
			hasNew = true
		}
	}
	if !hasChanged {
		t.Error("expected changed.txt in diff")
	}
	if !hasNew {
		t.Error("expected new.txt in diff")
	}

	// file1 should not be in diff
	for _, d := range diff {
		if d == file1 {
			t.Error("unchanged.txt should not be in diff")
		}
	}
}

func TestCheckpointManager_DiffFromCheckpoint_DeletedFile(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	file1 := filepath.Join(dir, "willdelete.txt")
	os.WriteFile(file1, []byte("content"), 0o644)

	msgs := []Message{{Role: "user", Content: "hi"}}
	cp, _ := cm.Create("before delete", "has file", msgs, []string{file1})

	// Remove the file from tracked files (simulating deletion)
	diff := cm.DiffFromCheckpoint(cp.ID, []string{})

	found := false
	for _, d := range diff {
		if strings.Contains(d, "willdelete.txt") && strings.Contains(d, "deleted") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected deleted file in diff, got: %v", diff)
	}
}

func TestCheckpointManager_FormatCheckpoints(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	// Empty
	out := cm.FormatCheckpoints()
	if out != "No checkpoints." {
		t.Errorf("expected 'No checkpoints.', got %q", out)
	}

	// Add some checkpoints
	msgs := []Message{{Role: "user", Content: "hi"}}
	cm.Create("before auth refactor", "desc", msgs, nil)
	cm.AutoCheckpoint(msgs, nil)
	cm.Create("tests passing", "desc", msgs, nil)

	out = cm.FormatCheckpoints()

	if !strings.Contains(out, "Checkpoints (3):") {
		t.Errorf("expected 'Checkpoints (3):' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "before auth refactor") {
		t.Errorf("expected checkpoint name in output, got:\n%s", out)
	}
	if !strings.Contains(out, "auto-checkpoint") {
		t.Errorf("expected 'auto-checkpoint' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "tests passing") {
		t.Errorf("expected 'tests passing' in output, got:\n%s", out)
	}
	// Check formatting contains separator
	if !strings.Contains(out, "─") {
		t.Errorf("expected separator in output, got:\n%s", out)
	}
}

func TestCheckpointManager_Prune(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)
	cm.MaxCheckpoints = 5

	msgs := []Message{{Role: "user", Content: "hi"}}

	// Create 3 named + 5 auto = 8 total (exceeds max of 5)
	cm.Create("named1", "first named", msgs, nil)
	cm.AutoCheckpoint(msgs, nil)
	cm.AutoCheckpoint(msgs, nil)
	cm.Create("named2", "second named", msgs, nil)
	cm.AutoCheckpoint(msgs, nil)
	cm.AutoCheckpoint(msgs, nil)
	cm.AutoCheckpoint(msgs, nil)
	cm.Create("named3", "third named", msgs, nil)

	cm.Prune()

	list := cm.List()
	// All named should be preserved
	namedCount := 0
	for _, cp := range list {
		if !cp.Auto {
			namedCount++
		}
	}
	if namedCount != 3 {
		t.Errorf("expected 3 named checkpoints preserved, got %d", namedCount)
	}
	// Total should be <= MaxCheckpoints
	if len(list) > cm.MaxCheckpoints {
		t.Errorf("expected <= %d checkpoints after prune, got %d", cm.MaxCheckpoints, len(list))
	}
}

func TestCheckpointManager_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	msgs := []Message{{Role: "user", Content: "hello world"}}
	cm.Create("persist test", "testing persistence", msgs, nil)
	cm.AutoCheckpoint(msgs, nil)

	if err := cm.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load into a new manager
	cm2 := NewCheckpointManager(dir)
	if err := cm2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(cm2.Checkpoints) != 2 {
		t.Fatalf("expected 2 checkpoints after load, got %d", len(cm2.Checkpoints))
	}
	if cm2.Checkpoints[0].Name != "persist test" {
		t.Errorf("expected name 'persist test', got %q", cm2.Checkpoints[0].Name)
	}
	if !cm2.Checkpoints[1].Auto {
		t.Error("expected second checkpoint to be auto")
	}
}

func TestCheckpointManager_Load_NoFile(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	// Should not error when no file exists
	if err := cm.Load(); err != nil {
		t.Fatalf("Load with no file should not error: %v", err)
	}
	if len(cm.Checkpoints) != 0 {
		t.Errorf("expected 0 checkpoints, got %d", len(cm.Checkpoints))
	}
}

func TestCheckpointManager_TokenEstimate(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	msgs := []Message{
		{Role: "user", Content: "This is a test message with some content"},
		{Role: "assistant", Content: "This is a response with more content here"},
	}

	cp, err := cm.Create("tokens", "token test", msgs, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if cp.TokenCount <= 0 {
		t.Errorf("expected positive token count, got %d", cp.TokenCount)
	}
}

func TestCheckpointManager_DiffFromCheckpoint_NotFound(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)

	diff := cm.DiffFromCheckpoint("nonexistent", []string{})
	if diff != nil {
		t.Errorf("expected nil diff for nonexistent checkpoint, got %v", diff)
	}
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "now"},
		{5 * time.Minute, "5m ago"},
		{2 * time.Hour, "2h ago"},
		{48 * time.Hour, "2d ago"},
	}

	for _, tt := range tests {
		got := formatAge(tt.d)
		if got != tt.want {
			t.Errorf("formatAge(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestGenerateCheckpointID(t *testing.T) {
	id1 := generateCheckpointID("test", time.Now())
	if len(id1) != 12 {
		t.Errorf("expected 12-char ID, got %d chars: %q", len(id1), id1)
	}

	// Different names should produce different IDs
	id2 := generateCheckpointID("other", time.Now())
	if id1 == id2 {
		t.Error("expected different IDs for different names")
	}
}

func TestHashFiles(t *testing.T) {
	dir := t.TempDir()

	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")
	os.WriteFile(f1, []byte("content a"), 0o644)
	os.WriteFile(f2, []byte("content b"), 0o644)

	state := hashFiles([]string{f1, f2})
	if len(state) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(state))
	}
	if state[f1] == "" {
		t.Error("expected non-empty hash for f1")
	}
	if state[f1] == state[f2] {
		t.Error("expected different hashes for different content")
	}

	// Nonexistent file should be skipped
	state = hashFiles([]string{"/nonexistent/file.txt"})
	if len(state) != 0 {
		t.Errorf("expected 0 entries for nonexistent file, got %d", len(state))
	}
}

func TestEstimateTokens(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "hello world test"},           // 16 chars -> 4 tokens
		{Role: "assistant", Content: "response here testing"}, // 21 chars -> 5 tokens
	}
	tokens := estimateTokens(msgs)
	if tokens <= 0 {
		t.Errorf("expected positive token count, got %d", tokens)
	}

	// Empty content but has messages should give minimum
	emptyMsgs := []Message{{Role: "user", Content: ""}}
	tokens = estimateTokens(emptyMsgs)
	if tokens < 1 {
		t.Errorf("expected at least 1 token for non-empty messages, got %d", tokens)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SmartCheckpointer tests (preserved from original)
// ─────────────────────────────────────────────────────────────────────────────

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
