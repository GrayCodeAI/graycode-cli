// Package retry provides configurable retry logic with exponential backoff.
package retry

import (
	"context"
	"math"
	"strings"
	"time"
)

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
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	retryable := []string{
		"timeout",
		"temporary",
		"connection refused",
		"no such host",
		"reset by peer",
		"broken pipe",
		"too many requests",
		"rate limit",
		"503",
		"502",
		"504",
		"internal server error",
	}
	for _, r := range retryable {
		if strings.Contains(s, r) {
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
		err = fn()
		if err == nil {
			return nil
		}
		if i == cfg.MaxRetries || !retryable(err) {
			return err
		}
		delay := backoff(i, cfg.BaseDelay, cfg.MaxDelay, cfg.Multiplier)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
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
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(delay):
		}
	}
	return result, err
}

func backoff(attempt int, base, max time.Duration, multiplier float64) time.Duration {
	d := float64(base) * math.Pow(multiplier, float64(attempt))
	if d > float64(max) {
		return max
	}
	return time.Duration(d)
}
