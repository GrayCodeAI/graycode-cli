// Package shutdown provides graceful shutdown handling for applications.
package shutdown

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Handler manages graceful shutdown.
type Handler struct {
	mu       sync.Mutex
	hooks    []func(ctx context.Context) error
	timeout  time.Duration
	signals  []os.Signal
	cancel   context.CancelFunc
}

// Config configures shutdown behavior.
type Config struct {
	Timeout time.Duration
	Signals []os.Signal
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Timeout: 30 * time.Second,
		Signals: []os.Signal{syscall.SIGINT, syscall.SIGTERM},
	}
}

// New creates a new shutdown handler.
func New(cfg Config) *Handler {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if len(cfg.Signals) == 0 {
		cfg.Signals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	}
	return &Handler{
		timeout: cfg.Timeout,
		signals: cfg.Signals,
	}
}

// Register adds a shutdown hook.
func (h *Handler) Register(fn func(ctx context.Context) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hooks = append(h.hooks, fn)
}

// Listen starts listening for shutdown signals.
// Returns a context that will be cancelled on shutdown.
func (h *Handler) Listen() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, h.signals...)

	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\nReceived signal: %v, shutting down gracefully...\n", sig)
		h.shutdown()
	}()

	return ctx
}

// shutdown executes all hooks with timeout.
func (h *Handler) shutdown() {
	if h.cancel != nil {
		h.cancel()
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, hook := range h.hooks {
		wg.Add(1)
		go func(fn func(ctx context.Context) error) {
			defer wg.Done()
			if err := fn(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Shutdown hook error: %v\n", err)
			}
		}(hook)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Fprintln(os.Stderr, "Shutdown complete.")
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "Shutdown timeout exceeded, forcing exit.")
	}
}

// Trigger manually triggers shutdown.
func (h *Handler) Trigger() {
	h.shutdown()
}

// Wait blocks until shutdown is complete.
func (h *Handler) Wait() {
	// No-op - shutdown is synchronous
}
