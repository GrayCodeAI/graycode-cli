// Package circuitbreaker provides a small, self-contained circuit breaker used
// to protect repeated operations from retry storms. It is the Go port of
// OpenClaude's auto-compact circuit breaker: after a threshold of consecutive
// failures, the breaker "opens" and skips the operation for a cooldown window,
// then re-arms in a half-open state so a single success closes it and a single
// failure re-opens it. Graycode uses it to stop runaway auto-compaction (an
// irrecoverable prompt_too_long would otherwise retry thousands of times).
package circuitbreaker

import (
	"sync"
	"time"
)

// State describes the breaker's lifecycle.
type State int

const (
	// Closed allows calls; a success keeps it closed.
	Closed State = iota
	// Open skips calls until the cooldown elapses.
	Open
	// HalfOpen allows a probe call after the cooldown; one failure re-opens.
	HalfOpen
)

// Breaker is a concurrency-safe circuit breaker. The zero value is not usable;
// use New.
type Breaker struct {
	maxConsecutiveFailures int
	cooldown               time.Duration

	mu                  sync.Mutex
	consecutiveFailures int
	nextRetryAt         time.Time
	lastFailureAt       time.Time
}

// New returns a breaker that opens after maxConsecutiveFailures (must be >= 1)
// consecutive failures, skipping calls for cooldown.
func New(maxConsecutiveFailures int, cooldown time.Duration) *Breaker {
	if maxConsecutiveFailures < 1 {
		maxConsecutiveFailures = 1
	}
	if cooldown < 0 {
		cooldown = 0
	}
	return &Breaker{
		maxConsecutiveFailures: maxConsecutiveFailures,
		cooldown:               cooldown,
	}
}

// Decision is the outcome of ShouldAllow.
type Decision struct {
	// Allow reports whether the caller may proceed with the operation.
	Allow bool
	// WasHalfOpen is true when the cooldown had elapsed and a probe was admitted
	// (so the caller knows one more failure will re-open the breaker).
	WasHalfOpen bool
	// EffectiveConsecutiveFailures is the count to use for a subsequent failure
	// record (the half-open probe is seeded one below the threshold).
	EffectiveConsecutiveFailures int
}

// ShouldAllow consults the breaker at time now. When the breaker is open and
// the cooldown has not elapsed, it returns Allow=false (skip). When the
// cooldown has elapsed it admits a probe with WasHalfOpen=true, seeding the
// failure count one below the threshold so a single failure re-opens.
func (b *Breaker) ShouldAllow(now time.Time) Decision {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.consecutiveFailures < b.maxConsecutiveFailures {
		return Decision{Allow: true, EffectiveConsecutiveFailures: b.consecutiveFailures}
	}
	if b.nextRetryAt.IsZero() && !b.lastFailureAt.IsZero() {
		b.nextRetryAt = b.lastFailureAt.Add(b.cooldown)
	}
	if !b.nextRetryAt.IsZero() && now.Before(b.nextRetryAt) {
		return Decision{Allow: false, EffectiveConsecutiveFailures: b.consecutiveFailures}
	}
	b.consecutiveFailures = b.maxConsecutiveFailures - 1
	return Decision{Allow: true, WasHalfOpen: true, EffectiveConsecutiveFailures: b.consecutiveFailures}
}

// RecordSuccess resets the consecutive-failure count, closing the breaker.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consecutiveFailures = 0
	b.nextRetryAt = time.Time{}
	b.lastFailureAt = time.Time{}
}

// RecordFailure increments the consecutive-failure count and records the time
// the cooldown window should start from.
func (b *Breaker) RecordFailure(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consecutiveFailures++
	b.lastFailureAt = now
	b.nextRetryAt = now.Add(b.cooldown)
}

// State reports the current logical state at time now.
func (b *Breaker) State(now time.Time) State {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.consecutiveFailures < b.maxConsecutiveFailures {
		return Closed
	}
	if !b.nextRetryAt.IsZero() && now.Before(b.nextRetryAt) {
		return Open
	}
	return HalfOpen
}

// ConsecutiveFailures returns the current consecutive-failure count.
func (b *Breaker) ConsecutiveFailures() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.consecutiveFailures
}
