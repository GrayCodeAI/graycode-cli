package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/testutil"
)

// TestIPLimiter_BurstAndPerIPIsolation verifies the token bucket allows up to
// burst requests immediately, then throttles, while other IPs are unaffected.
func TestIPLimiter_BurstAndPerIPIsolation(t *testing.T) {
	l := newIPLimiter(1, 4) // 1 token/sec, burst 4
	for i := 0; i < 4; i++ {
		if !l.Allow("198.51.100.1") {
			t.Fatalf("request %d within burst should be allowed", i)
		}
	}
	if l.Allow("198.51.100.1") {
		t.Error("expected request beyond burst to be limited")
	}
	if !l.Allow("198.51.100.2") {
		t.Error("different IP should get its own bucket and be allowed")
	}
}

// TestIPLimiter_Refills verifies tokens are replenished over time.
func TestIPLimiter_Refills(t *testing.T) {
	l := newIPLimiter(10, 1) // 10 tokens/sec, burst 1
	if !l.Allow("203.0.113.7") {
		t.Fatal("first request should be allowed")
	}
	if l.Allow("203.0.113.7") {
		t.Fatal("second immediate request should be limited (burst 1)")
	}
	time.Sleep(150 * time.Millisecond) // refill ~1.5 tokens
	if !l.Allow("203.0.113.7") {
		t.Error("expected request after refill to be allowed")
	}
}

// TestDaemon_RateLimitChat verifies per-IP chat rate limiting returns 429
// beyond the burst window (H9).
func TestDaemon_RateLimitChat(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, daemonTestSessionFactory(nil))
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	// Exhaust the chat limiter's burst for the loopback client IP. The next
	// /v1/chat must be throttled with 429.
	ip := "127.0.0.1"
	for srv.chatLimiter.Allow(ip) {
	}
	resp, _ := postDaemonChat(t, addr, ChatRequest{Prompt: "hi"}, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after chat burst exhausted, got %d", resp.StatusCode)
	}
}

// TestDaemon_CancelEndpoint verifies POST /v1/cancel cancels an in-flight
// generation and returns 404 when none is active (H10).
func TestDaemon_CancelEndpoint(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, daemonTestSessionFactory(nil))
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	srv.registerCancel("cancel-sess-1", cancel)
	defer cancel()

	resp := postCancel(t, addr, `{"session_id":"cancel-sess-1"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /v1/cancel, got %d", resp.StatusCode)
	}
	var body map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		resp.Body.Close()
		t.Fatalf("decode cancel response: %v", err)
	}
	resp.Body.Close()
	if !body["cancelled"] {
		t.Error("expected cancelled=true")
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Error("generation context was not cancelled")
	}

	// After the generation ends, its entry is unregistered → 404.
	srv.unregisterCancel("cancel-sess-1", cancel)
	resp = postCancel(t, addr, `{"session_id":"cancel-sess-1"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after generation ended, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Invalid session id → 400.
	resp = postCancel(t, addr, `{"session_id":".."} `)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid session id, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestDaemon_GlobalConcurrencyCap verifies handleChat returns 503 when the
// global concurrency semaphore is saturated (H9).
func TestDaemon_GlobalConcurrencyCap(t *testing.T) {
	t.Setenv("HAWK_DAEMON_MAX_CONCURRENT", "1")
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, daemonTestSessionFactory(nil))
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	// Occupy the single slot.
	srv.concurrencySem <- struct{}{}
	defer func() { <-srv.concurrencySem }()

	resp, _ := postDaemonChat(t, addr, ChatRequest{Prompt: "hi"}, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when concurrency slot is full, got %d", resp.StatusCode)
	}
}

func postCancel(t *testing.T, addr, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://"+addr+"/v1/cancel", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new cancel request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/cancel: %v", err)
	}
	return resp
}
