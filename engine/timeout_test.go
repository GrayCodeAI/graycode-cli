package engine

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestWithTimeout_SetsDeadline(t *testing.T) {
	ctx := context.Background()
	cfg := TimeoutConfig{Total: 5 * time.Minute}
	ctx2, cancel := WithTimeout(ctx, cfg)
	defer cancel()

	dl, ok := ctx2.Deadline()
	if !ok {
		t.Fatal("expected deadline to be set")
	}
	remaining := time.Until(dl)
	if remaining < 4*time.Minute || remaining > 6*time.Minute {
		t.Fatalf("unexpected remaining time: %v", remaining)
	}
}

func TestWithTimeout_ZeroDuration(t *testing.T) {
	ctx := context.Background()
	cfg := TimeoutConfig{Total: 0}
	ctx2, cancel := WithTimeout(ctx, cfg)
	defer cancel()

	_, ok := ctx2.Deadline()
	if ok {
		t.Fatal("expected no deadline for zero duration")
	}
}

func TestRemainingTime_Warning(t *testing.T) {
	ctx := context.Background()
	cfg := TimeoutConfig{Total: 30 * time.Second}
	ctx2, cancel := WithTimeout(ctx, cfg)
	defer cancel()

	result := RemainingTime(ctx2)
	if !strings.Contains(result, "⚠") {
		t.Errorf("expected warning symbol for <1min, got %q", result)
	}
	if !strings.Contains(result, "remaining") {
		t.Errorf("expected 'remaining' in output, got %q", result)
	}
}

func TestRemainingTime_NoDeadline(t *testing.T) {
	result := RemainingTime(context.Background())
	if result != "" {
		t.Errorf("expected empty string for no deadline, got %q", result)
	}
}

func TestTimeoutMessage(t *testing.T) {
	msg := TimeoutMessage(2 * time.Minute)
	if !strings.Contains(msg, "2m0s") {
		t.Errorf("expected duration in message, got %q", msg)
	}
	if !strings.Contains(msg, "Partial progress saved") {
		t.Errorf("expected 'Partial progress saved' in message, got %q", msg)
	}
}
