package engine

import (
	"context"
	"fmt"
	"time"
)

// timeoutDeadlineKey is the context key for storing the deadline used by RemainingTime.
type timeoutDeadlineKey struct{}

// TimeoutConfig controls operation time budgets.
type TimeoutConfig struct {
	Total     time.Duration // total time for the entire operation
	PerTurn   time.Duration // max time per LLM turn (default: 60s)
	PerTool   time.Duration // max time per tool execution (default: 120s)
	Countdown bool          // show remaining time in output
}

// DefaultTimeoutConfig returns a TimeoutConfig with sensible per-turn and per-tool
// defaults. Total is left at zero (no overall deadline) so the caller can set it.
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		PerTurn: 60 * time.Second,
		PerTool: 120 * time.Second,
	}
}

// WithTimeout wraps a context with the total timeout and stores the deadline
// so that RemainingTime can report it.
func WithTimeout(ctx context.Context, cfg TimeoutConfig) (context.Context, context.CancelFunc) {
	if cfg.Total <= 0 {
		return ctx, func() {}
	}
	deadline := time.Now().Add(cfg.Total)
	ctx = context.WithValue(ctx, timeoutDeadlineKey{}, deadline)
	return context.WithDeadline(ctx, deadline)
}

// RemainingTime returns a formatted remaining-time string derived from the
// context deadline set by WithTimeout. If no deadline is set it returns an
// empty string.
func RemainingTime(ctx context.Context) string {
	dl, ok := ctx.Value(timeoutDeadlineKey{}).(time.Time)
	if !ok {
		// Fall back to the standard context deadline.
		dl, ok = ctx.Deadline()
		if !ok {
			return ""
		}
	}
	remaining := time.Until(dl)
	if remaining <= 0 {
		return "time expired"
	}
	formatted := remaining.Truncate(time.Second).String()
	if remaining < time.Minute {
		return fmt.Sprintf("⚠ %s remaining", formatted)
	}
	return fmt.Sprintf("%s remaining", formatted)
}

// TimeoutMessage returns a user-friendly message when the time budget is
// exhausted.
func TimeoutMessage(elapsed time.Duration) string {
	return fmt.Sprintf("Time budget exhausted (%s). Partial progress saved.", elapsed.Truncate(time.Second))
}
