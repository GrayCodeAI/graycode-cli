package cmd

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestFileWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var changes []string

	fw := NewFileWatcher(dir, []string{".git"}, func(path, diff string) {
		mu.Lock()
		changes = append(changes, path)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- fw.Start(ctx)
	}()

	// Give the watcher time to start.
	time.Sleep(200 * time.Millisecond)

	// Write a file.
	file := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + processing.
	time.Sleep(600 * time.Millisecond)

	mu.Lock()
	got := len(changes)
	mu.Unlock()

	if got == 0 {
		t.Error("expected at least one change callback, got 0")
	}

	fw.Stop()
	cancel()
}

func TestFileWatcherIgnoresPatterns(t *testing.T) {
	dir := t.TempDir()

	// Create a .git subdir to ignore.
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(gitDir, 0o755)

	var mu sync.Mutex
	var changes []string

	fw := NewFileWatcher(dir, []string{".git"}, func(path, diff string) {
		mu.Lock()
		changes = append(changes, path)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go fw.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	// Write inside .git — should be ignored.
	os.WriteFile(filepath.Join(gitDir, "index"), []byte("x"), 0o644)
	time.Sleep(600 * time.Millisecond)

	mu.Lock()
	got := len(changes)
	mu.Unlock()

	if got != 0 {
		t.Errorf("expected 0 changes for ignored .git files, got %d", got)
	}

	fw.Stop()
	cancel()
}

func TestFileWatcherStopIdempotent(t *testing.T) {
	fw := NewFileWatcher(t.TempDir(), nil, nil)
	// Calling Stop multiple times should not panic.
	fw.Stop()
	fw.Stop()
}

func TestGitDiffForFileNonRepo(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.txt")
	os.WriteFile(file, []byte("a"), 0o644)
	// Should return empty string without crashing.
	diff := gitDiffForFile(dir, file)
	if diff != "" {
		t.Errorf("expected empty diff outside git repo, got %q", diff)
	}
}
