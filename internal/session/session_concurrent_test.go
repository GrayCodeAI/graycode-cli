package session

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestWAL_ConcurrentAppend(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	graycodeDir := setTestSessionsDir(t, dir)
	if err := os.MkdirAll(graycodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	wal, err := NewWAL("concurrent-test")
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			msg := Message{
				Role:    "user",
				Content: fmt.Sprintf("message %d", n),
			}
			if err := wal.Append(msg); err != nil {
				t.Errorf("Append(%d) error: %v", n, err)
			}
		}(i)
	}
	wg.Wait()
}

func TestSession_ConcurrentSaveLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	graycodeDir := setTestSessionsDir(t, dir)
	if err := os.MkdirAll(graycodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sess := &Session{
		ID:        "concurrent-sess",
		Model:     "test-model",
		Provider:  "test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
	}

	if err := Save(sess); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			loaded, err := Load("concurrent-sess")
			if err != nil {
				t.Errorf("Load error: %v", err)
				return
			}
			if loaded.ID != "concurrent-sess" {
				t.Errorf("ID = %q, want concurrent-sess", loaded.ID)
			}
		}()
	}
	wg.Wait()
}

func TestSession_ConcurrentList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	graycodeDir := setTestSessionsDir(t, dir)
	if err := os.MkdirAll(graycodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		sess := &Session{
			ID:        fmt.Sprintf("list-test-%d", i),
			Model:     "test",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Messages:  []Message{{Role: "user", Content: "hello"}},
		}
		if err := Save(sess); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entries, err := List()
			if err != nil {
				t.Errorf("List error: %v", err)
				return
			}
			if len(entries) < 10 {
				t.Errorf("List returned %d entries, want >= 10", len(entries))
			}
		}()
	}
	wg.Wait()
}
