package tool

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupFile_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nofile.txt")
	bp, err := BackupFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp != "" {
		t.Fatalf("expected empty backup path, got %s", bp)
	}
}

func TestBackupFile_Success(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	os.WriteFile(path, []byte("hello"), 0o644)

	bp, err := BackupFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bp == "" {
		t.Fatal("expected non-empty backup path")
	}
	if _, err := os.Stat(bp); err != nil {
		t.Fatalf("backup file does not exist: %v", err)
	}
}

func TestRestoreFromBackup(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	os.WriteFile(path, []byte("original"), 0o644)

	_, err := BackupFile(path)
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	// Overwrite the file
	os.WriteFile(path, []byte("modified"), 0o644)

	if err := RestoreFromBackup(path); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "original" {
		t.Fatalf("expected 'original', got %q", string(data))
	}
}

func TestListBackups(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	os.WriteFile(path, []byte("v1"), 0o644)

	BackupFile(path)
	time.Sleep(1100 * time.Millisecond) // ensure distinct second-level timestamp
	os.WriteFile(path, []byte("v2"), 0o644)
	BackupFile(path)

	backups := ListBackups(path)
	if len(backups) < 2 {
		t.Fatalf("expected at least 2 backups, got %d", len(backups))
	}
}
