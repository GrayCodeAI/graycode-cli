package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{errors.New("connection timeout"), true},
		{errors.New("temporary failure"), true},
		{errors.New("connection refused"), true},
		{errors.New("503 service unavailable"), true},
		{errors.New("rate limit exceeded"), true},
		{errors.New("bad request"), false},
		{errors.New("invalid api key"), false},
		{nil, false},
	}

	for _, tt := range tests {
		result := IsRetryable(tt.err)
		if result != tt.expected {
			t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, result, tt.expected)
		}
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
			return errors.New("temporary error")
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
	tests := []struct {
		attempt    int
		expected   time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{10, 30 * time.Second}, // capped at max
	}

	for _, tt := range tests {
		result := backoff(tt.attempt, 1*time.Second, 30*time.Second, 2.0)
		if result != tt.expected {
			t.Errorf("backoff(%d) = %v, want %v", tt.attempt, result, tt.expected)
		}
	}
}
