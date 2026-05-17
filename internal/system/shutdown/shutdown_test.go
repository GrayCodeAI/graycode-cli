package shutdown

import (
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Timeout != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %v", cfg.Timeout)
	}
	if len(cfg.Signals) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(cfg.Signals))
	}
}

func TestNew(t *testing.T) {
	h := New(Config{Timeout: 5 * time.Second})
	if h.timeout != 5*time.Second {
		t.Fatalf("expected 5s timeout, got %v", h.timeout)
	}
}

func TestRegisterAndShutdown(t *testing.T) {
	h := New(Config{Timeout: 5 * time.Second})

	var called int32
	h.Register(func(ctx context.Context) error {
		atomic.AddInt32(&called, 1)
		return nil
	})

	h.Register(func(ctx context.Context) error {
		atomic.AddInt32(&called, 1)
		return nil
	})

	h.Trigger()

	if atomic.LoadInt32(&called) != 2 {
		t.Fatalf("expected 2 hooks called, got %d", called)
	}
}

func TestShutdownWithTimeout(t *testing.T) {
	h := New(Config{Timeout: 100 * time.Millisecond})

	h.Register(func(ctx context.Context) error {
		select {
		case <-time.After(1 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	start := time.Now()
	h.Trigger()
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected timeout, but took %v", elapsed)
	}
}

func TestListen(t *testing.T) {
	h := New(Config{Timeout: 5 * time.Second, Signals: []os.Signal{syscall.SIGUSR1}})

	var called int32
	h.Register(func(ctx context.Context) error {
		atomic.AddInt32(&called, 1)
		return nil
	})

	ctx := h.Listen()

	// Context should not be done yet
	select {
	case <-ctx.Done():
		t.Fatal("context should not be done yet")
	default:
	}

	// Trigger shutdown
	h.Trigger()

	// Context should be done now
	select {
	case <-ctx.Done():
		// Expected
	case <-time.After(1 * time.Second):
		t.Fatal("context should be done after trigger")
	}

	if atomic.LoadInt32(&called) != 1 {
		t.Fatalf("expected hook called, got %d", called)
	}
}

func TestMultipleHooks(t *testing.T) {
	h := New(Config{Timeout: 5 * time.Second})

	var count int32
	for i := 0; i < 5; i++ {
		h.Register(func(ctx context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		})
	}

	h.Trigger()

	if atomic.LoadInt32(&count) != 5 {
		t.Fatalf("expected 5 hooks called, got %d", count)
	}
}
