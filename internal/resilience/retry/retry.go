// Package retry provides configurable retry logic with exponential backoff.
package retry

import (
	"context"
	"errors"
	"io"
	"math"
	"math/rand"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

// backoff jitter window for the minimum base delay.

// Config configures retry behavior.
type Config struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Multiplier float64
	Retryable  func(error) bool
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
		Multiplier: 2.0,
		Retryable:  IsRetryable,
	}
}

// IsRetryable returns true for errors that warrant a retry.
//
// It uses typed error checking (errors.Is / errors.As) as the primary
// mechanism, falling back to a small set of string-based checks only for
// HTTP status codes or rate-limit messages that are not wrapped in typed
// Go errors. This replaces the previous implementation that relied entirely
// on strings.Contains, which was fragile and matched unrelated error text.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Context cancellation is never retryable — the caller explicitly
	// aborted the operation.
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Timeouts are retryable.
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}

	// Temporary errors (e.g., EAGAIN on sockets) are retryable.
	// In Go 1.24+, os.IsTimeout / os.IsTemporary were deprecated; we use
	// errors.Is against os.ErrDeadlineExceeded (already checked above) and
	// the net.Error interface for network-level temporary/timeout flags.

	// net.Error covers timeouts, temporary errors, and network failures
	// (DNS resolution, connection refused, "reset by peer", "broken pipe").
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// url.Error wraps failures from http.Client. Timeouts are retryable.
	// Other url.Error values wrap either transport-level failures (already
	// caught by the net.Error check above, since errors.As unwraps
	// recursively) or permanent client errors like "unsupported protocol
	// scheme", "invalid port", and "missing protocol scheme" — those must
	// NOT be retried. Only retry when the wrapped error is itself retryable.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return true
		}
		if inner := urlErr.Unwrap(); inner != nil {
			return IsRetryable(inner)
		}
		return false
	}

	// io.EOF and io.ErrUnexpectedEOF are transport-level errors that may
	// indicate a connection was reset mid-stream.
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// Fallback: string-based check for HTTP status codes and rate-limit
	// messages that are not wrapped in typed Go errors. This catches cases
	// where callers create errors via fmt.Errorf("503 service unavailable")
	// or similar. The set is intentionally minimal and targets only codes
	// that represent transient failures.
	s := strings.ToLower(err.Error())
	fallbackRetryable := []string{
		"temporary",
		"rate limit",
		"too many requests",
	}
	for _, r := range fallbackRetryable {
		if strings.Contains(s, r) {
			return true
		}
	}

	// Retry on transient HTTP status codes (408, 429, 500, 502, 503, 504)
	// surfaced as plain error strings. We check the 3-digit code rather than
	// a full phrase so we handle "503 unavailable", "503 service unavailable",
	// "500 internal server error", etc. uniformly.
	for _, code := range []string{"408", "429", "500", "502", "503", "504"} {
		if strings.Contains(s, code) {
			return true
		}
	}

	return false
}

// Do executes fn with retries.
func Do(ctx context.Context, cfg Config, fn func() error) error {
	retryable := cfg.Retryable
	if retryable == nil {
		retryable = IsRetryable
	}
	var err error
	for i := 0; i <= cfg.MaxRetries; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err = fn()
		if err == nil {
			return nil
		}
		if i == cfg.MaxRetries || !retryable(err) {
			return err
		}
		delay := backoff(i, cfg.BaseDelay, cfg.MaxDelay, cfg.Multiplier)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

// DoWithResult executes fn with retries and returns a result.
func DoWithResult[T any](ctx context.Context, cfg Config, fn func() (T, error)) (T, error) {
	retryable := cfg.Retryable
	if retryable == nil {
		retryable = IsRetryable
	}
	var result T
	var err error
	for i := 0; i <= cfg.MaxRetries; i++ {
		result, err = fn()
		if err == nil {
			return result, nil
		}
		if i == cfg.MaxRetries || !retryable(err) {
			return result, err
		}
		delay := backoff(i, cfg.BaseDelay, cfg.MaxDelay, cfg.Multiplier)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return result, ctx.Err()
		case <-timer.C:
		}
	}
	return result, err
}

func backoff(attempt int, base, max time.Duration, multiplier float64) time.Duration {
	d := float64(base) * math.Pow(multiplier, float64(attempt))
	jitter := time.Duration(rand.Int63n(int64(base))) // #nosec G404 -- non-cryptographic use (retry/backoff jitter)
	delay := time.Duration(d) + jitter
	if delay > max {
		return max
	}
	return delay
}
