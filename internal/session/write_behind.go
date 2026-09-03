// Package session provides the Go-native port of DeepSeek Harness's
// write-behind batching controller (dsh-session-persistence/write-behind.ts).
//
// WriteBehind buffers session events in memory and flushes them in batches
// on a fixed timer deadline or when explicitly flushed. This ports the DSH
// SessionWriteBehind class: a bounded per-session write batching controller
// with failure retention, background flushing, and a quiescence barrier.
package session

import (
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
)

// WriteBehindOptions configures one session's write-behind controller,
// mirroring DSH's SessionWriteBehindOptions.
type WriteBehindOptions struct {
	// MaxDelay is the maximum intentional batching wait after an idle queue
	// receives work. Ported from DSH's maxDelayMs.
	MaxDelay time.Duration

	// Write persists one stable ordered prefix of events; it must block until
	// the backend confirms durability. Ported from DSH's write().
	Write func(events []eventlog.WireEvent) error

	// ReportBackgroundFailure observes a detached background write failure
	// without rejecting the producer. Ported from DSH's reportBackgroundFailure.
	ReportBackgroundFailure func(eventlog.WireEvent, error)
}

// WriteBehind owns one session's pending events, fixed batching deadline,
// active write, failure retention, and explicit quiescence barrier.
// Ported from DSH's SessionWriteBehind class.
//
// Concurrency model: all state is protected by mu. Background writes run in
// goroutines that signal completion via doneCh. Flush() uses a cond-based
// approach to wait for quiescence without polling.
type WriteBehind struct {
	opts WriteBehindOptions

	mu       sync.Mutex
	cond     sync.Cond
	pending  []eventlog.WireEvent
	timer    *time.Timer
	active   bool
	doneCh   chan struct{} // closed when the active write completes
	barrier  chan struct{} // non-nil during an explicit flush (others join it)
	deadline bool          // deadline expired while a write was active
	paused   bool          // automatic path paused due to a background failure
}

// NewWriteBehind creates a new write-behind controller for one session.
func NewWriteBehind(opts WriteBehindOptions) *WriteBehind {
	w := &WriteBehind{opts: opts}
	w.cond.L = &w.mu
	return w
}

// HasWork reports whether this controller owns queued events or an active write.
func (w *WriteBehind) HasWork() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.pending) > 0 || w.active
}

// PendingCount returns the number of events currently buffered (not yet written).
func (w *WriteBehind) PendingCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.pending)
}

// Enqueue copies one event into the persistence-owned queue and starts a
// fixed deadline when the automatic path is idle. Ported from DSH's enqueue().
func (w *WriteBehind) Enqueue(ev eventlog.WireEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()

	wasEmpty := len(w.pending) == 0
	w.pending = append(w.pending, ev)

	// If a barrier flush is in progress, don't arm the timer — the drain
	// will pick up the new event.
	if w.barrier != nil {
		w.cond.Broadcast()
		return
	}

	if w.paused {
		// A previous background write failed; resume the automatic timer.
		w.paused = false
		w.deadline = false
		w.armTimerLocked()
		return
	}

	if wasEmpty {
		w.armTimerLocked()
	}
}

// Flush cancels the batching wait and durably drains to a quiescent point.
// Concurrent callers join the same barrier. Returns after all pending events
// are durably written. Ported from DSH's flush().
func (w *WriteBehind) Flush() error {
	w.mu.Lock()

	// If a flush is already in progress, join its barrier.
	if w.barrier != nil {
		ch := w.barrier
		w.mu.Unlock()
		<-ch
		return nil
	}

	w.cancelTimerLocked()
	w.deadline = false
	w.paused = false

	barrier := make(chan struct{})
	w.barrier = barrier

	// Wait for any active background write to complete before draining.
	activeDone := w.doneCh
	if w.active && activeDone == nil {
		activeDone = make(chan struct{})
		w.doneCh = activeDone
	}
	w.mu.Unlock()

	// Wait for the active background write (if any).
	if activeDone != nil {
		<-activeDone
	}

	// Now drain all pending events synchronously.
	err := w.drain()

	w.mu.Lock()
	w.barrier = nil
	close(barrier)
	w.mu.Unlock()

	return err
}

// CancelAutomaticWait cancels the current automatic deadline without draining
// retained work. Ported from DSH's cancelAutomaticWait().
func (w *WriteBehind) CancelAutomaticWait() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cancelTimerLocked()
	w.deadline = false
}

// --- internal helpers ---

func (w *WriteBehind) armTimerLocked() {
	if w.timer != nil {
		return
	}
	w.timer = time.AfterFunc(w.opts.MaxDelay, func() {
		w.mu.Lock()
		w.timer = nil
		if w.active {
			w.deadline = true
			w.mu.Unlock()
			return
		}
		if len(w.pending) == 0 {
			w.mu.Unlock()
			return
		}
		w.startBackgroundLocked()
		w.mu.Unlock()
	})
}

func (w *WriteBehind) cancelTimerLocked() {
	if w.timer == nil {
		return
	}
	w.timer.Stop()
	w.timer = nil
}

// startBackgroundLocked launches an asynchronous write for the current pending
// prefix. Must be called with mu held. Ported from DSH's startBackground().
func (w *WriteBehind) startBackgroundLocked() {
	batch := w.pending
	w.pending = nil
	w.cancelTimerLocked()
	w.deadline = false
	w.active = true
	w.doneCh = make(chan struct{})

	doneCh := w.doneCh
	go func() {
		err := w.opts.Write(batch)
		w.mu.Lock()
		close(doneCh)
		w.doneCh = nil
		w.active = false
		if err != nil {
			// Re-queue the batch (preserving order) and pause automatic path.
			w.pending = append(batch, w.pending...)
			w.cancelTimerLocked()
			w.deadline = false
			w.paused = true
			if w.opts.ReportBackgroundFailure != nil {
				for _, ev := range batch {
					w.opts.ReportBackgroundFailure(ev, err)
				}
			}
			w.cond.Broadcast()
		} else {
			// If deadline expired while this write was active, start another
			// background write for remaining work (but not during a barrier flush).
			if w.deadline && len(w.pending) > 0 && w.barrier == nil {
				w.deadline = false
				w.startBackgroundLocked()
			} else if w.barrier != nil {
				w.cond.Broadcast()
			}
		}
		w.mu.Unlock()
	}()
}

// drain synchronously flushes all pending events in order. Must not be called
// while a background write is active (caller must wait for doneCh first).
// Ported from DSH's drainBarrier: it drains the CURRENT pending prefix; events
// enqueued during the drain that arrive after the last batch is written are left
// in pending for the next flush. The barrier is released by Flush after drain
// returns.
func (w *WriteBehind) drain() error {
	for {
		w.mu.Lock()
		if w.active {
			// Should not happen if caller waited for doneCh, but guard anyway.
			activeDone := w.doneCh
			if activeDone == nil {
				activeDone = make(chan struct{})
				w.doneCh = activeDone
			}
			w.mu.Unlock()
			<-activeDone
			continue
		}
		if len(w.pending) == 0 {
			w.mu.Unlock()
			return nil
		}
		batch := w.pending
		w.pending = nil
		w.cancelTimerLocked()
		w.deadline = false
		w.active = true
		w.doneCh = make(chan struct{})
		doneCh := w.doneCh
		w.mu.Unlock()

		err := w.opts.Write(batch)

		w.mu.Lock()
		close(doneCh)
		w.doneCh = nil
		w.active = false
		if err != nil {
			// Re-queue on failure.
			w.pending = append(batch, w.pending...)
			w.cancelTimerLocked()
			w.deadline = false
			w.paused = true
			if w.opts.ReportBackgroundFailure != nil {
				for _, ev := range batch {
					w.opts.ReportBackgroundFailure(ev, err)
				}
			}
			w.mu.Unlock()
			return err
		}
		w.mu.Unlock()
	}
}
