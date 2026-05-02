package session

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
)

func newTestWAL(t *testing.T) (*WAL, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wal")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	return &WAL{f: f, path: path, id: "test"}, path
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		n++
	}
	return n
}

func TestBatchedWAL_Append(t *testing.T) {
	wal, path := newTestWAL(t)
	bw := NewBatchedWAL(wal)

	for i := 0; i < 3; i++ {
		if err := bw.Append(Message{Role: "user", Content: "msg"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	if n := countLines(t, path); n != 3 {
		t.Fatalf("expected 3 lines, got %d", n)
	}
}

func TestBatchedWAL_AutoFlush(t *testing.T) {
	wal, path := newTestWAL(t)
	bw := NewBatchedWAL(wal)

	for i := 0; i < 12; i++ {
		if err := bw.Append(Message{Role: "user", Content: "msg"}); err != nil {
			t.Fatal(err)
		}
	}
	// First 10 should have auto-flushed; remaining 2 still buffered.
	if n := countLines(t, path); n != 10 {
		t.Fatalf("expected 10 auto-flushed lines, got %d", n)
	}
	bw.Flush()
	if n := countLines(t, path); n != 12 {
		t.Fatalf("expected 12 total lines, got %d", n)
	}
}

func TestBatchedWAL_Close(t *testing.T) {
	wal, path := newTestWAL(t)
	bw := NewBatchedWAL(wal)

	for i := 0; i < 5; i++ {
		if err := bw.Append(Message{Role: "assistant", Content: "hi"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := bw.Close(); err != nil {
		t.Fatal(err)
	}
	if n := countLines(t, path); n != 5 {
		t.Fatalf("expected 5 lines after close, got %d", n)
	}
}
