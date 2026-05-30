// Package mission provides multi-feature parallel execution for hawk.
// This file adds a watchdog for stall detection in worker goroutines.
package mission

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// WatchdogConfig configures the stall-detection watchdog.
type WatchdogConfig struct {
	StallTimeout  time.Duration // default 90s — no output for this long = stall
	CheckInterval time.Duration // default 10s — how often to check
}

// Watchdog monitors worker progress and triggers fallback on stalls.
type Watchdog struct {
	config     WatchdogConfig
	mu         sync.Mutex
	lastActive map[string]time.Time // feature ID -> last activity timestamp
	onStall    func(featureID string)
}

// NewWatchdog creates a new watchdog with the given config and stall callback.
func NewWatchdog(cfg WatchdogConfig, onStall func(string)) *Watchdog {
	if cfg.StallTimeout == 0 {
		cfg.StallTimeout = 90 * time.Second
	}
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 10 * time.Second
	}
	return &Watchdog{
		config:     cfg,
		lastActive: make(map[string]time.Time),
		onStall:    onStall,
	}
}

// Register marks a feature as active (call when worker starts).
func (w *Watchdog) Register(featureID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastActive[featureID] = time.Now()
}

// Touch updates the last-activity timestamp for a feature.
// Call this when a worker produces output.
func (w *Watchdog) Touch(featureID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastActive[featureID] = time.Now()
}

// Unregister removes a feature from monitoring (call on completion).
func (w *Watchdog) Unregister(featureID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.lastActive, featureID)
}

// Run starts the watchdog loop. Blocks until ctx is cancelled.
func (w *Watchdog) Run(ctx context.Context) {
	ticker := time.NewTicker(w.config.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.check()
		}
	}
}

// check iterates all registered features and fires onStall for stale ones.
func (w *Watchdog) check() {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	for id, lastActive := range w.lastActive {
		if now.Sub(lastActive) > w.config.StallTimeout {
			fmt.Fprintf(os.Stderr, "hawk: warning: feature %s stalled (no output for %v)\n", id, w.config.StallTimeout)
			if w.onStall != nil {
				w.onStall(id)
			}
		}
	}
}

// ActiveCount returns the number of currently monitored features.
func (w *Watchdog) ActiveCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.lastActive)
}
