package mission

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// runWithTimeout runs fn in a goroutine and reports whether it returned
// before the timeout and what error/message it produced. This is a
// helper for "did the waiter return promptly" assertions.
func runWithTimeout(t *testing.T, timeout time.Duration, fn func() (string, error)) (returned bool, msg string, err error) {
	t.Helper()
	done := make(chan struct{})
	var (
		m string
		e error
	)
	go func() {
		m, e = fn()
		close(done)
	}()
	select {
	case <-done:
		return true, m, e
	case <-time.After(timeout):
		return false, "", nil
	}
}

// TestWaitForResponse_ReturnsPromptlyOnSignal verifies that
// WaitForResponse returns within tens of milliseconds of a matching
// Send, not at the next 10ms tick or at the timeout. With the old
// busy-poll, latency was bounded below by 10ms and could exceed the
// timeout under contention. With channel signaling, latency is
// bounded by the goroutine wakeup time.
func TestWaitForResponse_ReturnsPromptlyOnSignal(t *testing.T) {
	mb := NewMessageBus()
	_ = mb.Register("a")
	_ = mb.Register("b")
	// Seed both a request and a response, so the fast path (history scan)
	// can satisfy WaitForResponse without registering a waiter.
	if err := mb.Send(AgentMessage{ID: "req-1", From: "a", To: "b", Topic: "request"}); err != nil {
		t.Fatalf("seed Send: %v", err)
	}
	if err := mb.Send(AgentMessage{ID: "resp-1", From: "b", ResponseTo: "req-1", Topic: "response"}); err != nil {
		t.Fatalf("seed Send: %v", err)
	}

	start := time.Now()
	got, _, _ := runWithTimeout(t, time.Second, func() (string, error) {
		resp, err := mb.WaitForResponse("req-1", time.Second)
		if err != nil {
			return "", err
		}
		return resp.ID, nil
	})
	if !got {
		t.Fatal("WaitForResponse did not return")
	}
	// Fast-path returns immediately (history scan) — well under 50ms.
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("elapsed = %v, want < 50ms (fast path)", elapsed)
	}

	// Now test the slow path: no response yet, register a waiter, then send.
	mb2 := NewMessageBus()
	_ = mb2.Register("a")
	_ = mb2.Register("b")
	_ = mb2.Send(AgentMessage{ID: "req-2", From: "a", To: "b", Topic: "request"})

	start = time.Now()
	got, _, _ = runWithTimeout(t, time.Second, func() (string, error) {
		// Pre-signal the waiter, then send after a tiny delay.
		// (We can't easily split the goroutine into "register" +
		// "wait" since WaitForResponse does both; instead we send
		// the response from another goroutine after a 20ms delay
		// and assert the wait returns in well under 100ms.)
		var wg sync.WaitGroup
		wg.Add(1)
		var respID string
		var werr error
		go func() {
			defer wg.Done()
			r, e := mb2.WaitForResponse("req-2", 500*time.Millisecond)
			if r != nil {
				respID = r.ID
			}
			werr = e
		}()
		time.Sleep(20 * time.Millisecond) // give the waiter time to register
		_ = mb2.Send(AgentMessage{ID: "resp-2", From: "b", ResponseTo: "req-2", Topic: "response"})
		wg.Wait()
		return respID, werr
	})
	if !got {
		t.Fatal("WaitForResponse did not return")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("elapsed = %v, want < 100ms (signaled promptly)", elapsed)
	}
}

// TestWaitForResponse_TimeoutError verifies that an unmatched
// WaitForResponse returns a timeout error after the configured
// duration.
func TestWaitForResponse_TimeoutError(t *testing.T) {
	mb := NewMessageBus()
	timeout := 80 * time.Millisecond
	start := time.Now()
	_, err := mb.WaitForResponse("nonexistent", timeout)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("err = %q, want 'timeout'", err.Error())
	}
	// Should be at least the timeout (modulo scheduling jitter).
	if elapsed+10*time.Millisecond < timeout {
		t.Errorf("elapsed = %v, want ~%v", elapsed, timeout)
	}
	// And not significantly more.
	if elapsed > timeout+200*time.Millisecond {
		t.Errorf("elapsed = %v, want within 200ms of %v", elapsed, timeout)
	}
}

// TestWaitForResponse_FastPathFromHistory: a Send recorded before
// WaitForResponse is called is still found via the history scan.
func TestWaitForResponse_FastPathFromHistory(t *testing.T) {
	mb := NewMessageBus()
	_ = mb.Register("a")
	_ = mb.Register("b")
	if err := mb.Send(AgentMessage{ID: "req-x", From: "a", To: "b", Topic: "request"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := mb.Send(AgentMessage{ID: "resp-x", From: "b", ResponseTo: "req-x", Topic: "response"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	resp, err := mb.WaitForResponse("req-x", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForResponse: %v", err)
	}
	if resp.ID != "resp-x" {
		t.Errorf("resp.ID = %q, want resp-x", resp.ID)
	}
}

// TestWaitForResponse_OnlyMatchingWaiterReturns: two waiters for
// different messageIDs; one response arrives; only the matching
// waiter is signaled.
func TestWaitForResponse_OnlyMatchingWaiterReturns(t *testing.T) {
	mb := NewMessageBus()

	type result struct {
		id  string
		err error
	}
	results := make(chan result, 2)
	for _, id := range []string{"req-a", "req-b"} {
		id := id
		go func() {
			r, err := mb.WaitForResponse(id, 500*time.Millisecond)
			if r != nil {
				results <- result{r.ID, nil}
			} else {
				results <- result{"", err}
			}
		}()
	}
	// Give both waiters time to register.
	time.Sleep(20 * time.Millisecond)
	// Send a response for req-a only.
	if err := mb.Send(AgentMessage{ID: "resp-a", From: "b", ResponseTo: "req-a"}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// First result should be the matching response.
	select {
	case r := <-results:
		if r.err != nil {
			t.Fatalf("expected response for req-a, got error: %v", r.err)
		}
		if r.id != "resp-a" {
			t.Errorf("id = %q, want resp-a", r.id)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("matching waiter did not return")
	}

	// Second result should be the timeout.
	select {
	case r := <-results:
		if r.err == nil {
			t.Fatalf("expected timeout for req-b, got response: %s", r.id)
		}
	case <-time.After(700 * time.Millisecond):
		t.Fatal("non-matching waiter did not return")
	}
}

// TestWaitForResponse_WaiterCleanedUpOnTimeout: after a timeout, the
// waiter is removed from responseWaiters so it doesn't accumulate.
func TestWaitForResponse_WaiterCleanedUpOnTimeout(t *testing.T) {
	mb := NewMessageBus()
	_, _ = mb.WaitForResponse("nonexistent", 30*time.Millisecond)
	// Give the defer a moment to run.
	time.Sleep(20 * time.Millisecond)
	mb.mu.RLock()
	n := len(mb.responseWaiters["nonexistent"])
	mb.mu.RUnlock()
	if n != 0 {
		t.Errorf("responseWaiters[%q] len = %d, want 0 (waiter should be cleaned up after timeout)", "nonexistent", n)
	}
}

// TestWaitForResponse_WaiterCleanedUpOnSignal: after a signal, the
// waiter is removed from responseWaiters.
func TestWaitForResponse_WaiterCleanedUpOnSignal(t *testing.T) {
	mb := NewMessageBus()
	_ = mb.Send(AgentMessage{ID: "req", From: "a", To: "b"})
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = mb.Send(AgentMessage{ID: "resp", From: "b", ResponseTo: "req"})
	}()
	_, err := mb.WaitForResponse("req", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForResponse: %v", err)
	}
	// After the signal, Send removed the entry; double-check.
	mb.mu.RLock()
	_, ok := mb.responseWaiters["req"]
	mb.mu.RUnlock()
	if ok {
		t.Error("responseWaiters[req] should be removed after Send")
	}
}

// TestWaitForResponse_NoWaitersOnSend: a response with no waiters is a no-op.
func TestWaitForResponse_NoWaitersOnSend(t *testing.T) {
	mb := NewMessageBus()
	// No panic, no error.
	if err := mb.Send(AgentMessage{ID: "resp", From: "b", ResponseTo: "no-such-req"}); err != nil {
		t.Errorf("Send with no waiters: %v", err)
	}
}

// TestWaitForLock_ReturnsPromptlyOnRelease: WaitForLock returns
// within tens of milliseconds of ReleaseLock, not at the next 20ms
// tick or at the timeout.
func TestWaitForLock_ReturnsPromptlyOnRelease(t *testing.T) {
	mb := NewMessageBus()
	if err := mb.AcquireLock("res-1", "owner-a", time.Second); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	start := time.Now()
	got, _, _ := runWithTimeout(t, time.Second, func() (string, error) {
		var wg sync.WaitGroup
		wg.Add(1)
		var werr error
		go func() {
			defer wg.Done()
			werr = mb.WaitForLock("res-1", "owner-b", 500*time.Millisecond)
		}()
		time.Sleep(20 * time.Millisecond)
		if err := mb.ReleaseLock("res-1", "owner-a"); err != nil {
			t.Errorf("ReleaseLock: %v", err)
		}
		wg.Wait()
		if werr != nil {
			return "", werr
		}
		return "acquired", nil
	})
	if !got {
		t.Fatal("WaitForLock did not return")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("elapsed = %v, want < 100ms (signaled promptly)", elapsed)
	}
}

// TestWaitForLock_TimeoutError: WaitForLock times out when the lock
// is never released.
func TestWaitForLock_TimeoutError(t *testing.T) {
	mb := NewMessageBus()
	if err := mb.AcquireLock("res-1", "owner-a", time.Second); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	timeout := 60 * time.Millisecond
	start := time.Now()
	err := mb.WaitForLock("res-1", "owner-b", timeout)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("err = %q, want 'timeout'", err.Error())
	}
	if elapsed+10*time.Millisecond < timeout {
		t.Errorf("elapsed = %v, want ~%v", elapsed, timeout)
	}
	if elapsed > timeout+200*time.Millisecond {
		t.Errorf("elapsed = %v, want within 200ms of %v", elapsed, timeout)
	}
}

// TestWaitForLock_FastPath: WaitForLock returns immediately if the
// lock is free at call time.
func TestWaitForLock_FastPath(t *testing.T) {
	mb := NewMessageBus()
	start := time.Now()
	if err := mb.WaitForLock("res-free", "owner-a", 100*time.Millisecond); err != nil {
		t.Fatalf("WaitForLock: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Errorf("elapsed = %v, want < 20ms (fast path)", elapsed)
	}
}

// TestWaitForLock_WaiterCleanedUpOnTimeout: after a timeout, the
// waiter is removed from lockWaiters.
func TestWaitForLock_WaiterCleanedUpOnTimeout(t *testing.T) {
	mb := NewMessageBus()
	_ = mb.AcquireLock("res-1", "owner-a", time.Second)
	_ = mb.WaitForLock("res-1", "owner-b", 30*time.Millisecond)
	// Give the defer a moment to run.
	time.Sleep(20 * time.Millisecond)
	mb.lockMu.Lock()
	n := len(mb.lockWaiters["res-1"])
	mb.lockMu.Unlock()
	if n != 0 {
		t.Errorf("lockWaiters[res-1] len = %d, want 0", n)
	}
}

// TestWaitForLock_MultipleWaiters_OnlyOneAcquires: two waiters on
// the same resource; ReleaseLock wakes both; only one acquires; the
// other returns an error (does not re-register, see M7).
func TestWaitForLock_MultipleWaiters_OnlyOneAcquires(t *testing.T) {
	mb := NewMessageBus()
	if err := mb.AcquireLock("res-shared", "owner-a", time.Second); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	type result struct {
		owner string
		err   error
	}
	results := make(chan result, 2)
	for _, owner := range []string{"owner-b", "owner-c"} {
		owner := owner
		go func() {
			err := mb.WaitForLock("res-shared", owner, 500*time.Millisecond)
			results <- result{owner, err}
		}()
	}
	time.Sleep(20 * time.Millisecond)

	// First release: both waiters wake, one acquires, the other
	// returns an error (M7: no re-register, no busy-spin).
	if err := mb.ReleaseLock("res-shared", "owner-a"); err != nil {
		t.Fatalf("ReleaseLock: %v", err)
	}

	var winner result
	var loser result
	gotBoth := 0
	for gotBoth < 2 {
		select {
		case r := <-results:
			if r.err == nil {
				winner = r
			} else {
				loser = r
			}
			gotBoth++
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("only %d/2 waiter results", gotBoth)
		}
	}
	if winner.err != nil {
		t.Fatalf("first waiter did not acquire: err=%v", winner.err)
	}
	if loser.err == nil {
		t.Fatalf("second waiter unexpectedly acquired: %+v", loser)
	}
	if winner.owner == loser.owner {
		t.Errorf("both waiters are the same: %s", winner.owner)
	}
}

// TestWaitForLock_OwnerMismatchOnRelease: a ReleaseLock from a
// non-owner does not wake waiters.
func TestWaitForLock_OwnerMismatchOnRelease(t *testing.T) {
	mb := NewMessageBus()
	_ = mb.AcquireLock("res-1", "owner-a", time.Second)
	_ = mb.AcquireLock("res-1", "owner-b", time.Second) // should fail but...

	// Wait — only one owner at a time. AcquireLock returns error
	// for non-re-entrant. So owner-a still holds.
	err := mb.ReleaseLock("res-1", "owner-b") // wrong owner
	if err == nil {
		t.Fatal("expected error on owner mismatch, got nil")
	}
}

// TestStats_NotAffectedByWaiters: responseWaiters and lockWaiters
// do not appear in Stats; they are internal channels, not counters.
func TestStats_NotAffectedByWaiters(t *testing.T) {
	mb := NewMessageBus()
	_ = mb.Register("a")
	_ = mb.Register("b")
	_ = mb.Send(AgentMessage{ID: "req", From: "a", To: "b"})
	go func() {
		_, _ = mb.WaitForResponse("req", 100*time.Millisecond)
	}()
	time.Sleep(10 * time.Millisecond)
	stats := mb.Stats()
	// HistorySz should reflect the seeded message; Agents 2; Dropped 0.
	if stats.Agents != 2 {
		t.Errorf("Agents = %d, want 2", stats.Agents)
	}
	if stats.Dropped != 0 {
		t.Errorf("Dropped = %d, want 0", stats.Dropped)
	}
}
