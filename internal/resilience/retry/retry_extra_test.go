package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.BaseDelay != 1*time.Second {
		t.Errorf("BaseDelay = %v, want %v", cfg.BaseDelay, 1*time.Second)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want %v", cfg.MaxDelay, 30*time.Second)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("Multiplier = %f, want 2.0", cfg.Multiplier)
	}
	if cfg.Retryable == nil {
		t.Error("Retryable should not be nil")
	}
}

func TestIsRetryable_NilError(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("IsRetryable(nil) should be false")
	}
}

func TestIsRetryable_Timeout(t *testing.T) {
	if !IsRetryable(errors.New("request timeout")) {
		t.Error("IsRetryable should return true for timeout")
	}
}

func TestIsRetryable_Temporary(t *testing.T) {
	if !IsRetryable(errors.New("temporary failure")) {
		t.Error("IsRetryable should return true for temporary")
	}
}

func TestIsRetryable_ConnectionRefused(t *testing.T) {
	if !IsRetryable(errors.New("connection refused")) {
		t.Error("IsRetryable should return true for connection refused")
	}
}

func TestIsRetryable_DNSFailure(t *testing.T) {
	if !IsRetryable(errors.New("no such host")) {
		t.Error("IsRetryable should return true for no such host")
	}
}

func TestIsRetryable_503(t *testing.T) {
	if !IsRetryable(errors.New("503 service unavailable")) {
		t.Error("IsRetryable should return true for 503")
	}
}

func TestIsRetryable_502(t *testing.T) {
	if !IsRetryable(errors.New("502 bad gateway")) {
		t.Error("IsRetryable should return true for 502")
	}
}

func TestIsRetryable_504(t *testing.T) {
	if !IsRetryable(errors.New("504 gateway timeout")) {
		t.Error("IsRetryable should return true for 504")
	}
}

func TestIsRetryable_RateLimit(t *testing.T) {
	if !IsRetryable(errors.New("rate limit exceeded")) {
		t.Error("IsRetryable should return true for rate limit")
	}
}

func TestIsRetryable_TooManyRequests(t *testing.T) {
	if !IsRetryable(errors.New("429 too many requests")) {
		t.Error("IsRetryable should return true for too many requests")
	}
}

func TestIsRetryable_NonRetryable(t *testing.T) {
	if IsRetryable(errors.New("not found")) {
		t.Error("IsRetryable should return false for non-retryable error")
	}
}

func TestDo_SuccessOnFirstTry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.MaxDelay = 10 * time.Millisecond

	callCount := 0
	err := Do(context.Background(), cfg, func() error {
		callCount++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestDo_RetryThenSuccess(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.MaxDelay = 10 * time.Millisecond

	callCount := 0
	err := Do(context.Background(), cfg, func() error {
		callCount++
		if callCount < 3 {
			return errors.New("timeout")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestDo_MaxRetriesExceeded(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxRetries = 2
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.MaxDelay = 10 * time.Millisecond

	callCount := 0
	err := Do(context.Background(), cfg, func() error {
		callCount++
		return errors.New("timeout")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if callCount != 3 { // 1 initial + 2 retries
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestDo_NonRetryableError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.MaxDelay = 10 * time.Millisecond

	callCount := 0
	err := Do(context.Background(), cfg, func() error {
		callCount++
		return errors.New("not found")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (non-retryable)", callCount)
	}
}

func TestDo_ContextCanceled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 100 * time.Millisecond
	cfg.MaxDelay = 1 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Do(ctx, cfg, func() error {
		return errors.New("timeout")
	})
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestDo_ContextCanceledDuringRetry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 100 * time.Millisecond
	cfg.MaxDelay = 1 * time.Second

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := Do(ctx, cfg, func() error {
		callCount++
		return errors.New("timeout")
	})
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestDo_CustomRetryable(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.MaxDelay = 10 * time.Millisecond
	cfg.Retryable = func(err error) bool {
		return err.Error() == "retry me"
	}

	callCount := 0
	err := Do(context.Background(), cfg, func() error {
		callCount++
		return errors.New("retry me")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if callCount != cfg.MaxRetries+1 {
		t.Errorf("callCount = %d, want %d", callCount, cfg.MaxRetries+1)
	}
}

// --- DoWithResult tests ---

func TestDoWithResult_SuccessOnFirstTry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.MaxDelay = 10 * time.Millisecond

	result, err := DoWithResult(context.Background(), cfg, func() (string, error) {
		return "success", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("result = %q, want %q", result, "success")
	}
}

func TestDoWithResult_RetryThenSuccess(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.MaxDelay = 10 * time.Millisecond

	callCount := 0
	result, err := DoWithResult(context.Background(), cfg, func() (int, error) {
		callCount++
		if callCount < 2 {
			return 0, errors.New("timeout")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("result = %d, want 42", result)
	}
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
}

func TestDoWithResult_MaxRetriesExceeded(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxRetries = 2
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.MaxDelay = 10 * time.Millisecond

	callCount := 0
	result, err := DoWithResult(context.Background(), cfg, func() (int, error) {
		callCount++
		return 0, errors.New("timeout")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if result != 0 {
		t.Errorf("result = %d, want 0", result)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestDoWithResult_NonRetryableError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.MaxDelay = 10 * time.Millisecond

	callCount := 0
	_, err := DoWithResult(context.Background(), cfg, func() (int, error) {
		callCount++
		return 0, errors.New("not found")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestDoWithResult_ContextCanceled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 100 * time.Millisecond
	cfg.MaxDelay = 1 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := DoWithResult(ctx, cfg, func() (int, error) {
		return 0, errors.New("timeout")
	})
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestDoWithResult_ContextCanceledDuringRetry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 100 * time.Millisecond
	cfg.MaxDelay = 1 * time.Second

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := DoWithResult(ctx, cfg, func() (int, error) {
		return 0, errors.New("timeout")
	})
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestBackoff_CappedAtMax(t *testing.T) {
	// With high attempt, should be capped at max
	delay := backoff(10, 1*time.Second, 5*time.Second, 2.0)
	if delay > 5*time.Second {
		t.Errorf("delay = %v, should be capped at 5s", delay)
	}
}
