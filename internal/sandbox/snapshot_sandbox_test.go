package sandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewSandboxManager(t *testing.T) {
	dir := t.TempDir()
	mgr := NewSandboxManager(dir)
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.Dir != dir {
		t.Errorf("expected dir %q, got %q", dir, mgr.Dir)
	}
	if mgr.MaxSandboxes != 64 {
		t.Errorf("expected MaxSandboxes 64, got %d", mgr.MaxSandboxes)
	}
	if len(mgr.Sandboxes) != 0 {
		t.Errorf("expected empty sandboxes map")
	}
}

func TestRestoreRejectsTraversalPath(t *testing.T) {
	workDir := t.TempDir()
	outside := filepath.Join(filepath.Dir(workDir), "escape.txt")
	mgr := NewSandboxManager(t.TempDir())
	data, err := json.Marshal(SandboxState{
		ID:      "sb-restore",
		WorkDir: workDir,
		Files:   map[string][]byte{"../escape.txt": []byte("owned")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Restore(data); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside file exists after rejected restore: %v", err)
	}
}

func TestRestoreRejectsSymlinkPath(t *testing.T) {
	workDir := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(workDir, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	mgr := NewSandboxManager(t.TempDir())
	data, err := json.Marshal(SandboxState{
		ID:      "sb-restore",
		WorkDir: workDir,
		Files:   map[string][]byte{"linked/escape.txt": []byte("owned")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Restore(data); err == nil {
		t.Fatal("expected symlink path to be rejected")
	}
	if _, err := os.Stat(filepath.Join(target, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("symlink target was modified after rejected restore: %v", err)
	}
}

func TestCreateSandbox(t *testing.T) {
	dir := t.TempDir()
	workDir := t.TempDir()

	// Write some files to the work dir.
	os.WriteFile(filepath.Join(workDir, "main.go"), []byte("package main"), 0o644)
	os.MkdirAll(filepath.Join(workDir, "pkg"), 0o755)
	os.WriteFile(filepath.Join(workDir, "pkg/lib.go"), []byte("package pkg"), 0o644)

	mgr := NewSandboxManager(dir)
	sb, err := mgr.Create(workDir, map[string]string{"GOPATH": "/go"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if !strings.HasPrefix(sb.ID, "sb-") {
		t.Errorf("expected ID prefix sb-, got %q", sb.ID)
	}
	if sb.WorkDir != workDir {
		t.Errorf("expected WorkDir %q, got %q", workDir, sb.WorkDir)
	}
	if sb.Status != "running" {
		t.Errorf("expected status running, got %q", sb.Status)
	}
	if sb.EnvVars["GOPATH"] != "/go" {
		t.Errorf("expected GOPATH=/go, got %q", sb.EnvVars["GOPATH"])
	}
	if len(sb.Files) != 2 {
		t.Errorf("expected 2 files captured, got %d", len(sb.Files))
	}
	if string(sb.Files["main.go"]) != "package main" {
		t.Errorf("unexpected file content for main.go")
	}
}

func TestCreateSandboxMaxLimit(t *testing.T) {
	dir := t.TempDir()
	mgr := NewSandboxManager(dir)
	mgr.MaxSandboxes = 2

	workDir := t.TempDir()
	_, err := mgr.Create(workDir, nil)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	_, err = mgr.Create(workDir, nil)
	if err != nil {
		t.Fatalf("second create failed: %v", err)
	}
	_, err = mgr.Create(workDir, nil)
	if err == nil {
		t.Fatal("expected error on third create")
	}
	if !strings.Contains(err.Error(), "maximum sandbox limit") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPauseAndResume(t *testing.T) {
	dir := t.TempDir()
	workDir := t.TempDir()

	os.WriteFile(filepath.Join(workDir, "file.txt"), []byte("hello"), 0o644)

	mgr := NewSandboxManager(dir)
	sb, err := mgr.Create(workDir, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Modify a file before pausing.
	os.WriteFile(filepath.Join(workDir, "file.txt"), []byte("modified"), 0o644)
	os.WriteFile(filepath.Join(workDir, "new.txt"), []byte("new file"), 0o644)

	// Pause.
	if pauseErr := mgr.Pause(sb.ID); pauseErr != nil {
		t.Fatalf("Pause failed: %v", pauseErr)
	}
	if sb.Status != "paused" {
		t.Errorf("expected status paused, got %q", sb.Status)
	}
	if sb.PausedAt == nil {
		t.Error("expected PausedAt to be set")
	}

	// Verify persisted to disk.
	path := filepath.Join(dir, sb.ID+".json")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("expected persisted file at %s: %v", path, statErr)
	}

	// Wipe work dir to simulate environment teardown.
	os.RemoveAll(workDir)
	os.MkdirAll(workDir, 0o755)

	// Resume.
	resumed, err := mgr.Resume(sb.ID)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if resumed.Status != "running" {
		t.Errorf("expected status running, got %q", resumed.Status)
	}
	if resumed.ResumedAt == nil {
		t.Error("expected ResumedAt to be set")
	}

	// Verify files restored.
	content, err := os.ReadFile(filepath.Join(workDir, "file.txt"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(content) != "modified" {
		t.Errorf("expected modified content, got %q", string(content))
	}
	content, err = os.ReadFile(filepath.Join(workDir, "new.txt"))
	if err != nil {
		t.Fatalf("read new restored file: %v", err)
	}
	if string(content) != "new file" {
		t.Errorf("expected 'new file', got %q", string(content))
	}
}

func TestPauseErrors(t *testing.T) {
	dir := t.TempDir()
	mgr := NewSandboxManager(dir)

	// Pause nonexistent.
	if err := mgr.Pause("sb-nonexistent"); err == nil {
		t.Error("expected error for nonexistent sandbox")
	}

	// Pause already paused.
	workDir := t.TempDir()
	sb, _ := mgr.Create(workDir, nil)
	mgr.Pause(sb.ID)
	if err := mgr.Pause(sb.ID); err == nil {
		t.Error("expected error for pausing already-paused sandbox")
	}
}

func TestResumeFromDisk(t *testing.T) {
	dir := t.TempDir()
	workDir := t.TempDir()

	os.WriteFile(filepath.Join(workDir, "data.txt"), []byte("important"), 0o644)

	// Create and pause with one manager.
	mgr1 := NewSandboxManager(dir)
	sb, _ := mgr1.Create(workDir, map[string]string{"KEY": "VAL"})
	mgr1.Pause(sb.ID)

	// Simulate restart: new manager, no in-memory state.
	mgr2 := NewSandboxManager(dir)

	// Wipe work dir.
	os.RemoveAll(workDir)
	os.MkdirAll(workDir, 0o755)

	// Resume from disk.
	resumed, err := mgr2.Resume(sb.ID)
	if err != nil {
		t.Fatalf("Resume from disk failed: %v", err)
	}
	if resumed.EnvVars["KEY"] != "VAL" {
		t.Errorf("expected env KEY=VAL, got %q", resumed.EnvVars["KEY"])
	}

	content, _ := os.ReadFile(filepath.Join(workDir, "data.txt"))
	if string(content) != "important" {
		t.Errorf("expected restored data, got %q", string(content))
	}
}

func TestSnapshot(t *testing.T) {
	dir := t.TempDir()
	workDir := t.TempDir()

	os.WriteFile(filepath.Join(workDir, "app.go"), []byte("package app"), 0o644)

	mgr := NewSandboxManager(dir)
	sb, _ := mgr.Create(workDir, map[string]string{"ENV": "test"})

	data, err := mgr.Snapshot(sb.ID)
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	// Verify it's valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("snapshot is not valid JSON: %v", err)
	}
	if parsed["id"] != sb.ID {
		t.Errorf("expected id %q in snapshot", sb.ID)
	}
	if parsed["status"] != "running" {
		t.Errorf("expected status running in snapshot")
	}
}

func TestSnapshotNotFound(t *testing.T) {
	dir := t.TempDir()
	mgr := NewSandboxManager(dir)
	_, err := mgr.Snapshot("sb-nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent sandbox")
	}
}

func TestRestore(t *testing.T) {
	dir := t.TempDir()
	workDir := t.TempDir()

	os.WriteFile(filepath.Join(workDir, "config.yaml"), []byte("key: value"), 0o644)

	mgr := NewSandboxManager(dir)
	sb, _ := mgr.Create(workDir, map[string]string{"MODE": "dev"})

	// Take snapshot.
	data, _ := mgr.Snapshot(sb.ID)

	// Use a new work dir for restore.
	newWorkDir := t.TempDir()

	// Modify snapshot to point to new dir.
	var state SandboxState
	json.Unmarshal(data, &state)
	state.WorkDir = newWorkDir
	modifiedData, _ := json.Marshal(state)

	// Restore.
	restored, err := mgr.Restore(modifiedData)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if restored.Status != "running" {
		t.Errorf("expected status running, got %q", restored.Status)
	}
	if restored.EnvVars["MODE"] != "dev" {
		t.Errorf("expected MODE=dev, got %q", restored.EnvVars["MODE"])
	}

	content, _ := os.ReadFile(filepath.Join(newWorkDir, "config.yaml"))
	if string(content) != "key: value" {
		t.Errorf("expected restored config, got %q", string(content))
	}
}

func TestRestoreInvalidData(t *testing.T) {
	dir := t.TempDir()
	mgr := NewSandboxManager(dir)
	_, err := mgr.Restore([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid data")
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	mgr := NewSandboxManager(dir)

	// Empty list.
	if len(mgr.List()) != 0 {
		t.Error("expected empty list")
	}

	workDir := t.TempDir()
	sb1, _ := mgr.Create(workDir, nil)
	time.Sleep(10 * time.Millisecond) // Ensure different creation times.
	sb2, _ := mgr.Create(workDir, nil)

	list := mgr.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 sandboxes, got %d", len(list))
	}
	// Newest first.
	if list[0].ID != sb2.ID {
		t.Errorf("expected newest first, got %q", list[0].ID)
	}
	if list[1].ID != sb1.ID {
		t.Errorf("expected oldest second, got %q", list[1].ID)
	}
}

func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	mgr := NewSandboxManager(dir)
	workDir := t.TempDir()

	sb, _ := mgr.Create(workDir, nil)
	// Artificially age the sandbox.
	sb.CreatedAt = time.Now().Add(-2 * time.Hour)

	// Persist it.
	os.MkdirAll(dir, 0o755)
	data, _ := json.Marshal(sb)
	os.WriteFile(filepath.Join(dir, sb.ID+".json"), data, 0o644)

	// Create a fresh one that should survive cleanup.
	fresh, _ := mgr.Create(workDir, nil)

	err := mgr.Cleanup(1 * time.Hour)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if len(mgr.Sandboxes) != 1 {
		t.Errorf("expected 1 sandbox remaining, got %d", len(mgr.Sandboxes))
	}
	if _, ok := mgr.Sandboxes[fresh.ID]; !ok {
		t.Error("fresh sandbox should survive cleanup")
	}
	if _, ok := mgr.Sandboxes[sb.ID]; ok {
		t.Error("old sandbox should be cleaned up")
	}

	// Verify persisted file removed.
	if _, err := os.Stat(filepath.Join(dir, sb.ID+".json")); !os.IsNotExist(err) {
		t.Error("expected persisted file to be removed")
	}
}

func TestDiffSandbox(t *testing.T) {
	dir := t.TempDir()
	workDir := t.TempDir()

	os.WriteFile(filepath.Join(workDir, "a.txt"), []byte("original"), 0o644)
	os.WriteFile(filepath.Join(workDir, "b.txt"), []byte("keep"), 0o644)

	mgr := NewSandboxManager(dir)
	sb, _ := mgr.Create(workDir, nil)

	// Modify files.
	os.WriteFile(filepath.Join(workDir, "a.txt"), []byte("changed"), 0o644)
	os.Remove(filepath.Join(workDir, "b.txt"))
	os.WriteFile(filepath.Join(workDir, "c.txt"), []byte("new"), 0o644)

	diffs, err := mgr.DiffSandbox(sb.ID)
	if err != nil {
		t.Fatalf("DiffSandbox failed: %v", err)
	}

	if len(diffs) != 3 {
		t.Fatalf("expected 3 diffs, got %d: %v", len(diffs), diffs)
	}

	// Sorted alphabetically.
	expected := []string{
		"added: c.txt",
		"deleted: b.txt",
		"modified: a.txt",
	}
	for i, want := range expected {
		if diffs[i] != want {
			t.Errorf("diff[%d]: expected %q, got %q", i, want, diffs[i])
		}
	}
}

func TestDiffSandboxNotFound(t *testing.T) {
	dir := t.TempDir()
	mgr := NewSandboxManager(dir)
	_, err := mgr.DiffSandbox("sb-nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent sandbox")
	}
}

func TestDiffSandboxNoChanges(t *testing.T) {
	dir := t.TempDir()
	workDir := t.TempDir()

	os.WriteFile(filepath.Join(workDir, "static.txt"), []byte("unchanged"), 0o644)

	mgr := NewSandboxManager(dir)
	sb, _ := mgr.Create(workDir, nil)

	diffs, err := mgr.DiffSandbox(sb.ID)
	if err != nil {
		t.Fatalf("DiffSandbox failed: %v", err)
	}
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %v", diffs)
	}
}

func TestFormatStatus(t *testing.T) {
	dir := t.TempDir()
	mgr := NewSandboxManager(dir)

	// Empty.
	out := mgr.FormatStatus()
	if !strings.Contains(out, "Sandboxes (0)") {
		t.Errorf("expected empty status header, got %q", out)
	}

	workDir := t.TempDir()
	sb1, _ := mgr.Create(workDir, nil)
	sb2, _ := mgr.Create(workDir, nil)
	mgr.Pause(sb2.ID)

	// Add a terminated one.
	sb3, _ := mgr.Create(workDir, nil)
	sb3.Status = "terminated"

	out = mgr.FormatStatus()
	if !strings.Contains(out, "Sandboxes (3)") {
		t.Errorf("expected 3 sandboxes in status, got %q", out)
	}
	if !strings.Contains(out, "[running] "+sb1.ID) {
		t.Errorf("expected running sandbox in output")
	}
	if !strings.Contains(out, "[paused] "+sb2.ID) {
		t.Errorf("expected paused sandbox in output")
	}
	if !strings.Contains(out, "[terminated] "+sb3.ID) {
		t.Errorf("expected terminated sandbox in output")
	}
	if !strings.Contains(out, "Resumable") {
		t.Errorf("expected Resumable for paused sandbox")
	}
	if !strings.Contains(out, "Cleanup eligible") {
		t.Errorf("expected Cleanup eligible for terminated sandbox")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestCaptureFilesSkipsHidden(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("gitconfig"), 0o644)
	os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("yes"), 0o644)

	files, err := captureFiles(dir)
	if err != nil {
		t.Fatalf("captureFiles failed: %v", err)
	}
	if _, ok := files[".git/config"]; ok {
		t.Error("should not capture hidden directory files")
	}
	if _, ok := files["visible.txt"]; !ok {
		t.Error("should capture visible files")
	}
}

func TestCaptureFilesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := captureFiles(dir)
	if err != nil {
		t.Fatalf("captureFiles failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected empty map, got %d files", len(files))
	}
}

func TestCaptureFilesNonexistent(t *testing.T) {
	files, err := captureFiles("/nonexistent/path/xyz")
	if err != nil {
		t.Fatalf("captureFiles should not error for nonexistent: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected empty map for nonexistent dir")
	}
}

func TestGenerateID(t *testing.T) {
	id1, err := generateID()
	if err != nil {
		t.Fatalf("generateID failed: %v", err)
	}
	id2, _ := generateID()

	if !strings.HasPrefix(id1, "sb-") {
		t.Errorf("expected sb- prefix, got %q", id1)
	}
	if len(id1) != 15 { // "sb-" + 12 hex chars
		t.Errorf("expected length 15, got %d for %q", len(id1), id1)
	}
	if id1 == id2 {
		t.Error("expected unique IDs")
	}
}

func TestCopyFileMap(t *testing.T) {
	original := map[string][]byte{
		"a.txt": []byte("content a"),
		"b.txt": []byte("content b"),
	}
	copied := copyFileMap(original)

	// Modify original.
	original["a.txt"][0] = 'X'

	if copied["a.txt"][0] == 'X' {
		t.Error("copy should be independent of original")
	}

	// Nil case.
	if copyFileMap(nil) != nil {
		t.Error("expected nil for nil input")
	}
}
