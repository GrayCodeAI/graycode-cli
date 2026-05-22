package retention

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()
	if p.MaxAge != 30*24*time.Hour {
		t.Errorf("MaxAge = %v, want 30d", p.MaxAge)
	}
	if p.MaxSizeMB != 500 {
		t.Errorf("MaxSizeMB = %d, want 500", p.MaxSizeMB)
	}
}

func TestCleanDirectoryRemovesOldFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a file with an old mtime.
	oldFile := filepath.Join(dir, "old.txt")
	if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(oldFile, oldTime, oldTime)

	// Create a recent file.
	recentFile := filepath.Join(dir, "recent.txt")
	if err := os.WriteFile(recentFile, []byte("recent"), 0o644); err != nil {
		t.Fatal(err)
	}

	policy := Policy{MaxAge: 24 * time.Hour}
	result := CleanDirectory(dir, policy)

	if result.FilesRemoved != 1 {
		t.Errorf("FilesRemoved = %d, want 1", result.FilesRemoved)
	}
	if result.BytesFreed != 3 {
		t.Errorf("BytesFreed = %d, want 3", result.BytesFreed)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v, want none", result.Errors)
	}

	// Recent file should still exist.
	if _, err := os.Stat(recentFile); err != nil {
		t.Error("recent file should still exist")
	}
}

func TestCleanDirectorySkipsDirs(t *testing.T) {
	dir := t.TempDir()
	oldDir := filepath.Join(dir, "olddir")
	os.Mkdir(oldDir, 0o755)
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(oldDir, oldTime, oldTime)

	result := CleanDirectory(dir, Policy{MaxAge: time.Hour})
	if result.FilesRemoved != 0 {
		t.Errorf("should not remove directories, got %d removed", result.FilesRemoved)
	}
}

func TestCleanDirectoryNonexistentDir(t *testing.T) {
	result := CleanDirectory("/nonexistent/path", Policy{MaxAge: time.Hour})
	if len(result.Errors) == 0 {
		t.Error("expected error for nonexistent directory")
	}
}

func TestEnforceSizeNoop(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644)

	result := EnforceSize(dir, Policy{MaxSizeMB: 100})
	if result.FilesRemoved != 0 {
		t.Errorf("FilesRemoved = %d, want 0 (under limit)", result.FilesRemoved)
	}
}

func TestEnforceSizeRemovesOldest(t *testing.T) {
	oldTime := time.Now().Add(-time.Hour)

	// Case 1: Both files fit under limit — nothing removed.
	dir1 := t.TempDir()
	os.WriteFile(filepath.Join(dir1, "a.dat"), make([]byte, 400_000), 0o644)
	os.WriteFile(filepath.Join(dir1, "b.dat"), make([]byte, 400_000), 0o644)

	result := EnforceSize(dir1, Policy{MaxSizeMB: 1})
	if result.FilesRemoved != 0 {
		t.Errorf("case 1: FilesRemoved = %d, want 0", result.FilesRemoved)
	}

	// Case 2: Three files totaling 1.6MB, limit 1MB. Oldest removed first.
	dir2 := t.TempDir()
	oldFile := filepath.Join(dir2, "old.dat")
	os.WriteFile(oldFile, make([]byte, 400_000), 0o644)
	os.Chtimes(oldFile, oldTime, oldTime)

	midFile := filepath.Join(dir2, "mid.dat")
	os.WriteFile(midFile, make([]byte, 400_000), 0o644)

	newFile := filepath.Join(dir2, "new.dat")
	os.WriteFile(newFile, make([]byte, 800_000), 0o644)

	result = EnforceSize(dir2, Policy{MaxSizeMB: 1})
	// After removing old (400KB): 1.2MB left, still over.
	// After removing mid (400KB): 800KB left, under. So 2 removed.
	if result.FilesRemoved != 2 {
		t.Errorf("case 2: FilesRemoved = %d, want 2", result.FilesRemoved)
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Error("case 2: newest (biggest) file should still exist")
	}
}

func TestEnforceSizeNonexistentDir(t *testing.T) {
	result := EnforceSize("/nonexistent/path", Policy{MaxSizeMB: 1})
	if len(result.Errors) == 0 {
		t.Error("expected error for nonexistent directory")
	}
}
