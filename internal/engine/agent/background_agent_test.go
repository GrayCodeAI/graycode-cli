package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackgroundAgentPool_NewPool(t *testing.T) {
	t.Parallel()
	pool := NewBackgroundAgentPool()
	if pool == nil {
		t.Fatal("NewBackgroundAgentPool returned nil")
	}
	if pool.HasPending() {
		t.Error("new pool should have no pending tasks")
	}
	if pool.PendingCount() != 0 {
		t.Errorf("PendingCount() = %d, want 0", pool.PendingCount())
	}
}

// TestBackgroundAgentPool_StopCancelsInFlight verifies that Stop() cancels
// every in-flight background agent (C8 fix). Previously Submit used
// context.Background(), so agents could never be cancelled via the pool.
func TestBackgroundAgentPool_StopCancelsInFlight(t *testing.T) {
	t.Parallel()
	parent, pcancel := context.WithCancel(context.Background())
	defer pcancel()
	pool := NewBackgroundAgentPoolWithContext(parent)

	var started atomic.Bool
	var cancelled atomic.Bool
	pool.Submit("bg-stop", "wait", func(ctx context.Context, prompt string) (string, error) {
		started.Store(true)
		<-ctx.Done()
		cancelled.Store(true)
		return "", ctx.Err()
	})

	// Wait for the agent to actually start before stopping.
	deadline := time.Now().Add(2 * time.Second)
	for !started.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !started.Load() {
		t.Fatal("background agent did not start")
	}

	pool.Stop()

	deadline = time.Now().Add(2 * time.Second)
	for !cancelled.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !cancelled.Load() {
		t.Error("Stop() did not cancel the in-flight background agent")
	}
	if pool.PendingCount() != 0 {
		t.Errorf("PendingCount() = %d, want 0 after Stop()", pool.PendingCount())
	}
}

// TestBackgroundAgentPool_ParentCancellation verifies that cancelling the
// parent context (session teardown) also cancels in-flight agents.
func TestBackgroundAgentPool_ParentCancellation(t *testing.T) {
	t.Parallel()
	parent, cancel := context.WithCancel(context.Background())
	pool := NewBackgroundAgentPoolWithContext(parent)

	var cancelled atomic.Bool
	pool.Submit("bg-parent", "wait", func(ctx context.Context, prompt string) (string, error) {
		<-ctx.Done()
		cancelled.Store(true)
		return "", ctx.Err()
	})

	time.Sleep(50 * time.Millisecond)
	cancel()

	deadline := time.Now().Add(2 * time.Second)
	for !cancelled.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !cancelled.Load() {
		t.Error("cancelling parent context did not cancel the background agent")
	}
	pool.Stop()
}

func TestBackgroundAgentPool_SubmitAndCollect(t *testing.T) {
	t.Parallel()
	pool := NewBackgroundAgentPool()

	pool.Submit("task-1", "do something", func(ctx context.Context, prompt string) (string, error) {
		time.Sleep(time.Millisecond)
		return "result-1", nil
	})

	// Use WaitAll to ensure task completes deterministically
	results := pool.WaitAll()
	if len(results) != 1 {
		t.Fatalf("WaitAll() returned %d results, want 1", len(results))
	}
	if results[0].ID != "task-1" {
		t.Errorf("ID = %q, want %q", results[0].ID, "task-1")
	}
	if results[0].Output != "result-1" {
		t.Errorf("Output = %q, want %q", results[0].Output, "result-1")
	}
	if results[0].Error != nil {
		t.Errorf("Error = %v, want nil", results[0].Error)
	}
	if results[0].Elapsed <= 0 {
		t.Error("Elapsed should be positive")
	}
}

func TestBackgroundAgentPool_SubmitError(t *testing.T) {
	t.Parallel()
	pool := NewBackgroundAgentPool()
	expectedErr := errors.New("spawn failed")

	pool.Submit("err-task", "fail", func(ctx context.Context, prompt string) (string, error) {
		return "", expectedErr
	})

	time.Sleep(50 * time.Millisecond)

	results := pool.Collect()
	if len(results) != 1 {
		t.Fatalf("Collect() returned %d results, want 1", len(results))
	}
	if results[0].Error == nil || results[0].Error.Error() != expectedErr.Error() {
		t.Errorf("Error = %v, want %v", results[0].Error, expectedErr)
	}
}

func TestBackgroundAgentPool_CollectEmpty(t *testing.T) {
	t.Parallel()
	pool := NewBackgroundAgentPool()
	results := pool.Collect()
	if len(results) != 0 {
		t.Errorf("Collect() on empty pool returned %d results", len(results))
	}
}

func TestBackgroundAgentPool_MultipleSubmits(t *testing.T) {
	t.Parallel()
	pool := NewBackgroundAgentPool()

	for i := 0; i < 5; i++ {
		id := "task-" + string(rune('a'+i))
		pool.Submit(id, "prompt", func(ctx context.Context, prompt string) (string, error) {
			time.Sleep(10 * time.Millisecond)
			return "done", nil
		})
	}

	if pool.PendingCount() != 5 {
		t.Errorf("PendingCount() = %d, want 5", pool.PendingCount())
	}

	time.Sleep(100 * time.Millisecond)

	results := pool.Collect()
	if len(results) != 5 {
		t.Errorf("Collect() returned %d results, want 5", len(results))
	}

	if pool.HasPending() {
		t.Error("HasPending() should be false after all collected")
	}
}

func TestBackgroundAgentPool_WaitAll(t *testing.T) {
	t.Parallel()
	pool := NewBackgroundAgentPool()

	pool.Submit("slow", "wait", func(ctx context.Context, prompt string) (string, error) {
		time.Sleep(100 * time.Millisecond)
		return "waited", nil
	})

	results := pool.WaitAll()
	if len(results) != 1 {
		t.Fatalf("WaitAll() returned %d results, want 1", len(results))
	}
	if results[0].Output != "waited" {
		t.Errorf("Output = %q, want %q", results[0].Output, "waited")
	}
}

func TestBackgroundAgentPool_AllResults(t *testing.T) {
	t.Parallel()
	pool := NewBackgroundAgentPool()

	pool.Submit("r1", "p1", func(ctx context.Context, prompt string) (string, error) {
		return "out1", nil
	})
	pool.Submit("r2", "p2", func(ctx context.Context, prompt string) (string, error) {
		return "out2", nil
	})

	time.Sleep(50 * time.Millisecond)
	pool.Collect()

	all := pool.AllResults()
	if len(all) != 2 {
		t.Errorf("AllResults() returned %d, want 2", len(all))
	}
}

func TestBackgroundAgentPool_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	pool := NewBackgroundAgentPool()
	var count atomic.Int32

	for i := 0; i < 20; i++ {
		pool.Submit("concurrent", "p", func(ctx context.Context, prompt string) (string, error) {
			count.Add(1)
			time.Sleep(10 * time.Millisecond)
			return "ok", nil
		})
	}

	// Concurrent reads while tasks are running
	go func() { pool.HasPending() }()
	go func() { pool.PendingCount() }()
	go func() { pool.Collect() }()

	pool.WaitAll()

	if count.Load() != 20 {
		t.Errorf("expected 20 tasks to run, got %d", count.Load())
	}
}

func TestBackgroundAgentPool_FormatResults_Empty(t *testing.T) {
	t.Parallel()
	result := FormatResults(nil)
	if result != "" {
		t.Errorf("FormatResults(nil) = %q, want empty", result)
	}
}

func TestBackgroundAgentPool_FormatResults_WithResults(t *testing.T) {
	t.Parallel()
	results := []BackgroundResult{
		{ID: "t1", Prompt: "research X", Output: "found Y", Elapsed: time.Second},
		{ID: "t2", Prompt: "check Z", Error: errors.New("failed"), Elapsed: 2 * time.Second},
	}
	formatted := FormatResults(results)
	if formatted == "" {
		t.Error("FormatResults should produce non-empty output")
	}
}
