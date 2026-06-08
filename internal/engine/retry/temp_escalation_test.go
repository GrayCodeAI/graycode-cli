package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
)

// TestTempEscalation_SucceedsOnFirstAttempt verifies that a non-empty result at
// temp=0 is returned immediately without further calls.
func TestTempEscalation_SucceedsOnFirstAttempt(t *testing.T) {
	calls := 0
	result, err := TempEscalationRetry(context.Background(), 3, func(_ context.Context, temp float64) (string, error) {
		calls++
		if temp != 0.0 {
			t.Errorf("first call should be at temp 0.0, got %v", temp)
		}
		return "some output", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "some output" {
		t.Errorf("result = %q, want %q", result, "some output")
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

// TestTempEscalation_RetriesOnEmptyResult verifies escalation through all retries
// and returns the first non-empty result.
func TestTempEscalation_RetriesOnEmptyResult(t *testing.T) {
	calls := 0
	var seenTemps []float64

	result, err := TempEscalationRetry(context.Background(), 3, func(_ context.Context, temp float64) (string, error) {
		calls++
		seenTemps = append(seenTemps, temp)
		if calls < 3 {
			return "   ", nil // whitespace-only is treated as empty
		}
		return "fixed output", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "fixed output" {
		t.Errorf("result = %q, want %q", result, "fixed output")
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}

	// Temperature must start at 0 and increase.
	if seenTemps[0] != 0.0 {
		t.Errorf("first call should be temp 0.0, got %v", seenTemps[0])
	}
	for i := 1; i < len(seenTemps); i++ {
		if seenTemps[i] <= seenTemps[i-1] {
			t.Errorf("temperature must increase monotonically: %v", seenTemps)
		}
	}
}

// TestTempEscalation_AllEmpty returns empty string with nil error when every
// attempt returns whitespace and no errors occurred.
func TestTempEscalation_AllEmpty(t *testing.T) {
	calls := 0
	result, err := TempEscalationRetry(context.Background(), 2, func(_ context.Context, _ float64) (string, error) {
		calls++
		return "", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
	if calls != 3 { // 1 initial + 2 retries
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

// TestTempEscalation_ErrorsRetried verifies that errors cause escalation.
func TestTempEscalation_ErrorsRetried(t *testing.T) {
	calls := 0
	sentinel := errors.New("transient error")

	result, err := TempEscalationRetry(context.Background(), 3, func(_ context.Context, _ float64) (string, error) {
		calls++
		if calls < 4 {
			return "", sentinel
		}
		return "recovered", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "recovered" {
		t.Errorf("result = %q, want %q", result, "recovered")
	}
}

// TestTempEscalation_AllErrors returns the last error when every attempt fails.
func TestTempEscalation_AllErrors(t *testing.T) {
	sentinel := errors.New("persistent failure")
	result, err := TempEscalationRetry(context.Background(), 2, func(_ context.Context, _ float64) (string, error) {
		return "", sentinel
	})
	if err == nil {
		t.Fatal("expected error when all attempts fail, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
}

// TestTempEscalation_ContextCancellation returns ctx.Err() when context is
// cancelled before an attempt.
func TestTempEscalation_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	calls := 0
	result, err := TempEscalationRetry(ctx, 5, func(_ context.Context, _ float64) (string, error) {
		calls++
		return "should not reach", nil
	})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
	// The already-cancelled ctx is caught before the first attempt.
	if calls != 0 {
		t.Errorf("expected 0 calls with pre-cancelled context, got %d", calls)
	}
}

// TestTempEscalation_ZeroRetries calls exactly once at temp 0.0.
func TestTempEscalation_ZeroRetries(t *testing.T) {
	calls := 0
	result, err := TempEscalationRetry(context.Background(), 0, func(_ context.Context, temp float64) (string, error) {
		calls++
		if temp != 0.0 {
			t.Errorf("zero-retry mode should call at temp 0.0, got %v", temp)
		}
		return "single attempt", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 call, got %d", calls)
	}
	if result != "single attempt" {
		t.Errorf("result = %q", result)
	}
}

// TestTempEscalation_TemperatureSchedule verifies the linear schedule for
// maxRetries=4 (totalAttempts=5): temps should be 0.0, 0.25, 0.5, 0.75, 1.0.
func TestTempEscalation_TemperatureSchedule(t *testing.T) {
	wantTemps := []float64{0.0, 0.25, 0.50, 0.75, 1.0}
	var gotTemps []float64

	_, _ = TempEscalationRetry(context.Background(), 4, func(_ context.Context, temp float64) (string, error) {
		gotTemps = append(gotTemps, temp)
		return "", nil // force all attempts
	})

	if len(gotTemps) != len(wantTemps) {
		t.Fatalf("expected %d temperature readings, got %d", len(wantTemps), len(gotTemps))
	}
	for i, want := range wantTemps {
		if math.Abs(gotTemps[i]-want) > 1e-9 {
			t.Errorf("temp[%d] = %v, want %v", i, gotTemps[i], want)
		}
	}
}

// TestTempEscalation_NegativeMaxRetries behaves like zero retries.
func TestTempEscalation_NegativeMaxRetries(t *testing.T) {
	calls := 0
	_, err := TempEscalationRetry(context.Background(), -5, func(_ context.Context, temp float64) (string, error) {
		calls++
		return fmt.Sprintf("attempt %d temp %.2f", calls, temp), nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("negative maxRetries should produce exactly 1 call, got %d", calls)
	}
}
