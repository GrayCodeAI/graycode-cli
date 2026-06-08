package retry

import (
	"context"
	"strings"
)

// TempEscalationRetry calls attemptFn at temperature 0.0 first.
// If the result is empty or only whitespace, it retries up to maxRetries more
// times, linearly escalating the temperature toward 1.0 on each retry.
//
// This is the Kimi-Dev pattern for stuck code-repair LLM calls: low temperature
// first to get deterministic output; escalate if the model stalls so it can
// explore less obvious repairs.
//
// Temperature schedule (examples):
//
//	maxRetries=1  -> [0.0, 1.0]
//	maxRetries=2  -> [0.0, 0.5, 1.0]
//	maxRetries=3  -> [0.0, 0.33, 0.67, 1.0]
//	maxRetries=4  -> [0.0, 0.25, 0.50, 0.75, 1.0]
//
// If maxRetries <= 0 the function is called exactly once at temperature 0.0.
//
// The function returns the first non-empty result. If every attempt returns an
// empty result, the last error (if any) is returned; otherwise a nil error is
// returned with the empty string (callers may treat empty as a signal to fall
// back to a different strategy).
//
// Context cancellation is respected between attempts: if ctx is already done
// before an attempt, ctx.Err() is returned immediately.
func TempEscalationRetry(
	ctx context.Context,
	maxRetries int,
	attemptFn func(ctx context.Context, temp float64) (string, error),
) (string, error) {
	totalAttempts := maxRetries + 1
	if totalAttempts < 1 {
		totalAttempts = 1
	}

	var lastErr error

	for i := 0; i < totalAttempts; i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		temp := escalationTemp(i, totalAttempts)
		result, err := attemptFn(ctx, temp)
		if err != nil {
			lastErr = err
			continue
		}

		if strings.TrimSpace(result) != "" {
			return result, nil
		}
		// Empty result — escalate temperature on next attempt.
	}

	return "", lastErr
}

// escalationTemp computes the temperature for attempt i out of totalAttempts.
// attempt 0 always returns 0.0; the last attempt always returns 1.0; intermediate
// values are linearly spaced.
func escalationTemp(attempt, totalAttempts int) float64 {
	if totalAttempts <= 1 {
		return 0.0
	}
	return float64(attempt) / float64(totalAttempts-1)
}
