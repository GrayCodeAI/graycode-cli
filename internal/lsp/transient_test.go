package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkspaceQueue_FIFO(t *testing.T) {
	wq := NewWorkspaceQueue()
	dir := t.TempDir()

	ctx := context.Background()
	var order []int
	var mu sync.Mutex

	unlock1, err := wq.Lock(ctx, dir)
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		unlock2, err := wq.Lock(ctx, dir)
		if err != nil {
			t.Errorf("second lock failed: %v", err)
			return
		}
		defer unlock2()

		mu.Lock()
		order = append(order, 2)
		mu.Unlock()
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	order = append(order, 1)
	mu.Unlock()
	unlock1()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second routine")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("expected FIFO execution [1, 2], got %v", order)
	}
}

func TestWorkspaceQueue_ParallelWorkspaces(t *testing.T) {
	wq := NewWorkspaceQueue()
	dirA := filepath.Join(t.TempDir(), "workspaceA")
	dirB := filepath.Join(t.TempDir(), "workspaceB")

	ctx := context.Background()
	unlockA, err := wq.Lock(ctx, dirA)
	if err != nil {
		t.Fatalf("lock A failed: %v", err)
	}
	defer unlockA()

	// Locking dirB should succeed immediately even while dirA is held
	ctxTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	unlockB, err := wq.Lock(ctxTimeout, dirB)
	if err != nil {
		t.Fatalf("lock B should succeed concurrently, got %v", err)
	}
	unlockB()
}

func TestWorkspaceQueue_ContextCanceled(t *testing.T) {
	wq := NewWorkspaceQueue()
	dir := t.TempDir()

	unlock, err := wq.Lock(context.Background(), dir)
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}
	defer unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled before lock

	_, err = wq.Lock(ctx, dir)
	if err == nil {
		t.Fatal("expected error on canceled context lock")
	}
}

func TestReadBoundedSource(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.go")
	content := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	read, err := ReadBoundedSource(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if read != content {
		t.Errorf("read %q, want %q", read, content)
	}

	// Non-existent file
	_, err = ReadBoundedSource(filepath.Join(tmpDir, "nonexistent.go"))
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestFileURI(t *testing.T) {
	uri := FileURI("sample.go")
	if !strings.HasPrefix(uri, "file://") {
		t.Errorf("expected file:// prefix, got %q", uri)
	}
	if strings.Contains(uri, "\\") {
		t.Errorf("expected forward slashes in URI, got %q", uri)
	}
}

func TestExecuteTransient_ClosedManager(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go": {Command: "gopls", Extensions: []string{".go"}},
	}}
	m := NewManager(cfg)
	_ = m.Close()

	err := m.ExecuteTransient(context.Background(), t.TempDir(), "file.go", "go", true, func(c *LSPClient, uri string) error {
		return nil
	})
	if err != ErrManagerClosed {
		t.Errorf("expected ErrManagerClosed, got %v", err)
	}
}

func TestExecuteTransient_UnconfiguredLanguage(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{}}
	m := NewManager(cfg)
	defer m.Close()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file.xyz")
	_ = os.WriteFile(filePath, []byte("content"), 0o644)

	err := m.ExecuteTransient(context.Background(), tmpDir, filePath, "unknown-lang", true, func(c *LSPClient, uri string) error {
		return nil
	})
	if err == nil {
		t.Error("expected error for unconfigured language")
	}
}

func TestExecuteTransient_SourceFileNotFound(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go": {Command: "gopls", Extensions: []string{".go"}},
	}}
	m := NewManager(cfg)
	defer m.Close()

	tmpDir := t.TempDir()
	err := m.ExecuteTransient(context.Background(), tmpDir, filepath.Join(tmpDir, "missing.go"), "go", true, func(c *LSPClient, uri string) error {
		return nil
	})
	if err == nil {
		t.Error("expected error when reading missing source file")
	}
}

func TestExecuteTransient_GlobalQueue(t *testing.T) {
	q := GetWorkspaceQueue()
	if q == nil {
		t.Fatal("expected non-nil global workspace queue")
	}
}

func TestWorkspaceQueue_AtomicIsolation(t *testing.T) {
	wq := NewWorkspaceQueue()
	dir := t.TempDir()

	var counter atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := wq.Lock(context.Background(), dir)
			if err != nil {
				t.Errorf("lock error: %v", err)
				return
			}
			defer unlock()

			val := counter.Add(1)
			time.Sleep(5 * time.Millisecond)
			if counter.Load() != val {
				t.Errorf("race detected in workspace queue serialization")
			}
		}()
	}

	wg.Wait()
	if counter.Load() != 10 {
		t.Errorf("counter = %d, want 10", counter.Load())
	}
}
