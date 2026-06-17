package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupWorkspaceProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize git repo
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "config", "user.email", "test@test.com")
	runCmd(t, dir, "git", "config", "user.name", "Test")
	runCmd(t, dir, "git", "config", "commit.gpgsign", "false")
	runCmd(t, dir, "git", "config", "tag.gpgsign", "false")

	// Create project files
	os.MkdirAll(filepath.Join(dir, "pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "pkg", "util.go"), []byte("package pkg\n\nfunc Helper() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test Project\n"), 0o644)

	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "initial commit")

	return dir
}

func runCmd(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=commit.gpgsign",
		"GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=tag.gpgsign",
		"GIT_CONFIG_VALUE_1=false",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %s: %v", name, args, out, err)
	}
}

func TestCapture_SavesAllFiles(t *testing.T) {
	dir := setupWorkspaceProject(t)
	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	snap, err := store.Capture(dir, "test snapshot", "testing capture")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	if snap.ID == "" {
		t.Error("expected non-empty ID")
	}
	if snap.Name != "test snapshot" {
		t.Errorf("expected name 'test snapshot', got %q", snap.Name)
	}
	if snap.Description != "testing capture" {
		t.Errorf("expected description 'testing capture', got %q", snap.Description)
	}
	if snap.FileCount != 3 {
		t.Errorf("expected 3 files, got %d", snap.FileCount)
	}
	if snap.Size <= 0 {
		t.Error("expected positive size")
	}

	// Verify specific files are captured
	if _, ok := snap.Files["main.go"]; !ok {
		t.Error("main.go not captured")
	}
	if _, ok := snap.Files[filepath.Join("pkg", "util.go")]; !ok {
		t.Error("pkg/util.go not captured")
	}
	if _, ok := snap.Files["README.md"]; !ok {
		t.Error("README.md not captured")
	}
}

func TestCapture_SkipsIgnoredDirectories(t *testing.T) {
	dir := setupWorkspaceProject(t)

	// Create ignored directories with files
	os.MkdirAll(filepath.Join(dir, "node_modules", "express"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "express", "index.js"), []byte("module.exports = {};\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "vendor", "lib"), 0o755)
	os.WriteFile(filepath.Join(dir, "vendor", "lib", "dep.go"), []byte("package lib\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755)

	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	snap, err := store.Capture(dir, "ignore test", "")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	for path := range snap.Files {
		if strings.HasPrefix(path, "node_modules") {
			t.Errorf("node_modules should be skipped, found: %s", path)
		}
		if strings.HasPrefix(path, "vendor") {
			t.Errorf("vendor should be skipped, found: %s", path)
		}
		if strings.HasPrefix(path, ".git") {
			t.Errorf(".git should be skipped, found: %s", path)
		}
	}
}

func TestRestore_BringsBackOriginalState(t *testing.T) {
	dir := setupWorkspaceProject(t)
	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	// Capture original state
	snap, err := store.Capture(dir, "original", "before changes")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	// Make changes
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\n// CHANGED\nfunc main() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "new_file.go"), []byte("package main\n"), 0o644)
	if removeErr := os.Remove(filepath.Join(dir, "README.md")); removeErr != nil {
		t.Fatalf("remove README: %v", removeErr)
	}

	// Verify changes exist
	content, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if !strings.Contains(string(content), "CHANGED") {
		t.Fatal("modification should exist before restore")
	}

	// Restore
	err = store.Restore(snap.ID, dir)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// Verify original state restored
	content, err = os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal("main.go should exist after restore")
	}
	if string(content) != "package main\n\nfunc main() {}\n" {
		t.Errorf("main.go content not restored, got: %q", string(content))
	}

	// Verify new file was deleted
	if _, statErr := os.Stat(filepath.Join(dir, "new_file.go")); !os.IsNotExist(statErr) {
		t.Error("new_file.go should be deleted after restore")
	}

	// Verify deleted file was restored
	content, err = os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatal("README.md should be restored")
	}
	if string(content) != "# Test Project\n" {
		t.Errorf("README.md content not restored, got: %q", string(content))
	}
}

func TestDiff_DetectsChanges(t *testing.T) {
	dir := setupWorkspaceProject(t)
	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	// Capture baseline
	snap, err := store.Capture(dir, "baseline", "")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	// Make changes: modify, add, delete
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\n// modified\nfunc main() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "added.go"), []byte("package main\n"), 0o644)
	os.Remove(filepath.Join(dir, "README.md"))

	// Diff
	diff, err := store.Diff(snap.ID, dir)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	if len(diff.Added) != 1 || diff.Added[0] != "added.go" {
		t.Errorf("expected 1 added file (added.go), got: %v", diff.Added)
	}
	if len(diff.Modified) != 1 || diff.Modified[0] != "main.go" {
		t.Errorf("expected 1 modified file (main.go), got: %v", diff.Modified)
	}
	if len(diff.Deleted) != 1 || diff.Deleted[0] != "README.md" {
		t.Errorf("expected 1 deleted file (README.md), got: %v", diff.Deleted)
	}
	if diff.Unchanged != 1 {
		t.Errorf("expected 1 unchanged file, got %d", diff.Unchanged)
	}
}

func TestList_ReturnsSortedByDate(t *testing.T) {
	dir := setupWorkspaceProject(t)
	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	// Create multiple snapshots with slight delays
	_, err := store.Capture(dir, "first", "")
	if err != nil {
		t.Fatalf("Capture 1 failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("// v2\npackage main\n"), 0o644)
	_, err = store.Capture(dir, "second", "")
	if err != nil {
		t.Fatalf("Capture 2 failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("// v3\npackage main\n"), 0o644)
	_, err = store.Capture(dir, "third", "")
	if err != nil {
		t.Fatalf("Capture 3 failed: %v", err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(list))
	}

	// Should be newest first
	if list[0].Name != "third" {
		t.Errorf("expected newest first, got %q", list[0].Name)
	}
	if list[1].Name != "second" {
		t.Errorf("expected second, got %q", list[1].Name)
	}
	if list[2].Name != "first" {
		t.Errorf("expected oldest last, got %q", list[2].Name)
	}
}

func TestDelete_RemovesSnapshot(t *testing.T) {
	dir := setupWorkspaceProject(t)
	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	snap, err := store.Capture(dir, "to-delete", "")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	// Verify it exists
	_, err = store.Get(snap.ID)
	if err != nil {
		t.Fatal("snapshot should exist before delete")
	}

	// Delete
	err = store.Delete(snap.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	_, err = store.Get(snap.ID)
	if err == nil {
		t.Error("snapshot should not exist after delete")
	}
}

func TestPrune_RespectsMaxSnapshots(t *testing.T) {
	dir := setupWorkspaceProject(t)
	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)
	store.MaxSnapshots = 3

	// Create 5 snapshots
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main // "+string(rune('a'+i))+"\n"), 0o644)
		time.Sleep(10 * time.Millisecond)
		_, err := store.Capture(dir, "snap-"+string(rune('a'+i)), "")
		if err != nil {
			t.Fatalf("Capture %d failed: %v", i, err)
		}
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(list) > 3 {
		t.Errorf("expected at most 3 snapshots after prune, got %d", len(list))
	}

	// Verify the newest snapshots are kept
	for _, s := range list {
		if s.Name == "snap-a" || s.Name == "snap-b" {
			t.Errorf("oldest snapshots should be pruned, found %q", s.Name)
		}
	}
}

func TestSHA256_Integrity(t *testing.T) {
	dir := setupWorkspaceProject(t)
	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	snap, err := store.Capture(dir, "hash-test", "")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	// Verify hashes are correct
	for rel, fs := range snap.Files {
		content, readErr := os.ReadFile(filepath.Join(dir, rel))
		if readErr != nil {
			t.Fatalf("reading %s: %v", rel, readErr)
		}
		hash := sha256.Sum256(content)
		expected := hex.EncodeToString(hash[:])
		if fs.Hash != expected {
			t.Errorf("hash mismatch for %s: got %s, want %s", rel, fs.Hash, expected)
		}
	}
}

func TestGitState_Captured(t *testing.T) {
	dir := setupWorkspaceProject(t)

	// Create and switch to a branch
	runCmd(t, dir, "git", "checkout", "-b", "feature-branch")
	os.WriteFile(filepath.Join(dir, "feature.go"), []byte("package main\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "feature work")

	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	snap, err := store.Capture(dir, "git-state", "")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	if snap.GitBranch != "feature-branch" {
		t.Errorf("expected branch 'feature-branch', got %q", snap.GitBranch)
	}
	if snap.GitCommit == "" {
		t.Error("expected non-empty git commit")
	}
	if len(snap.GitCommit) != 40 {
		t.Errorf("expected 40-char commit hash, got %d chars: %q", len(snap.GitCommit), snap.GitCommit)
	}
}

func TestEmptyProject(t *testing.T) {
	dir := t.TempDir()
	// Initialize git but no files
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "config", "user.email", "test@test.com")
	runCmd(t, dir, "git", "config", "user.name", "Test")
	runCmd(t, dir, "git", "config", "commit.gpgsign", "false")
	runCmd(t, dir, "git", "config", "tag.gpgsign", "false")

	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	snap, err := store.Capture(dir, "empty", "empty project")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	if snap.FileCount != 0 {
		t.Errorf("expected 0 files for empty project, got %d", snap.FileCount)
	}
	if snap.Size != 0 {
		t.Errorf("expected 0 size for empty project, got %d", snap.Size)
	}
}

func TestLargeFile_Handling(t *testing.T) {
	dir := t.TempDir()
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "config", "user.email", "test@test.com")
	runCmd(t, dir, "git", "config", "user.name", "Test")
	runCmd(t, dir, "git", "config", "commit.gpgsign", "false")
	runCmd(t, dir, "git", "config", "tag.gpgsign", "false")

	// Create a 1MB file
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}
	os.WriteFile(filepath.Join(dir, "large.bin"), largeContent, 0o644)

	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	snap, err := store.Capture(dir, "large-file", "")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	if snap.Size != int64(len(largeContent)) {
		t.Errorf("expected size %d, got %d", len(largeContent), snap.Size)
	}

	// Verify it can be restored
	os.Remove(filepath.Join(dir, "large.bin"))
	err = store.Restore(snap.ID, dir)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	restored, err := os.ReadFile(filepath.Join(dir, "large.bin"))
	if err != nil {
		t.Fatal("large.bin should exist after restore")
	}
	if len(restored) != len(largeContent) {
		t.Errorf("restored size mismatch: got %d, want %d", len(restored), len(largeContent))
	}

	// Verify content matches via hash
	origHash := sha256.Sum256(largeContent)
	restoredHash := sha256.Sum256(restored)
	if origHash != restoredHash {
		t.Error("restored content hash does not match original")
	}
}

func TestFormatList_Output(t *testing.T) {
	now := time.Now()
	snapshots := []*WorkspaceSnapshot{
		{
			ID:        "abc123def456",
			Name:      "before auth refactor",
			CreatedAt: now.Add(-2 * time.Minute),
			FileCount: 45,
			Size:      128 * 1024,
		},
		{
			ID:        "def456ghi789",
			Name:      "working state",
			CreatedAt: now.Add(-15 * time.Minute),
			FileCount: 42,
			Size:      115 * 1024,
		},
		{
			ID:        "ghi789jkl012",
			Name:      "pre-migration",
			CreatedAt: now.Add(-1 * time.Hour),
			FileCount: 40,
			Size:      110 * 1024,
		},
	}

	output := FormatList(snapshots)

	if !strings.Contains(output, "Snapshots:") {
		t.Error("expected header 'Snapshots:'")
	}
	if !strings.Contains(output, "abc123d") {
		t.Error("expected truncated ID 'abc123d'")
	}
	if !strings.Contains(output, "before auth refactor") {
		t.Error("expected snapshot name")
	}
	if !strings.Contains(output, "45 files") {
		t.Error("expected file count")
	}
	if !strings.Contains(output, "128KB") {
		t.Error("expected size")
	}
	if !strings.Contains(output, "2m ago") {
		t.Error("expected age '2m ago'")
	}
	if !strings.Contains(output, "1h ago") {
		t.Error("expected age '1h ago'")
	}

	// Test empty list
	emptyOutput := FormatList(nil)
	if !strings.Contains(emptyOutput, "No snapshots found") {
		t.Error("expected 'No snapshots found' for empty list")
	}
}

func TestFormatDiff_Output(t *testing.T) {
	diff := &SnapshotDiff{
		Added:     []string{"new1.go", "new2.go", "new3.go"},
		Modified:  []string{"main.go", "pkg/util.go", "config.go", "handler.go", "db.go"},
		Deleted:   []string{"old.go"},
		Unchanged: 37,
	}

	output := FormatDiff(diff, "before auth refactor")

	if !strings.Contains(output, `Changes since snapshot "before auth refactor"`) {
		t.Error("expected header with snapshot name")
	}
	if !strings.Contains(output, "Added: 3 files") {
		t.Error("expected 'Added: 3 files'")
	}
	if !strings.Contains(output, "Modified: 5 files") {
		t.Error("expected 'Modified: 5 files'")
	}
	if !strings.Contains(output, "Deleted: 1 file") {
		t.Errorf("expected 'Deleted: 1 file' (singular), got output: %s", output)
	}
	if !strings.Contains(output, "Unchanged: 37 files") {
		t.Error("expected 'Unchanged: 37 files'")
	}
}

func TestGet_ReturnsFullSnapshot(t *testing.T) {
	dir := setupWorkspaceProject(t)
	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	snap, err := store.Capture(dir, "get-test", "full retrieval")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	retrieved, err := store.Get(snap.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.ID != snap.ID {
		t.Errorf("ID mismatch: got %q, want %q", retrieved.ID, snap.ID)
	}
	if retrieved.Name != "get-test" {
		t.Errorf("Name mismatch: got %q", retrieved.Name)
	}
	if retrieved.FileCount != snap.FileCount {
		t.Errorf("FileCount mismatch: got %d, want %d", retrieved.FileCount, snap.FileCount)
	}
	if len(retrieved.Files) != len(snap.Files) {
		t.Errorf("Files count mismatch: got %d, want %d", len(retrieved.Files), len(snap.Files))
	}

	// Verify file content is fully available
	mainGo, ok := retrieved.Files["main.go"]
	if !ok {
		t.Fatal("main.go not found in retrieved snapshot")
	}
	if string(mainGo.Content) != "package main\n\nfunc main() {}\n" {
		t.Errorf("unexpected content: %q", string(mainGo.Content))
	}
}

func TestDelete_NonExistent(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)
	os.MkdirAll(storeDir, 0o755)

	err := store.Delete("nonexistent-id")
	if err == nil {
		t.Error("expected error when deleting non-existent snapshot")
	}
}

func TestRestore_PreservesGitDir(t *testing.T) {
	dir := setupWorkspaceProject(t)
	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	snap, err := store.Capture(dir, "preserve-git", "")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	// The .git directory should still exist after restore
	err = store.Restore(snap.ID, dir)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		t.Error(".git directory should be preserved after restore")
	}
}

func TestNewSnapshotStore_DefaultDir(t *testing.T) {
	store := NewSnapshotStore("")
	// L2: the default path is now home-relative (~/.hawk/snapshots) so
	// state stops leaking into <cwd>/.hawk/ when hawk is run from
	// inside a Go project root.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}
	expected := filepath.Join(home, ".hawk", "snapshots")
	if store.Dir != expected {
		t.Errorf("expected default dir %q, got %q", expected, store.Dir)
	}
	if store.MaxSnapshots != 20 {
		t.Errorf("expected MaxSnapshots 20, got %d", store.MaxSnapshots)
	}
}

func TestCapture_FileMode(t *testing.T) {
	dir := t.TempDir()
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "config", "user.email", "test@test.com")
	runCmd(t, dir, "git", "config", "user.name", "Test")
	runCmd(t, dir, "git", "config", "commit.gpgsign", "false")
	runCmd(t, dir, "git", "config", "tag.gpgsign", "false")

	// Create file with specific mode
	os.WriteFile(filepath.Join(dir, "script.sh"), []byte("#!/bin/bash\necho hello\n"), 0o755)

	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	snap, err := store.Capture(dir, "mode-test", "")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	fs, ok := snap.Files["script.sh"]
	if !ok {
		t.Fatal("script.sh not captured")
	}
	if fs.Mode&0o111 == 0 {
		t.Error("expected executable mode to be preserved")
	}
}

func TestDiff_NoChanges(t *testing.T) {
	dir := setupWorkspaceProject(t)
	storeDir := filepath.Join(t.TempDir(), "snapshots")
	store := NewSnapshotStore(storeDir)

	snap, err := store.Capture(dir, "no-changes", "")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	diff, err := store.Diff(snap.ID, dir)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	if len(diff.Added) != 0 {
		t.Errorf("expected 0 added, got %d", len(diff.Added))
	}
	if len(diff.Modified) != 0 {
		t.Errorf("expected 0 modified, got %d", len(diff.Modified))
	}
	if len(diff.Deleted) != 0 {
		t.Errorf("expected 0 deleted, got %d", len(diff.Deleted))
	}
	if diff.Unchanged != 3 {
		t.Errorf("expected 3 unchanged, got %d", diff.Unchanged)
	}
}
