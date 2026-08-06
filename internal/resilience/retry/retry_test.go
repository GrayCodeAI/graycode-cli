package retry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		// Typed errors use errors.Is / errors.As
		{"context deadline exceeded", context.DeadlineExceeded, true},
		{"context canceled", context.Canceled, false},
		{"os.ErrDeadlineExceeded", os.ErrDeadlineExceeded, true},
		{"io.EOF", io.EOF, true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"net.OpError timeout", &net.OpError{Err: os.ErrDeadlineExceeded}, true},
		{"net.OpError connection refused", &net.OpError{Err: errors.New("connection refused")}, true},
		{"url.Error timeout", &url.Error{Op: "Get", URL: "http://x", Err: context.DeadlineExceeded}, true},
		{"url.Error connection reset", &url.Error{Op: "Get", URL: "http://x", Err: errors.New("connection reset")}, true},

		// Fallback string-based checks
		{"503 via string", errors.New("503 service unavailable"), true},
		{"rate limit via string", errors.New("rate limit exceeded"), true},
		{"bad request", errors.New("bad request"), false},
		{"invalid api key", errors.New("invalid api key"), false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestIsRetryable_WrappedErrors verifies errors.As / errors.Is traversal works
// through wrapping.
func TestIsRetryable_WrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("api call failed: %w", context.DeadlineExceeded)
	if !IsRetryable(wrapped) {
		t.Error("expected wrapped context.DeadlineExceeded to be retryable")
	}

	wrappedCanceled := fmt.Errorf("api call failed: %w", context.Canceled)
	if IsRetryable(wrappedCanceled) {
		t.Error("expected wrapped context.Canceled to NOT be retryable")
	}

	wrappedTimeout := fmt.Errorf("api call failed: %w", os.ErrDeadlineExceeded)
	if !IsRetryable(wrappedTimeout) {
		t.Error("expected wrapped os.ErrDeadlineExceeded to be retryable")
	}
}

func TestDoSuccess(t *testing.T) {
	cfg := Config{MaxRetries: 2, BaseDelay: 10 * time.Millisecond}
	calls := 0
	err := Do(context.Background(), cfg, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDoRetryThenSuccess(t *testing.T) {
	cfg := Config{MaxRetries: 3, BaseDelay: 10 * time.Millisecond}
	calls := 0
	err := Do(context.Background(), cfg, func() error {
		calls++
		if calls < 3 {
			return context.DeadlineExceeded
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDoMaxRetriesExceeded(t *testing.T) {
	cfg := Config{MaxRetries: 2, BaseDelay: 10 * time.Millisecond}
	err := Do(context.Background(), cfg, func() error {
		return errors.New("temporary error")
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDoNonRetryable(t *testing.T) {
	cfg := Config{MaxRetries: 3, BaseDelay: 10 * time.Millisecond}
	calls := 0
	err := Do(context.Background(), cfg, func() error {
		calls++
		return errors.New("bad request")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call for non-retryable error, got %d", calls)
	}
}

func TestDoContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := Config{MaxRetries: 3, BaseDelay: 10 * time.Millisecond}
	err := Do(ctx, cfg, func() error {
		return errors.New("temporary error")
	})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDoWithResult(t *testing.T) {
	cfg := Config{MaxRetries: 2, BaseDelay: 10 * time.Millisecond}
	result, err := DoWithResult(context.Background(), cfg, func() (string, error) {
		return "success", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "success" {
		t.Fatalf("expected 'success', got %q", result)
	}
}

func TestBackoff(t *testing.T) {
	base := 1 * time.Second
	maxDelay := 30 * time.Second

	tests := []struct {
		attempt int
		minDur  time.Duration
		maxDur  time.Duration
	}{
		{0, 1 * time.Second, 2 * time.Second}, // base*2^0 + [0, base) jitter
		{1, 2 * time.Second, 3 * time.Second}, // base*2^1 + [0, base) jitter
		{2, 4 * time.Second, 5 * time.Second}, // base*2^2 + [0, base) jitter
		{3, 8 * time.Second, 9 * time.Second}, // base*2^3 + [0, base) jitter
		{10, maxDelay, maxDelay},              // capped at max
	}

	for _, tt := range tests {
		result := backoff(tt.attempt, base, maxDelay, 2.0)
		if result < tt.minDur || result > tt.maxDur {
			t.Errorf("backoff(%d) = %v, want between %v and %v", tt.attempt, result, tt.minDur, tt.maxDur)
		}
	}
}
