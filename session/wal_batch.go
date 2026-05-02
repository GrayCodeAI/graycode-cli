package session

import (
	"encoding/json"
	"sync"
	"time"
)

// BatchedWAL wraps a WAL and batches Append calls, flushing to disk on a
// timer (100ms) or when the buffer reaches 10 entries. This reduces the
// number of f.Sync() calls from one-per-append to one-per-flush.
type BatchedWAL struct {
	wal   *WAL
	mu    sync.Mutex
	buf   []Message
	timer *time.Timer
}

const (
	batchFlushInterval = 100 * time.Millisecond
	batchMaxSize       = 10
)

// NewBatchedWAL wraps an existing WAL with batching.
func NewBatchedWAL(wal *WAL) *BatchedWAL {
	return &BatchedWAL{wal: wal}
}

// Append buffers a message and flushes if the buffer is full.
func (b *BatchedWAL) Append(msg Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buf = append(b.buf, msg)
	if len(b.buf) >= batchMaxSize {
		return b.flushLocked()
	}
	b.ensureTimerLocked()
	return nil
}

// Flush writes all buffered messages to the underlying WAL.
func (b *BatchedWAL) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flushLocked()
}

func (b *BatchedWAL) flushLocked() error {
	if len(b.buf) == 0 {
		return nil
	}
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}

	b.wal.mu.Lock()
	defer b.wal.mu.Unlock()

	for _, msg := range b.buf {
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		data = append(data, '\n')
		if _, err := b.wal.f.Write(data); err != nil {
			return err
		}
	}
	err := b.wal.f.Sync()
	b.buf = b.buf[:0]
	return err
}

func (b *BatchedWAL) ensureTimerLocked() {
	if b.timer != nil {
		return
	}
	b.timer = time.AfterFunc(batchFlushInterval, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.flushLocked()
	})
}

// Close flushes remaining entries and closes the underlying WAL.
func (b *BatchedWAL) Close() error {
	_ = b.Flush()
	return b.wal.Close()
}
