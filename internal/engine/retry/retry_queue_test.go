package retry

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewRetryQueue(t *testing.T) {
	rq := NewRetryQueue()

	if rq.MaxSize != 100 {
		t.Errorf("expected MaxSize 100, got %d", rq.MaxSize)
	}
	if rq.BackoffBase != 1*time.Second {
		t.Errorf("expected BackoffBase 1s, got %v", rq.BackoffBase)
	}
	if rq.BackoffMax != 5*time.Minute {
		t.Errorf("expected BackoffMax 5m, got %v", rq.BackoffMax)
	}
	if len(rq.Items) != 0 {
		t.Errorf("expected empty items, got %d", len(rq.Items))
	}
}

func TestEnqueue(t *testing.T) {
	rq := NewRetryQueue()

	args := map[string]interface{}{"file": "src/auth.go"}
	item := rq.Enqueue("Edit", args, "old_str not found", 1)

	if item == nil {
		t.Fatal("expected item, got nil")
	}
	if item.Operation != "Edit" {
		t.Errorf("expected operation Edit, got %s", item.Operation)
	}
	if item.Error != "old_str not found" {
		t.Errorf("expected error 'old_str not found', got %s", item.Error)
	}
	if item.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", item.Attempts)
	}
	if item.Priority != 1 {
		t.Errorf("expected priority 1, got %d", item.Priority)
	}
	if item.Status != "pending" {
		t.Errorf("expected status pending, got %s", item.Status)
	}
	if item.MaxAttempts != 5 {
		t.Errorf("expected max attempts 5, got %d", item.MaxAttempts)
	}
	if rq.Size() != 1 {
		t.Errorf("expected size 1, got %d", rq.Size())
	}
}

func TestEnqueueDeduplication(t *testing.T) {
	rq := NewRetryQueue()

	args := map[string]interface{}{"file": "src/auth.go"}
	item1 := rq.Enqueue("Edit", args, "first error", 1)
	item2 := rq.Enqueue("Edit", args, "second error", 1)

	if rq.Size() != 1 {
		t.Errorf("expected size 1 after dedup, got %d", rq.Size())
	}
	if item1.ID != item2.ID {
		t.Error("expected same item returned for deduplicated enqueue")
	}
	if item2.Attempts != 2 {
		t.Errorf("expected 2 attempts after dedup, got %d", item2.Attempts)
	}
	if item2.Error != "second error" {
		t.Errorf("expected error updated to 'second error', got %s", item2.Error)
	}
}

func TestEnqueueNoDedupDifferentArgs(t *testing.T) {
	rq := NewRetryQueue()

	args1 := map[string]interface{}{"file": "src/auth.go"}
	args2 := map[string]interface{}{"file": "src/main.go"}

	rq.Enqueue("Edit", args1, "error 1", 1)
	rq.Enqueue("Edit", args2, "error 2", 2)

	if rq.Size() != 2 {
		t.Errorf("expected size 2 for different args, got %d", rq.Size())
	}
}

func TestEnqueueMaxSize(t *testing.T) {
	rq := NewRetryQueue()
	rq.MaxSize = 3

	for i := 0; i < 3; i++ {
		args := map[string]interface{}{"index": i}
		item := rq.Enqueue("Op", args, "err", 1)
		if item == nil {
			t.Fatalf("expected item at index %d, got nil", i)
		}
	}

	// Fourth should be rejected
	args := map[string]interface{}{"index": 99}
	item := rq.Enqueue("Op", args, "err", 1)
	if item != nil {
		t.Error("expected nil when queue is full")
	}
}

func TestDequeue(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond
	rq.BackoffMax = 10 * time.Millisecond

	args := map[string]interface{}{"cmd": "go test"}
	rq.Enqueue("Bash", args, "compilation error", 2)

	// Initially the item should not be ready (backoff not elapsed)
	// But with 1ms backoff, we can wait briefly
	time.Sleep(10 * time.Millisecond)

	item := rq.Dequeue()
	if item == nil {
		t.Fatal("expected item from dequeue, got nil")
	}
	if item.Status != "retrying" {
		t.Errorf("expected status retrying, got %s", item.Status)
	}
}

func TestDequeuePriority(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond
	rq.BackoffMax = 5 * time.Millisecond

	rq.Enqueue("LowPri", map[string]interface{}{"x": "low"}, "err", 3)
	rq.Enqueue("HighPri", map[string]interface{}{"x": "high"}, "err", 1)
	rq.Enqueue("MedPri", map[string]interface{}{"x": "med"}, "err", 2)

	time.Sleep(15 * time.Millisecond)

	item := rq.Dequeue()
	if item == nil {
		t.Fatal("expected item")
	}
	if item.Operation != "HighPri" {
		t.Errorf("expected highest priority item (HighPri), got %s", item.Operation)
	}
}

func TestDequeueEmpty(t *testing.T) {
	rq := NewRetryQueue()
	item := rq.Dequeue()
	if item != nil {
		t.Error("expected nil from empty queue")
	}
}

func TestMarkSuccess(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond

	item := rq.Enqueue("Op", map[string]interface{}{}, "err", 1)
	rq.MarkSuccess(item.ID)

	if item.Status != "succeeded" {
		t.Errorf("expected status succeeded, got %s", item.Status)
	}
}

func TestMarkFailed(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond

	item := rq.Enqueue("Op", map[string]interface{}{}, "initial error", 1)
	item.MaxAttempts = 3

	rq.MarkFailed(item.ID, "second error")
	if item.Status != "pending" {
		t.Errorf("expected status pending after first failure, got %s", item.Status)
	}
	if item.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", item.Attempts)
	}
	if item.Error != "second error" {
		t.Errorf("expected error 'second error', got %s", item.Error)
	}

	// Fail again to reach max
	rq.MarkFailed(item.ID, "third error")
	if item.Status != "failed_permanent" {
		t.Errorf("expected failed_permanent after max attempts, got %s", item.Status)
	}
}

func TestMarkFailedNonexistent(t *testing.T) {
	rq := NewRetryQueue()
	// Should not panic
	rq.MarkFailed("nonexistent_id", "some error")
}

func TestGetReady(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond
	rq.BackoffMax = 5 * time.Millisecond

	rq.Enqueue("Op1", map[string]interface{}{"a": 1}, "err", 2)
	rq.Enqueue("Op2", map[string]interface{}{"b": 2}, "err", 1)

	time.Sleep(15 * time.Millisecond)

	ready := rq.GetReady()
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready items, got %d", len(ready))
	}
	// Should be sorted by priority
	if ready[0].Operation != "Op2" {
		t.Errorf("expected Op2 first (higher priority), got %s", ready[0].Operation)
	}
}

func TestGetReadyExcludesNotReady(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 10 * time.Second // long backoff

	rq.Enqueue("Op1", map[string]interface{}{}, "err", 1)

	ready := rq.GetReady()
	if len(ready) != 0 {
		t.Errorf("expected 0 ready items with long backoff, got %d", len(ready))
	}
}

func TestRetryQueueGetPending(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond

	rq.Enqueue("Op1", map[string]interface{}{"a": 1}, "err", 3)
	rq.Enqueue("Op2", map[string]interface{}{"b": 2}, "err", 1)
	item3 := rq.Enqueue("Op3", map[string]interface{}{"c": 3}, "err", 2)
	rq.MarkSuccess(item3.ID)

	pending := rq.GetPending()
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending items, got %d", len(pending))
	}
	// Sorted by priority
	if pending[0].Operation != "Op2" {
		t.Errorf("expected Op2 first, got %s", pending[0].Operation)
	}
}

func TestCalculateBackoff(t *testing.T) {
	rq := NewRetryQueue()

	// Attempt 1: base * 2^1 = 2s + jitter
	d1 := rq.CalculateBackoff(1)
	if d1 < 2*time.Second || d1 > 3*time.Second {
		t.Errorf("attempt 1 backoff out of range: %v", d1)
	}

	// Attempt 3: base * 2^3 = 8s + jitter
	d3 := rq.CalculateBackoff(3)
	if d3 < 8*time.Second || d3 > 10*time.Second {
		t.Errorf("attempt 3 backoff out of range: %v", d3)
	}

	// Attempt 20: should be capped at BackoffMax (5min) + jitter
	d20 := rq.CalculateBackoff(20)
	maxWithJitter := rq.BackoffMax + time.Duration(float64(rq.BackoffMax)*0.25)
	if d20 > maxWithJitter {
		t.Errorf("attempt 20 backoff should be capped, got %v", d20)
	}
}

func TestCalculateBackoffIncreases(t *testing.T) {
	rq := NewRetryQueue()

	// Run multiple samples to verify the trend (accounting for jitter)
	var sum1, sum2, sum3 time.Duration
	samples := 100
	for i := 0; i < samples; i++ {
		sum1 += rq.CalculateBackoff(1)
		sum2 += rq.CalculateBackoff(2)
		sum3 += rq.CalculateBackoff(3)
	}

	avg1 := sum1 / time.Duration(samples)
	avg2 := sum2 / time.Duration(samples)
	avg3 := sum3 / time.Duration(samples)

	if avg2 <= avg1 {
		t.Errorf("backoff should increase: avg1=%v, avg2=%v", avg1, avg2)
	}
	if avg3 <= avg2 {
		t.Errorf("backoff should increase: avg2=%v, avg3=%v", avg2, avg3)
	}
}

func TestRetryQueuePrune(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond

	item1 := rq.Enqueue("Op1", map[string]interface{}{"a": 1}, "err", 1)
	item2 := rq.Enqueue("Op2", map[string]interface{}{"b": 2}, "err", 2)
	rq.Enqueue("Op3", map[string]interface{}{"c": 3}, "err", 3)

	rq.MarkSuccess(item1.ID)
	item2.MaxAttempts = 1
	rq.MarkFailed(item2.ID, "permanent")

	// Items are not old enough to prune
	rq.Prune()
	if rq.Size() != 3 {
		t.Errorf("expected 3 items (not old enough to prune), got %d", rq.Size())
	}

	// Artificially age the items
	item1.CreatedAt = time.Now().Add(-2 * time.Hour)
	item2.CreatedAt = time.Now().Add(-2 * time.Hour)

	rq.Prune()
	if rq.Size() != 1 {
		t.Errorf("expected 1 item after pruning old completed items, got %d", rq.Size())
	}
	if rq.Items[0].Operation != "Op3" {
		t.Errorf("expected Op3 to remain, got %s", rq.Items[0].Operation)
	}
}

func TestPruneKeepsPendingItems(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond

	item := rq.Enqueue("Op1", map[string]interface{}{}, "err", 1)
	item.CreatedAt = time.Now().Add(-2 * time.Hour)

	rq.Prune()
	if rq.Size() != 1 {
		t.Errorf("pending items should not be pruned, got size %d", rq.Size())
	}
}

func TestFormatQueue(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 5 * time.Second

	rq.Enqueue("Edit", map[string]interface{}{"file": "src/auth.go"}, "old_str not found", 1)
	rq.Enqueue("Bash", map[string]interface{}{"command": "go test"}, "compilation error", 2)

	item3 := rq.Enqueue("WebFetch", map[string]interface{}{}, "timeout", 3)
	item3.MaxAttempts = 3
	item3.Attempts = 3
	item3.Status = "failed_permanent"

	output := rq.FormatQueue()

	if !strings.Contains(output, "Retry Queue (2 pending)") {
		t.Errorf("expected '2 pending' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "[P1] Edit src/auth.go") {
		t.Errorf("expected Edit operation in output, got:\n%s", output)
	}
	if !strings.Contains(output, "[P2] Bash \"go test\"") {
		t.Errorf("expected Bash operation in output, got:\n%s", output)
	}
	if !strings.Contains(output, "PERMANENT FAILURE") {
		t.Errorf("expected PERMANENT FAILURE in output, got:\n%s", output)
	}
	if !strings.Contains(output, "\"old_str not found\"") {
		t.Errorf("expected error message in output, got:\n%s", output)
	}
	if !strings.Contains(output, "─────") {
		t.Errorf("expected separator in output, got:\n%s", output)
	}
}

func TestFormatQueueEmpty(t *testing.T) {
	rq := NewRetryQueue()
	output := rq.FormatQueue()
	if output != "Retry Queue (empty)" {
		t.Errorf("expected 'Retry Queue (empty)', got: %s", output)
	}
}

func TestSize(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond

	if rq.Size() != 0 {
		t.Errorf("expected size 0, got %d", rq.Size())
	}

	rq.Enqueue("Op1", map[string]interface{}{"a": 1}, "err", 1)
	rq.Enqueue("Op2", map[string]interface{}{"b": 2}, "err", 2)

	if rq.Size() != 2 {
		t.Errorf("expected size 2, got %d", rq.Size())
	}
}

func TestRetryQueueClear(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond

	rq.Enqueue("Op1", map[string]interface{}{}, "err", 1)
	rq.Enqueue("Op2", map[string]interface{}{"x": 1}, "err", 2)

	rq.Clear()
	if rq.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", rq.Size())
	}
}

func TestRetryQueueConcurrentAccess(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond
	rq.BackoffMax = 5 * time.Millisecond

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			args := map[string]interface{}{"idx": idx}
			rq.Enqueue("Op", args, "err", idx%5)
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rq.GetPending()
			rq.GetReady()
			rq.Size()
			rq.FormatQueue()
		}()
	}
	wg.Wait()

	// Concurrent dequeue
	time.Sleep(15 * time.Millisecond)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			item := rq.Dequeue()
			if item != nil {
				rq.MarkSuccess(item.ID)
			}
		}()
	}
	wg.Wait()
}

func TestDeduplicationOnlyAffectsActiveItems(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond

	args := map[string]interface{}{"file": "test.go"}

	item1 := rq.Enqueue("Edit", args, "err1", 1)
	rq.MarkSuccess(item1.ID)

	// After marking success, same operation should create new item
	item2 := rq.Enqueue("Edit", args, "err2", 1)
	if item2.ID == item1.ID {
		t.Error("expected new item after previous was marked succeeded")
	}
	if rq.Size() != 2 {
		t.Errorf("expected size 2, got %d", rq.Size())
	}
}

func TestMarkFailedRecalculatesBackoff(t *testing.T) {
	rq := NewRetryQueue()
	rq.BackoffBase = 1 * time.Millisecond
	rq.BackoffMax = 1 * time.Second

	item := rq.Enqueue("Op", map[string]interface{}{}, "err", 1)
	firstRetry := item.NextRetry

	time.Sleep(5 * time.Millisecond)
	rq.MarkFailed(item.ID, "new error")

	if !item.NextRetry.After(firstRetry) {
		t.Error("expected NextRetry to be updated after MarkFailed")
	}
}

func TestRetryQueueFormatDuration(t *testing.T) {
	rq := NewRetryQueue()

	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "0s"},
		{4 * time.Second, "4s"},
		{90 * time.Second, "1m30s"},
		{2 * time.Hour, "2h0m"},
	}

	for _, tt := range tests {
		got := rq.formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %s, want %s", tt.d, got, tt.want)
		}
	}
}
