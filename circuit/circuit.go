// Package circuit provides a circuit breaker pattern for resilience.
// When failures exceed a threshold, the circuit opens and fails fast.
// After a timeout, the circuit transitions to half-open to test recovery.
package circuit

import (
	"errors"
	"sync"
	"time"
)

// State represents the circuit breaker state.
type State int

const (
	Closed State = iota   // Normal operation
	Open                  // Failing fast
	HalfOpen              // Testing recovery
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Breaker is a circuit breaker.
type Breaker struct {
	mu                sync.RWMutex
	state             State
	failures          int
	lastFailureTime   time.Time
	successCount      int

	maxFailures       int
	timeout           time.Duration
	halfOpenMaxCalls  int
}

// ErrOpen is returned when the circuit is open.
var ErrOpen = errors.New("circuit breaker is open")

// Config configures a circuit breaker.
type Config struct {
	MaxFailures      int
	Timeout          time.Duration
	HalfOpenMaxCalls int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxFailures:      5,
		Timeout:          30 * time.Second,
		HalfOpenMaxCalls: 3,
	}
}

// New creates a new circuit breaker.
func New(cfg Config) *Breaker {
	return &Breaker{
		maxFailures:      cfg.MaxFailures,
		timeout:          cfg.Timeout,
		halfOpenMaxCalls: cfg.HalfOpenMaxCalls,
	}
}

// State returns the current state.
func (b *Breaker) State() State {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// Allow checks if a call should be allowed.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == Open {
		if time.Since(b.lastFailureTime) > b.timeout {
			b.state = HalfOpen
			b.successCount = 0
			return true
		}
		return false
	}

	if b.state == HalfOpen && b.successCount >= b.halfOpenMaxCalls {
		b.state = Closed
		b.failures = 0
		b.successCount = 0
	}

	return true
}

// RecordSuccess records a successful call.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == HalfOpen {
		b.successCount++
		if b.successCount >= b.halfOpenMaxCalls {
			b.state = Closed
			b.failures = 0
			b.successCount = 0
		}
		return
	}

	if b.state == Closed {
		b.failures = 0
	}
}

// RecordFailure records a failed call.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures++
	b.lastFailureTime = time.Now()

	if b.state == HalfOpen {
		b.state = Open
		return
	}

	if b.failures >= b.maxFailures {
		b.state = Open
	}
}

// Call executes fn if the circuit allows it.
func (b *Breaker) Call(fn func() error) error {
	if !b.Allow() {
		return ErrOpen
	}

	err := fn()
	if err != nil {
		b.RecordFailure()
		return err
	}

	b.RecordSuccess()
	return nil
}

// CallWithResult executes fn if the circuit allows it.
func CallWithResult[T any](b *Breaker, fn func() (T, error)) (T, error) {
	var zero T
	if !b.Allow() {
		return zero, ErrOpen
	}

	result, err := fn()
	if err != nil {
		b.RecordFailure()
		return zero, err
	}

	b.RecordSuccess()
	return result, nil
}

// Stats returns breaker statistics.
type Stats struct {
	State       string    `json:"state"`
	Failures    int       `json:"failures"`
	LastFailure time.Time `json:"last_failure,omitempty"`
}

// Stats returns current statistics.
func (b *Breaker) Stats() Stats {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return Stats{
		State:       b.state.String(),
		Failures:    b.failures,
		LastFailure: b.lastFailureTime,
	}
}

// Manager manages multiple named circuit breakers.
type Manager struct {
	mu        sync.RWMutex
	breakers  map[string]*Breaker
	config    Config
}

// NewManager creates a new circuit breaker manager.
func NewManager(cfg Config) *Manager {
	return &Manager{
		breakers: make(map[string]*Breaker),
		config:   cfg,
	}
}

// Get returns a breaker by name, creating it if needed.
func (m *Manager) Get(name string) *Breaker {
	m.mu.RLock()
	b, ok := m.breakers[name]
	m.mu.RUnlock()
	if ok {
		return b
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Double-check
	if b, ok := m.breakers[name]; ok {
		return b
	}
	b = New(m.config)
	m.breakers[name] = b
	return b
}

// List returns all breaker names and their stats.
func (m *Manager) List() map[string]Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]Stats, len(m.breakers))
	for name, b := range m.breakers {
		out[name] = b.Stats()
	}
	return out
}
