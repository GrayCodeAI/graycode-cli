package mission

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestWatchdog_NoStall(t *testing.T) {
	stallCalled := false
	w := NewWatchdog(WatchdogConfig{
		StallTimeout:  100 * time.Millisecond,
		CheckInterval: 20 * time.Millisecond,
	}, func(featureID string) {
		stallCalled = true
	})

	w.Register("feat-1")

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	go w.Run(ctx)
	w.Touch("feat-1")
	<-ctx.Done()

	if stallCalled {
		t.Fatal("expected no stall callback when feature is touched within timeout")
	}
}

func TestWatchdog_StallDetected(t *testing.T) {
	var stalledID string
	var mu sync.Mutex

	w := NewWatchdog(WatchdogConfig{
		StallTimeout:  50 * time.Millisecond,
		CheckInterval: 20 * time.Millisecond,
	}, func(featureID string) {
		mu.Lock()
		stalledID = featureID
		mu.Unlock()
	})

	w.Register("feat-stall")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go w.Run(ctx)

	// Wait for stall to be detected
	time.Sleep(150 * time.Millisecond)
	cancel()

	mu.Lock()
	defer mu.Unlock()
	if stalledID != "feat-stall" {
		t.Fatalf("expected stall callback for 'feat-stall', got %q", stalledID)
	}
}

func TestWatchdog_Unregister_StopsMonitoring(t *testing.T) {
	stallCalled := false
	w := NewWatchdog(WatchdogConfig{
		StallTimeout:  50 * time.Millisecond,
		CheckInterval: 20 * time.Millisecond,
	}, func(featureID string) {
		stallCalled = true
	})

	w.Register("feat-unreg")
	w.Unregister("feat-unreg")

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	go w.Run(ctx)
	<-ctx.Done()

	if stallCalled {
		t.Fatal("expected no stall callback after Unregister")
	}
}

func TestWatchdog_ConcurrentAccess(t *testing.T) {
	w := NewWatchdog(WatchdogConfig{
		StallTimeout:  10 * time.Second,
		CheckInterval: 1 * time.Second,
	}, func(featureID string) {})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Run(ctx)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := "feat-" + string(rune('A'+id))
			w.Register(name)
			w.Touch(name)
			w.Unregister(name)
		}(i)
	}
	wg.Wait()
}

func TestWatchdog_ActiveCount(t *testing.T) {
	w := NewWatchdog(WatchdogConfig{}, func(string) {})

	if w.ActiveCount() != 0 {
		t.Fatalf("expected 0, got %d", w.ActiveCount())
	}

	w.Register("a")
	w.Register("b")
	if w.ActiveCount() != 2 {
		t.Fatalf("expected 2, got %d", w.ActiveCount())
	}

	w.Unregister("a")
	if w.ActiveCount() != 1 {
		t.Fatalf("expected 1, got %d", w.ActiveCount())
	}
}
