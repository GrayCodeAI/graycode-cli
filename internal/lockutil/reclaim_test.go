package lockutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReclaimStaleLockRemovesStale(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "demo.lock")
	if err := os.WriteFile(lock, []byte("holder"), 0o600); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := ReclaimStaleLock(lock, "abc", func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if !reclaimed {
		t.Fatal("expected stale lock to be reclaimed")
	}
	if _, err := os.Stat(lock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock should be gone after reclaim, stat err = %v", err)
	}
}

func TestReclaimStaleLockRestoresLive(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "demo.lock")
	if err := os.WriteFile(lock, []byte("holder"), 0o600); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := ReclaimStaleLock(lock, "abc", func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed {
		t.Fatal("a live lock must not be reported as reclaimed")
	}
	data, err := os.ReadFile(lock)
	if err != nil {
		t.Fatalf("live lock not restored: %v", err)
	}
	if string(data) != "holder" {
		t.Fatalf("restored lock content = %q, want holder", data)
	}
}

func TestReclaimStaleLockMissingIsLostRace(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "nope.lock")
	reclaimed, err := ReclaimStaleLock(lock, "abc", func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed {
		t.Fatal("missing lock must not be reported as reclaimed")
	}
}

func TestReclaimStaleLockRestoreFailureFailsClosed(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "demo.lock")
	if err := os.WriteFile(lock, []byte("holder"), 0o600); err != nil {
		t.Fatal(err)
	}
	orig := restoreLockFile
	restoreLockFile = func(string, string) error { return errors.New("restore boom") }
	defer func() { restoreLockFile = orig }()

	if _, err := ReclaimStaleLock(lock, "abc", func(string) bool { return true }); err == nil {
		t.Fatal("expected error when restore fails")
	}
}

func TestRestoreLockFileNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	reclaimed := filepath.Join(dir, "reclaimed")
	path := filepath.Join(dir, "target")
	if err := os.WriteFile(reclaimed, []byte("r"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("new-holder"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := RestoreLockFile(reclaimed, path)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist, got %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "new-holder" {
		t.Fatalf("competing holder overwritten: %q", data)
	}
}

func TestRemoveLockFileMissingIsNoop(t *testing.T) {
	if err := RemoveLockFile(filepath.Join(t.TempDir(), "missing.lock")); err != nil {
		t.Fatalf("expected no-op, got %v", err)
	}
}
