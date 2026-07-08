package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BufferedConfig configures the buffered writer.
type BufferedConfig struct {
	FlushInterval time.Duration // default 500ms
	BufferSize    int           // default 50 entries
	MaxFileSize   int64         // default 50 MB; 0 = no rotation
	MaxBackups    int           // default 2
	FilePath      string        // if empty, uses direct writer
}

func (c BufferedConfig) withDefaults() BufferedConfig {
	if c.FlushInterval == 0 {
		c.FlushInterval = 500 * time.Millisecond
	}
	if c.BufferSize == 0 {
		c.BufferSize = 50
	}
	if c.MaxFileSize == 0 {
		c.MaxFileSize = 50 * 1024 * 1024 // 50 MB
	}
	if c.MaxBackups == 0 {
		c.MaxBackups = 2
	}
	return c
}

// bufferedWriter wraps an io.Writer with in-memory buffering,
// periodic flushing, and optional log rotation.
type bufferedWriter struct {
	mu          sync.Mutex
	buffer      []string
	config      BufferedConfig
	fileWriter  io.WriteCloser // nil if using direct writer
	direct      io.Writer      // non-nil when FilePath is empty
	currentSize int64
	flushTimer  *time.Timer
	closed      bool
}

// newBufferedWriter creates a buffered writer with the given config.
// If config.FilePath is set, it writes to that file with rotation.
// Otherwise it writes to os.Stderr directly.
func newBufferedWriter(cfg BufferedConfig) *bufferedWriter {
	cfg = cfg.withDefaults()

	bw := &bufferedWriter{
		buffer: make([]string, 0, cfg.BufferSize),
		config: cfg,
	}

	if cfg.FilePath == "" {
		bw.direct = os.Stderr
	} else {
		_ = os.MkdirAll(filepath.Dir(cfg.FilePath), 0o750)
		f, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			// Fall back to stderr if file can't be opened
			bw.direct = os.Stderr
		} else {
			bw.fileWriter = f
			info, err := f.Stat()
			if err == nil {
				bw.currentSize = info.Size()
			}
		}
	}

	// Start periodic flush
	bw.flushTimer = time.AfterFunc(cfg.FlushInterval, bw.periodicFlush)

	return bw
}

// Write implements io.Writer. Appends to the buffer and triggers
// an inline flush if the buffer is full.
func (bw *bufferedWriter) Write(p []byte) (n int, err error) {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	if bw.closed {
		return 0, fmt.Errorf("bufferedWriter is closed")
	}

	bw.buffer = append(bw.buffer, string(p))

	if len(bw.buffer) >= bw.config.BufferSize {
		bw.flushLocked()
	}

	return len(p), nil
}

// periodicFlush is called by the timer to flush buffered entries.
func (bw *bufferedWriter) periodicFlush() {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	if bw.closed {
		return
	}

	bw.flushLocked()

	// Reset the timer for the next interval
	bw.flushTimer.Reset(bw.config.FlushInterval)
}

// flushLocked flushes the buffer. Caller must hold bw.mu.
func (bw *bufferedWriter) flushLocked() {
	if len(bw.buffer) == 0 {
		return
	}

	writer := bw.resolveWriter()
	if writer == nil {
		bw.buffer = bw.buffer[:0]
		return
	}

	for _, entry := range bw.buffer {
		n, err := fmt.Fprintln(writer, entry)
		if err != nil {
			// Can't write — drop the entry and continue
			continue
		}
		bw.currentSize += int64(n)
	}

	bw.buffer = bw.buffer[:0]

	// Check rotation
	if bw.fileWriter != nil && bw.config.MaxFileSize > 0 && bw.currentSize >= bw.config.MaxFileSize {
		bw.rotate()
	}
}

// resolveWriter returns the underlying writer. Caller must hold bw.mu.
func (bw *bufferedWriter) resolveWriter() io.Writer {
	if bw.fileWriter != nil {
		return bw.fileWriter
	}
	return bw.direct
}

// rotate performs log rotation. Caller must hold bw.mu.
func (bw *bufferedWriter) rotate() {
	if bw.fileWriter == nil {
		return
	}

	// Close current file
	_ = bw.fileWriter.Close()

	// Shift backup files: .2 -> delete, .1 -> .2, current -> .1
	for i := bw.config.MaxBackups; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", bw.config.FilePath, i)
		if i == bw.config.MaxBackups {
			_ = os.Remove(src)
		} else {
			dst := fmt.Sprintf("%s.%d", bw.config.FilePath, i+1)
			_ = os.Rename(src, dst)
		}
	}

	// Current -> .1
	backup := bw.config.FilePath + ".1"
	_ = os.Rename(bw.config.FilePath, backup)

	// Open new file
	f, err := os.OpenFile(bw.config.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		bw.fileWriter = nil
		bw.direct = os.Stderr
		bw.currentSize = 0
		return
	}

	bw.fileWriter = f
	bw.currentSize = 0
}

// Close flushes remaining entries and closes the underlying file.
func (bw *bufferedWriter) Close() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	bw.closed = true
	bw.flushLocked()

	if bw.flushTimer != nil {
		bw.flushTimer.Stop()
	}

	if bw.fileWriter != nil {
		return bw.fileWriter.Close()
	}
	return nil
}

// Flush synchronously flushes buffered entries. Safe to call from tests.
func (bw *bufferedWriter) Flush() {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	bw.flushLocked()
}
