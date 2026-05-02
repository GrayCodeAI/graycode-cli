// Package ratelimit provides token bucket rate limiting.
package ratelimit

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrRateLimited is returned when the rate limit is exceeded.
var ErrRateLimited = errors.New("rate limited")

// Limiter is a token bucket rate limiter.
type Limiter struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	fillRate   float64 // tokens per second
	lastFill   time.Time
}

// Config configures a rate limiter.
type Config struct {
	Rate   int           // tokens per second
	Burst  int           // max burst size
}

// New creates a new rate limiter.
func New(cfg Config) *Limiter {
	return &Limiter{
		tokens:   float64(cfg.Burst),
		capacity: float64(cfg.Burst),
		fillRate: float64(cfg.Rate),
		lastFill: time.Now(),
	}
}

// Allow checks if a single request should be allowed.
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()

	if l.tokens >= 1 {
		l.tokens--
		return true
	}
	return false
}

// Wait blocks until a token is available or the context is cancelled.
func (l *Limiter) Wait(ctx context.Context) error {
	for {
		l.mu.Lock()
		l.refill()

		if l.tokens >= 1 {
			l.tokens--
			l.mu.Unlock()
			return nil
		}

		// Calculate wait time for next token
		needed := 1 - l.tokens
		waitTime := time.Duration(needed / l.fillRate * float64(time.Second))
		if waitTime < 1*time.Millisecond {
			waitTime = 1 * time.Millisecond
		}
		l.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Try again
		}
	}
}

// TryAcquire attempts to acquire n tokens.
func (l *Limiter) TryAcquire(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()

	if l.tokens >= float64(n) {
		l.tokens -= float64(n)
		return true
	}
	return false
}

// Tokens returns the current number of available tokens.
func (l *Limiter) Tokens() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refill()
	return l.tokens
}

func (l *Limiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.lastFill).Seconds()
	l.lastFill = now

	l.tokens += elapsed * l.fillRate
	if l.tokens > l.capacity {
		l.tokens = l.capacity
	}
}

// Manager manages multiple named rate limiters.
type Manager struct {
	mu        sync.RWMutex
	limiters  map[string]*Limiter
}

// NewManager creates a new rate limiter manager.
func NewManager() *Manager {
	return &Manager{
		limiters: make(map[string]*Limiter),
	}
}

// Get returns a limiter by name, creating it if needed.
func (m *Manager) Get(name string, cfg Config) *Limiter {
	m.mu.RLock()
	l, ok := m.limiters[name]
	m.mu.RUnlock()
	if ok {
		return l
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if l, ok := m.limiters[name]; ok {
		return l
	}
	l = New(cfg)
	m.limiters[name] = l
	return l
}

// PerSecond creates a rate limiter with the given requests per second.
func PerSecond(rps int) *Limiter {
	return New(Config{Rate: rps, Burst: rps})
}

// PerMinute creates a rate limiter with the given requests per minute.
func PerMinute(rpm int) *Limiter {
	rate := rpm / 60
	if rate < 1 {
		rate = 1
	}
	burst := rpm / 10
	if burst < 1 {
		burst = 1
	}
	return New(Config{Rate: rate, Burst: burst})
}
