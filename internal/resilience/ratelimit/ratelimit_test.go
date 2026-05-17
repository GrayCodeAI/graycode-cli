package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestAllow(t *testing.T) {
	l := New(Config{Rate: 10, Burst: 5})

	// Should allow burst
	for i := 0; i < 5; i++ {
		if !l.Allow() {
			t.Fatalf("expected allow at iteration %d", i)
		}
	}

	// Should deny after burst
	if l.Allow() {
		t.Fatal("expected deny after burst")
	}
}

func TestRefill(t *testing.T) {
	l := New(Config{Rate: 10, Burst: 2})

	// Use all tokens
	l.Allow()
	l.Allow()

	if l.Allow() {
		t.Fatal("expected deny")
	}

	// Wait for refill
	time.Sleep(200 * time.Millisecond)

	if !l.Allow() {
		t.Fatal("expected allow after refill")
	}
}

func TestWait(t *testing.T) {
	l := New(Config{Rate: 100, Burst: 1})

	// Use the only token
	l.Allow()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Should wait and eventually succeed
	start := time.Now()
	err := l.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed < 5*time.Millisecond {
		t.Fatal("expected some wait time")
	}
}

func TestWaitContextCancel(t *testing.T) {
	l := New(Config{Rate: 1, Burst: 1})
	l.Allow() // Use token

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := l.Wait(ctx)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestTryAcquire(t *testing.T) {
	l := New(Config{Rate: 10, Burst: 5})

	if !l.TryAcquire(3) {
		t.Fatal("expected acquire 3")
	}

	if !l.TryAcquire(2) {
		t.Fatal("expected acquire 2")
	}

	if l.TryAcquire(1) {
		t.Fatal("expected deny")
	}
}

func TestTokens(t *testing.T) {
	l := New(Config{Rate: 10, Burst: 5})

	tokens := l.Tokens()
	if tokens != 5 {
		t.Fatalf("expected 5 tokens, got %f", tokens)
	}

	l.Allow()
	tokens = l.Tokens()
	if tokens < 3.9 || tokens > 4.1 {
		t.Fatalf("expected ~4 tokens, got %f", tokens)
	}
}

func TestManager(t *testing.T) {
	m := NewManager()

	l1 := m.Get("api1", Config{Rate: 10, Burst: 5})
	l2 := m.Get("api1", Config{Rate: 100, Burst: 50}) // Should be ignored, already exists

	if l1 != l2 {
		t.Fatal("expected same limiter")
	}

	l3 := m.Get("api2", Config{Rate: 10, Burst: 5})
	if l1 == l3 {
		t.Fatal("expected different limiters")
	}
}

func TestPerSecond(t *testing.T) {
	l := PerSecond(5)
	if !l.Allow() {
		t.Fatal("expected allow")
	}
}

func TestPerMinute(t *testing.T) {
	l := PerMinute(60)
	if !l.Allow() {
		t.Fatal("expected allow")
	}
}
