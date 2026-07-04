package tool

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
)

// TransientError is an error that a tool may return to signal that the
// caller should retry the call. Tools that wrap system-level operations
// (file I/O, network) should wrap their underlying err in this type when
// the underlying cause is transient (e.g. EBUSY, ETXTBSY, network reset).
type TransientError struct {
	Cause error
}

// Error implements the error interface.
func (e *TransientError) Error() string {
	if e.Cause == nil {
		return "transient error"
	}
	return "transient: " + e.Cause.Error()
}

// Unwrap exposes the underlying cause for errors.Is/As.
func (e *TransientError) Unwrap() error { return e.Cause }

// NewTransientError wraps err in a *TransientError if it is not already one.
func NewTransientError(err error) error {
	if err == nil {
		return nil
	}
	var te *TransientError
	if errors.As(err, &te) {
		return err
	}
	return &TransientError{Cause: err}
}

// IsTransientError reports whether err is a *TransientError (recursively).
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}
	var te *TransientError
	return errors.As(err, &te)
}

// RetryPolicy controls how a tool call is retried on transient errors.
type RetryPolicy struct {
	// MaxRetries is the total number of attempts. 0 disables retry.
	MaxRetries int
	// BaseDelay is the initial backoff; doubled each attempt.
	BaseDelay time.Duration
	// MaxDelay caps the backoff.
	MaxDelay time.Duration
	// ShouldRetry lets tools customise the predicate. If nil, every error
	// returned by the tool is retried (when wrapped in *TransientError).
	ShouldRetry func(err error, attempt int) bool
}

// DefaultRetryPolicy is used when a tool doesn't override it. Conservative:
// 2 retries (3 attempts total), 200ms → 800ms backoff.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries: 2,
		BaseDelay:  200 * time.Millisecond,
		MaxDelay:   2 * time.Second,
	}
}

// RetryExecutor wraps a tool's Execute and retries on transient errors.
// ctx cancellation aborts the wait between attempts (so cancel is observed).
func RetryExecutor(ctx context.Context, t Tool, input []byte, policy RetryPolicy) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if policy.MaxRetries <= 0 {
		return t.Execute(ctx, input)
	}
	var lastErr error
	delay := policy.BaseDelay
	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		out, err := t.Execute(ctx, input)
		if err == nil {
			return out, nil
		}
		lastErr = err
		transient := IsTransientError(err)
		retry := transient
		if policy.ShouldRetry != nil {
			retry = retry && policy.ShouldRetry(err, attempt)
		}
		if !retry || attempt == policy.MaxRetries {
			break
		}
		// Wait, respecting ctx cancellation.
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return out, ctx.Err()
		}
		delay *= 2
		if delay > policy.MaxDelay {
			delay = policy.MaxDelay
		}
	}
	return "", lastErr
}

// IsTransientFileErr reports whether err is a transient file-I/O error
// (busy file, text-file busy, resource temporarily unavailable, network reset).
// Tools can use this with RetryPolicy.ShouldRetry to retry specific errors.
func IsTransientFileErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	transientSubstrings := []string{
		"resource temporarily unavailable",
		"text file busy",
		"device or resource busy",
		"busy",
		"connection reset",
		"connection refused",
		"broken pipe",
		"i/o timeout",
		"timeout",
		"temporary failure",
		"eagain",
		"etxtbsy",
		"ebusy",
	}
	for _, sub := range transientSubstrings {
		if strings.Contains(msg, sub) {
			return true
		}
	}
	return false
}
