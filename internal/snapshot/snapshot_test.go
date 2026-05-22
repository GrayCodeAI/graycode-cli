package snapshot

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "initial")
	return dir
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %s: %v", name, args, out, err)
	}
}

func TestTracker_Init(t *testing.T) {
	dir := setupTestProject(t)
	tracker := New(dir)

	if err := tracker.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if err := tracker.Init(); err != nil {
		t.Fatalf("Second Init failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".hawk", "snapshots", ".git")); err != nil {
		t.Error("shadow git repo not initialized")
	}
}

func TestTracker_TrackAndHistory(t *testing.T) {
	dir := setupTestProject(t)
	tracker := New(dir)
	tracker.Init()

	hash1, err := tracker.Track("first snapshot")
	if err != nil {
		t.Fatalf("Track 1 failed: %v", err)
	}
	if hash1 == "" {
		t.Error("expected non-empty hash")
	}

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)

	hash2, err := tracker.Track("added main func")
	if err != nil {
		t.Fatalf("Track 2 failed: %v", err)
	}
	if hash2 == hash1 {
		t.Error("second snapshot should have different hash")
	}

	history, err := tracker.History(10)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}
	if len(history) < 2 {
		t.Errorf("expected at least 2 snapshots, got %d", len(history))
	}
}

func TestTracker_Restore(t *testing.T) {
	dir := setupTestProject(t)
	tracker := New(dir)
	tracker.Init()

	hash1, _ := tracker.Track("original")

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("MODIFIED\n"), 0o644)
	tracker.Track("modified")

	data, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if string(data) != "MODIFIED\n" {
		t.Fatal("file should be modified")
	}

	if err := tracker.Restore(hash1); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	data, _ = os.ReadFile(filepath.Join(dir, "main.go"))
	if string(data) != "package main\n" {
		t.Errorf("expected original content after restore, got: %q", string(data))
	}
}

func TestTracker_Diff(t *testing.T) {
	dir := setupTestProject(t)
	tracker := New(dir)
	tracker.Init()

	hash1, _ := tracker.Track("before")
	os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main\n"), 0o644)
	hash2, _ := tracker.Track("after")

	diffs, err := tracker.Diff(hash1, hash2)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if len(diffs) == 0 {
		t.Error("expected at least one diff")
	}
}
