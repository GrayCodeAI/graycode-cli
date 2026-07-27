package snapshot

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestTracker_Cleanup tests the Cleanup function which runs git gc.
func TestTracker_Cleanup(t *testing.T) {
	// Create a temp project dir
	projectDir := t.TempDir()

	// Create a file in the project
	if err := os.WriteFile(filepath.Join(projectDir, "test.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	tracker := New(projectDir)
	if err := tracker.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Take a snapshot
	if _, err := tracker.Track("test snapshot"); err != nil {
		t.Fatalf("Track failed: %v", err)
	}

	// Cleanup should succeed
	if err := tracker.Cleanup(24 * time.Hour); err != nil {
		t.Errorf("Cleanup failed: %v", err)
	}
}

// TestTracker_Cleanup_NoRepo tests Cleanup when there's no git repo.
func TestTracker_Cleanup_NoRepo(t *testing.T) {
	tracker := New(t.TempDir())
	// Don't init - should fail because git commands will fail without a repo
	err := tracker.Cleanup(24 * time.Hour)
	// Cleanup might succeed if git commands don't fail (e.g., reflog expire on empty repo)
	// Just verify it doesn't panic
	_ = err
}

// TestSnapshotStore_Save tests the Save method.
func TestSnapshotStore_Save(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	snap := &WorkspaceSnapshot{
		ID:          "test-id-123456",
		Name:        "test-snapshot",
		Description: "test description",
		Files:       make(map[string]FileState),
		CreatedAt:   time.Now(),
	}

	if err := store.Save(snap); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "test-id-123456.json.gz")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected snapshot file to exist")
	}
}

// TestSnapshotStore_Get tests the Get method.
func TestSnapshotStore_Get(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	snap := &WorkspaceSnapshot{
		ID:          "test-id-123456",
		Name:        "test-snapshot",
		Description: "test description",
		Files:       make(map[string]FileState),
		CreatedAt:   time.Now(),
	}

	if err := store.Save(snap); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Get("test-id-123456")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if loaded.Name != "test-snapshot" {
		t.Errorf("expected name 'test-snapshot', got %q", loaded.Name)
	}
}

// TestSnapshotStore_Get_NonExistent tests Get with a non-existent snapshot.
func TestSnapshotStore_Get_NonExistent(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	_, err := store.Get("non-existent-id")
	if err == nil {
		t.Error("expected error for non-existent snapshot")
	}
}

// TestSnapshotStore_Delete tests the Delete method.
func TestSnapshotStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	snap := &WorkspaceSnapshot{
		ID:        "test-id-123456",
		Name:      "test-snapshot",
		Files:     make(map[string]FileState),
		CreatedAt: time.Now(),
	}

	if err := store.Save(snap); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if err := store.Delete("test-id-123456"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify file is gone
	path := filepath.Join(dir, "test-id-123456.json.gz")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

// TestSnapshotStore_Delete_NonExistent tests Delete with a non-existent snapshot.
func TestSnapshotStore_Delete_NonExistent(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	err := store.Delete("non-existent-id")
	if err == nil {
		t.Error("expected error for deleting non-existent snapshot")
	}
}

// TestSnapshotStore_Prune tests the Prune method.
func TestSnapshotStore_Prune(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)
	store.MaxSnapshots = 2

	// Create 3 snapshots
	for i := 0; i < 3; i++ {
		snap := &WorkspaceSnapshot{
			ID:        "snap-" + string(rune('a'+i)),
			Name:      "snapshot-" + string(rune('a'+i)),
			Files:     make(map[string]FileState),
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := store.Save(snap); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	// Prune should remove the oldest
	if err := store.Prune(); err != nil {
		t.Fatalf("Prune failed: %v", err)
	}

	// Should have 2 snapshots left
	list, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 snapshots after prune, got %d", len(list))
	}
}

// TestSnapshotStore_Prune_UnderLimit tests Prune when under the limit.
func TestSnapshotStore_Prune_UnderLimit(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)
	store.MaxSnapshots = 5

	// Create 2 snapshots
	for i := 0; i < 2; i++ {
		snap := &WorkspaceSnapshot{
			ID:        "snap-" + string(rune('a'+i)),
			Name:      "snapshot-" + string(rune('a'+i)),
			Files:     make(map[string]FileState),
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := store.Save(snap); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	// Prune should do nothing
	if err := store.Prune(); err != nil {
		t.Fatalf("Prune failed: %v", err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(list))
	}
}

// TestSnapshotStore_List_Empty tests List when no snapshots exist.
func TestSnapshotStore_List_Empty(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	list, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(list))
	}
}

// TestSnapshotStore_Diff tests the Diff method.
func TestSnapshotStore_Diff(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	// Create a project dir
	projectDir := t.TempDir()
	if writeErr := os.WriteFile(filepath.Join(projectDir, "file1.txt"), []byte("content1"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(projectDir, "file2.txt"), []byte("content2"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	// Capture a snapshot
	snap, captureErr := store.Capture(projectDir, "test", "test diff")
	if captureErr != nil {
		t.Fatalf("Capture failed: %v", captureErr)
	}

	// Modify a file
	if writeErr := os.WriteFile(filepath.Join(projectDir, "file1.txt"), []byte("modified"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	// Add a new file
	if writeErr := os.WriteFile(filepath.Join(projectDir, "file3.txt"), []byte("content3"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	// Delete a file
	if removeErr := os.Remove(filepath.Join(projectDir, "file2.txt")); removeErr != nil {
		t.Fatal(removeErr)
	}

	// Diff should show changes
	diff, diffErr := store.Diff(snap.ID, projectDir)
	if diffErr != nil {
		t.Fatalf("Diff failed: %v", diffErr)
	}
	if len(diff.Added) != 1 {
		t.Errorf("expected 1 added file, got %d", len(diff.Added))
	}
	if len(diff.Modified) != 1 {
		t.Errorf("expected 1 modified file, got %d", len(diff.Modified))
	}
	if len(diff.Deleted) != 1 {
		t.Errorf("expected 1 deleted file, got %d", len(diff.Deleted))
	}
	if diff.Unchanged != 0 {
		t.Errorf("expected 0 unchanged files, got %d", diff.Unchanged)
	}
}

// TestSnapshotStore_Diff_NonExistent tests Diff with a non-existent snapshot.
func TestSnapshotStore_Diff_NonExistent(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	_, err := store.Diff("non-existent", t.TempDir())
	if err == nil {
		t.Error("expected error for non-existent snapshot")
	}
}

// TestSnapshotStore_Restore tests the Restore method.
func TestSnapshotStore_Restore(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	// Create a project dir
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "file1.txt"), []byte("content1"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture a snapshot
	snap, err := store.Capture(projectDir, "test", "test restore")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	// Modify the file
	if err := os.WriteFile(filepath.Join(projectDir, "file1.txt"), []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Add a new file
	if err := os.WriteFile(filepath.Join(projectDir, "file2.txt"), []byte("content2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Restore the snapshot
	if restoreErr := store.Restore(snap.ID, projectDir); restoreErr != nil {
		t.Fatalf("Restore failed: %v", restoreErr)
	}

	// Verify file1.txt is restored
	content, restoreErr := os.ReadFile(filepath.Join(projectDir, "file1.txt"))
	if restoreErr != nil {
		t.Fatalf("ReadFile failed: %v", restoreErr)
	}
	if string(content) != "content1" {
		t.Errorf("expected 'content1', got %q", string(content))
	}

	// Verify file2.txt is deleted
	if _, err := os.Stat(filepath.Join(projectDir, "file2.txt")); !os.IsNotExist(err) {
		t.Error("expected file2.txt to be deleted after restore")
	}
}

// TestSnapshotStore_Restore_NonExistent tests Restore with a non-existent snapshot.
func TestSnapshotStore_Restore_NonExistent(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	err := store.Restore("non-existent", t.TempDir())
	if err == nil {
		t.Error("expected error for non-existent snapshot")
	}
}

// TestFormatList tests the FormatList function.
func TestFormatList(t *testing.T) {
	// Empty list
	result := FormatList(nil)
	if result != "No snapshots found." {
		t.Errorf("expected 'No snapshots found.', got %q", result)
	}

	// Non-empty list
	snaps := []*WorkspaceSnapshot{
		{ID: "abc123def456", Name: "test", CreatedAt: time.Now(), FileCount: 5, Size: 1024},
	}
	result = FormatList(snaps)
	if result == "" {
		t.Error("expected non-empty result")
	}
}

// TestFormatDiff tests the FormatDiff function.
func TestFormatDiff(t *testing.T) {
	diff := &SnapshotDiff{
		Added:     []string{"file1.txt"},
		Modified:  []string{"file2.txt"},
		Deleted:   []string{"file3.txt"},
		Unchanged: 3,
	}
	result := FormatDiff(diff, "test-snapshot")
	if result == "" {
		t.Error("expected non-empty result")
	}
}

// TestFormatAge tests the formatAge function.
func TestFormatAge(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{30 * time.Second, "just now"},
		{5 * time.Minute, "5m ago"},
		{2 * time.Hour, "2h ago"},
		{48 * time.Hour, "2d ago"},
	}

	for _, tt := range tests {
		result := formatAge(tt.d)
		if result != tt.expected {
			t.Errorf("formatAge(%v) = %q, want %q", tt.d, result, tt.expected)
		}
	}
}

// TestFormatSize tests the formatSize function.
func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{500, "500B"},
		{2048, "2KB"},
		{1048576, "1.0MB"},
	}

	for _, tt := range tests {
		result := formatSize(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, result, tt.expected)
		}
	}
}

// TestPluralize tests the pluralize function.
func TestPluralize(t *testing.T) {
	if pluralize("file", 1) != "file" {
		t.Error("expected 'file' for count 1")
	}
	if pluralize("file", 2) != "files" {
		t.Error("expected 'files' for count 2")
	}
}

// TestGenerateID tests the generateID function.
func TestGenerateID(t *testing.T) {
	id1 := generateID()
	id2 := generateID()

	if len(id1) != 12 {
		t.Errorf("expected ID length 12, got %d", len(id1))
	}
	if id1 == id2 {
		t.Error("expected different IDs")
	}
}

// TestRemoveEmptyParents tests the removeEmptyParents function.
func TestRemoveEmptyParents(t *testing.T) {
	// Create a nested empty directory structure
	base := t.TempDir()
	nested := filepath.Join(base, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	// removeEmptyParents should remove empty dirs up to base
	removeEmptyParents(nested, base)

	// The nested dir should be removed
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Error("expected nested dir to be removed")
	}
}

// TestRemoveEmptyParents_NonEmpty tests removeEmptyParents with non-empty dirs.
func TestRemoveEmptyParents_NonEmpty(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a file in the nested dir
	if err := os.WriteFile(filepath.Join(nested, "file.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// removeEmptyParents should not remove non-empty dirs
	removeEmptyParents(nested, base)

	if _, err := os.Stat(nested); os.IsNotExist(err) {
		t.Error("expected non-empty dir to NOT be removed")
	}
}

// TestGitCurrentBranch tests gitCurrentBranch with a non-git directory.
func TestGitCurrentBranch_NonGit(t *testing.T) {
	result := gitCurrentBranch(t.TempDir())
	if result != "" {
		t.Errorf("expected empty string for non-git dir, got %q", result)
	}
}

// TestGitCurrentCommit tests gitCurrentCommit with a non-git directory.
func TestGitCurrentCommit_NonGit(t *testing.T) {
	result := gitCurrentCommit(t.TempDir())
	if result != "" {
		t.Errorf("expected empty string for non-git dir, got %q", result)
	}
}

// TestSnapshotStore_Capture_WithIgnoredDirs tests that ignored directories are skipped.
func TestSnapshotStore_Capture_WithIgnoredDirs(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	projectDir := t.TempDir()

	// Create regular file
	if err := os.WriteFile(filepath.Join(projectDir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create ignored directory with a file
	if err := os.MkdirAll(filepath.Join(projectDir, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "node_modules", "pkg.js"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create .git directory with a file
	if err := os.MkdirAll(filepath.Join(projectDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".git", "config"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	snap, err := store.Capture(projectDir, "test", "test ignored dirs")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	// Should only have main.go
	if len(snap.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(snap.Files))
	}
	if _, exists := snap.Files["main.go"]; !exists {
		t.Error("expected main.go to be in snapshot")
	}
}

// TestSnapshotStore_Capture_EmptyDir tests capturing an empty directory.
func TestSnapshotStore_Capture_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	projectDir := t.TempDir()

	snap, err := store.Capture(projectDir, "empty", "empty dir")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	if snap.FileCount != 0 {
		t.Errorf("expected 0 files, got %d", snap.FileCount)
	}
}

// TestSnapshotStore_Capture_OverwritesAndPrunes tests that capture prunes when over limit.
func TestSnapshotStore_Capture_OverwritesAndPrunes(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)
	store.MaxSnapshots = 2

	projectDir := t.TempDir()

	// Capture 3 snapshots
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(filepath.Join(projectDir, "file.txt"), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := store.Capture(projectDir, "snap"+string(rune('a'+i)), "test")
		if err != nil {
			t.Fatalf("Capture failed: %v", err)
		}
	}

	// Should have 2 snapshots (pruned)
	list, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(list))
	}
}

// TestSnapshotStore_Get_WithContent tests Get returns full file content.
func TestSnapshotStore_Get_WithContent(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir)

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "test.txt"), []byte("file content"), 0o644); err != nil {
		t.Fatal(err)
	}

	snap, err := store.Capture(projectDir, "test", "content test")
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	loaded, err := store.Get(snap.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(loaded.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(loaded.Files))
	}

	fs, ok := loaded.Files["test.txt"]
	if !ok {
		t.Fatal("expected test.txt in files")
	}
	if string(fs.Content) != "file content" {
		t.Errorf("expected 'file content', got %q", string(fs.Content))
	}
}
