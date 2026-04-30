package circuit

import (
	"errors"
	"testing"
	"time"
)

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{Closed, "closed"},
		{Open, "open"},
		{HalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if s := tt.state.String(); s != tt.expected {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, s, tt.expected)
		}
	}
}

func TestBreakerClosed(t *testing.T) {
	b := New(Config{MaxFailures: 3, Timeout: 1 * time.Second})

	if b.State() != Closed {
		t.Fatalf("expected closed state, got %s", b.State())
	}

	// Successful calls should work
	for i := 0; i < 5; i++ {
		err := b.Call(func() error { return nil })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if b.State() != Closed {
		t.Fatalf("expected closed after successes, got %s", b.State())
	}
}

func TestBreakerOpens(t *testing.T) {
	b := New(Config{MaxFailures: 3, Timeout: 1 * time.Hour})

	// Fail 3 times
	for i := 0; i < 3; i++ {
		err := b.Call(func() error { return errors.New("fail") })
		if err == nil || err == ErrOpen {
			t.Fatalf("expected error, got %v", err)
		}
	}

	if b.State() != Open {
		t.Fatalf("expected open state, got %s", b.State())
	}

	// Next call should fail fast
	err := b.Call(func() error { return nil })
	if err != ErrOpen {
		t.Fatalf("expected ErrOpen, got %v", err)
	}
}

func TestBreakerHalfOpen(t *testing.T) {
	b := New(Config{MaxFailures: 2, Timeout: 50 * time.Millisecond, HalfOpenMaxCalls: 2})

	// Fail to open
	for i := 0; i < 2; i++ {
		b.Call(func() error { return errors.New("fail") })
	}

	if b.State() != Open {
		t.Fatal("expected open")
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// Should be half-open
	if !b.Allow() {
		t.Fatal("expected allow after timeout")
	}
	if b.State() != HalfOpen {
		t.Fatalf("expected half-open, got %s", b.State())
	}

	// Success should move back to closed
	b.RecordSuccess()
	b.RecordSuccess()

	if b.State() != Closed {
		t.Fatalf("expected closed after successes, got %s", b.State())
	}
}

func TestBreakerHalfOpenFailure(t *testing.T) {
	b := New(Config{MaxFailures: 2, Timeout: 50 * time.Millisecond})

	// Open the circuit
	for i := 0; i < 2; i++ {
		b.Call(func() error { return errors.New("fail") })
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// Should be half-open
	if !b.Allow() {
		t.Fatal("expected allow after timeout")
	}

	// Failure in half-open should go back to open
	b.RecordFailure()

	if b.State() != Open {
		t.Fatalf("expected open after half-open failure, got %s", b.State())
	}
}

func TestCallWithResult(t *testing.T) {
	b := New(Config{MaxFailures: 3})

	result, err := CallWithResult(b, func() (string, error) {
		return "hello", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "hello" {
		t.Fatalf("expected 'hello', got %q", result)
	}

	// Open circuit
	b2 := New(Config{MaxFailures: 3, Timeout: 1 * time.Hour})
	for i := 0; i < 3; i++ {
		CallWithResult(b2, func() (string, error) { return "", errors.New("fail") })
	}

	_, err = CallWithResult(b2, func() (string, error) {
		return "hello", nil
	})
	if err != ErrOpen {
		t.Fatalf("expected ErrOpen, got %v", err)
	}
}

func TestStats(t *testing.T) {
	b := New(Config{MaxFailures: 3})
	stats := b.Stats()
	if stats.State != "closed" {
		t.Fatalf("expected state 'closed', got %q", stats.State)
	}
	if stats.Failures != 0 {
		t.Fatalf("expected 0 failures, got %d", stats.Failures)
	}
}

func TestManager(t *testing.T) {
	m := NewManager(DefaultConfig())

	b1 := m.Get("api1")
	b2 := m.Get("api1")
	if b1 != b2 {
		t.Fatal("expected same breaker instance")
	}

	b3 := m.Get("api2")
	if b1 == b3 {
		t.Fatal("expected different breaker instances")
	}

	stats := m.List()
	if len(stats) != 2 {
		t.Fatalf("expected 2 breakers, got %d", len(stats))
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxFailures != 5 {
		t.Fatalf("expected max failures 5, got %d", cfg.MaxFailures)
	}
	if cfg.Timeout != 30*time.Second {
		t.Fatalf("expected timeout 30s, got %v", cfg.Timeout)
	}
}
