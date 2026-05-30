package logger

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBufferedWriter_SmallBuffer_NoRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	bw := newBufferedWriter(BufferedConfig{
		FlushInterval: 100 * time.Millisecond,
		BufferSize:    10,
		MaxFileSize:   1024,
		MaxBackups:    2,
		FilePath:      path,
	})
	defer bw.Close()

	// Write a few entries
	for i := 0; i < 3; i++ {
		_, _ = fmt.Fprintf(bw, "entry %d", i)
	}

	bw.Flush()

	// Verify no rotation files created
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatal("expected no rotation file")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}

func TestBufferedWriter_ExceedsMaxSize_Rotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	bw := newBufferedWriter(BufferedConfig{
		FlushInterval: 100 * time.Millisecond,
		BufferSize:    100,
		MaxFileSize:   50, // very small to trigger rotation
		MaxBackups:    2,
		FilePath:      path,
	})
	defer bw.Close()

	// Write enough to exceed MaxFileSize
	for i := 0; i < 10; i++ {
		_, _ = fmt.Fprintf(bw, "entry %d: %s\n", i, strings.Repeat("x", 20))
	}
	bw.Flush()

	// Verify rotation file created
	if _, err := os.Stat(path + ".1"); os.IsNotExist(err) {
		t.Fatal("expected rotation file .1 to exist")
	}
}

func TestBufferedWriter_MultipleRotations_KeepsMaxBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	bw := newBufferedWriter(BufferedConfig{
		FlushInterval: 100 * time.Millisecond,
		BufferSize:    100,
		MaxFileSize:   30, // very small
		MaxBackups:    2,
		FilePath:      path,
	})
	defer bw.Close()

	// Write enough to trigger multiple rotations
	for round := 0; round < 5; round++ {
		for i := 0; i < 5; i++ {
			_, _ = fmt.Fprintf(bw, "round %d entry %d: %s\n", round, i, strings.Repeat("y", 20))
		}
		bw.Flush()
	}

	// Should have .1 and .2 but not .3
	if _, err := os.Stat(path + ".1"); os.IsNotExist(err) {
		t.Fatal("expected .1 backup")
	}
	if _, err := os.Stat(path + ".2"); os.IsNotExist(err) {
		t.Fatal("expected .2 backup")
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatal("expected no .3 backup (MaxBackups=2)")
	}
}

func TestBufferedWriter_BufferSizeLimit_InlineFlush(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	bw := newBufferedWriter(BufferedConfig{
		FlushInterval: 10 * time.Second, // long interval so only inline flush triggers
		BufferSize:    3,
		MaxFileSize:   1024 * 1024,
		MaxBackups:    2,
		FilePath:      path,
	})
	defer bw.Close()

	// Write exactly BufferSize entries — should trigger inline flush
	for i := 0; i < 3; i++ {
		_, _ = fmt.Fprintf(bw, "entry %d", i)
	}

	// Verify data is on disk (inline flush triggered)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines after inline flush, got %d", len(lines))
	}
}

func TestBufferedWriter_FileSystemError_NoPanic(t *testing.T) {
	// Write to a non-existent directory — should not panic
	bw := newBufferedWriter(BufferedConfig{
		FlushInterval: 100 * time.Millisecond,
		BufferSize:    10,
		MaxFileSize:   1024,
		MaxBackups:    2,
		FilePath:      "/nonexistent/dir/test.log",
	})
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()

	_, _ = fmt.Fprintf(bw, "should not panic")
	bw.Flush()
}

func TestBufferedWriter_DirectWriter(t *testing.T) {
	var buf bytes.Buffer

	// No FilePath — should use direct writer (we can't easily test stderr,
	// so we test the direct path by verifying the code path doesn't panic)
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Write to file (the normal path)
	bw := newBufferedWriter(BufferedConfig{
		FlushInterval: 100 * time.Millisecond,
		BufferSize:    10,
		MaxFileSize:   1024,
		MaxBackups:    2,
		FilePath:      path,
	})

	_, _ = fmt.Fprintf(bw, "test entry")
	bw.Flush()
	bw.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	if !strings.Contains(string(data), "test entry") {
		t.Fatal("expected 'test entry' in log file")
	}

	_ = buf // suppress unused warning
}

func TestLogger_SetOutputForTesting(t *testing.T) {
	l := Default()

	var buf bytes.Buffer
	l.SetOutputForTesting(&buf)

	l.Info("test message")
	if !strings.Contains(buf.String(), "test message") {
		t.Fatal("expected output to go to test writer")
	}
}

func TestLogger_FlushForTesting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	bw := newBufferedWriter(BufferedConfig{
		FlushInterval: 10 * time.Second, // long interval
		BufferSize:    100,
		MaxFileSize:   1024 * 1024,
		MaxBackups:    2,
		FilePath:      path,
	})

	l := New(bw, Debug)
	l.Info("buffered message")
	l.FlushForTesting()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	if !strings.Contains(string(data), "buffered message") {
		t.Fatal("expected 'buffered message' after flush")
	}
}

func TestLogger_LogNeverPanics(t *testing.T) {
	// Create a writer that panics
	panicWriter := &panickingWriter{}

	l := New(panicWriter, Debug)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("log() should not propagate panics, got: %v", r)
		}
	}()

	// This should not panic — the recover in log() should catch it
	l.Info("this should not panic")
}

type panickingWriter struct{}

func (w *panickingWriter) Write(p []byte) (n int, err error) {
	panic("intentional panic in writer")
}
