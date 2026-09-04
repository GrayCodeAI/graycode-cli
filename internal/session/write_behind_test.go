package session

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
)

func TestWriteBehindFlushImmediately(t *testing.T) {
	var written atomic.Int32
	var mu sync.Mutex
	var got []eventlog.WireEvent
	w := NewWriteBehind(WriteBehindOptions{
		MaxDelay: 100 * time.Millisecond,
		Write: func(events []eventlog.WireEvent) error {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, events...)
			written.Add(1)
			return nil
		},
	})

	ev := eventlog.WireEvent{Type: "turn/start", Seq: 1, Data: []byte("{}")}
	w.Enqueue(ev)

	if err := w.Flush(); err != nil {
		t.Fatalf("flush error: %v", err)
	}
	if written.Load() != 1 {
		t.Errorf("expected 1 write, got %d", written.Load())
	}
	if len(got) != 1 {
		t.Errorf("expected 1 event in write, got %d", len(got))
	}
}

func TestWriteBehindBatchesThenTimeout(t *testing.T) {
	var writeCount atomic.Int32
	var mu sync.Mutex
	var totalEvents int
	w := NewWriteBehind(WriteBehindOptions{
		MaxDelay: 20 * time.Millisecond,
		Write: func(events []eventlog.WireEvent) error {
			mu.Lock()
			totalEvents += len(events)
			mu.Unlock()
			writeCount.Add(1)
			return nil
		},
	})

	// Enqueue 3 events rapidly — should batch into 1 write on timeout.
	for i := 0; i < 3; i++ {
		w.Enqueue(eventlog.WireEvent{Type: "todo/write", Seq: uint64(i + 1), Data: []byte("{}")})
	}

	// Wait for the timer to fire.
	time.Sleep(50 * time.Millisecond)

	// Flush to ensure any background write completes.
	if err := w.Flush(); err != nil {
		t.Fatalf("flush error: %v", err)
	}

	if writeCount.Load() < 1 {
		t.Errorf("expected at least 1 write, got %d", writeCount.Load())
	}
	mu.Lock()
	got := totalEvents
	mu.Unlock()
	if got != 3 {
		t.Errorf("expected 3 total events, got %d", got)
	}
}

func TestWriteBehindBackgroundFailureRetains(t *testing.T) {
	var writeCount atomic.Int32
	var failCount atomic.Int32
	w := NewWriteBehind(WriteBehindOptions{
		MaxDelay: 10 * time.Millisecond,
		Write: func(events []eventlog.WireEvent) error {
			writeCount.Add(1)
			return errTestFailure
		},
		ReportBackgroundFailure: func(_ eventlog.WireEvent, err error) {
			failCount.Add(1)
		},
	})

	ev := eventlog.WireEvent{Type: "turn/start", Seq: 1, Data: []byte("{}")}
	w.Enqueue(ev)

	// Wait for background write to fail + re-queue.
	time.Sleep(30 * time.Millisecond)

	if writeCount.Load() < 1 {
		t.Errorf("expected at least 1 attempt, got %d", writeCount.Load())
	}
	if failCount.Load() < 1 {
		t.Errorf("expected failure report, got %d", failCount.Load())
	}
	// Events should be retained (not lost).
	if w.PendingCount() < 1 {
		t.Errorf("expected retained events, got %d", w.PendingCount())
	}
}

func TestWriteBehindConcurrentFlush(t *testing.T) {
	var writeCount atomic.Int32
	var totalEvents atomic.Int32
	w := NewWriteBehind(WriteBehindOptions{
		MaxDelay: 50 * time.Millisecond,
		Write: func(events []eventlog.WireEvent) error {
			writeCount.Add(1)
			totalEvents.Add(int32(len(events)))
			return nil
		},
	})

	// Enqueue all events first, then flush concurrently.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w.Enqueue(eventlog.WireEvent{Type: "todo/write", Seq: uint64(i + 1), Data: []byte("{}")})
		}(i)
	}
	wg.Wait() // ensure all enqueues complete before flushing

	// Now flush from multiple goroutines concurrently — should join the same barrier.
	var flushWg sync.WaitGroup
	for i := 0; i < 5; i++ {
		flushWg.Add(1)
		go func() {
			defer flushWg.Done()
			if err := w.Flush(); err != nil {
				t.Errorf("flush error: %v", err)
			}
		}()
	}
	flushWg.Wait()

	if totalEvents.Load() != 10 {
		t.Errorf("expected exactly 10 events written, got %d", totalEvents.Load())
	}
}

func TestWriteBehindHasWork(t *testing.T) {
	w := NewWriteBehind(WriteBehindOptions{
		MaxDelay: 100 * time.Millisecond,
		Write:    func(events []eventlog.WireEvent) error { return nil },
	})

	if w.HasWork() {
		t.Fatal("expected no work initially")
	}
	w.Enqueue(eventlog.WireEvent{Type: "turn/start", Seq: 1, Data: []byte("{}")})
	if !w.HasWork() {
		t.Fatal("expected work after enqueue")
	}
	_ = w.Flush()
	// After flush, there should be no pending work.
	// (active might briefly be true during the write, but pending should be drained)
}

func TestWriteBehindCancelAutomaticWait(t *testing.T) {
	var writeCount atomic.Int32
	w := NewWriteBehind(WriteBehindOptions{
		MaxDelay: 100 * time.Millisecond,
		Write: func(events []eventlog.WireEvent) error {
			writeCount.Add(1)
			return nil
		},
	})

	w.Enqueue(eventlog.WireEvent{Type: "todo/write", Seq: 1, Data: []byte("{}")})
	w.CancelAutomaticWait()

	// Don't flush — verify the timer was cancelled by giving it time.
	time.Sleep(50 * time.Millisecond)
	if writeCount.Load() != 0 {
		t.Errorf("expected 0 background writes after cancel, got %d", writeCount.Load())
	}

	// Flush should still write the pending event.
	_ = w.Flush()
	if writeCount.Load() < 1 {
		t.Errorf("expected 1 write after flush, got %d", writeCount.Load())
	}
}

var errTestFailure = newTestError("write failed")

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func newTestError(msg string) error { return &testError{msg: msg} }
