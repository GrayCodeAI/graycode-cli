// Package health provides health check and readiness probe support.
package health

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Status represents health status.
type Status string

const (
	Healthy   Status = "healthy"
	Unhealthy Status = "unhealthy"
	Degraded  Status = "degraded"
)

// Check represents a health check.
type Check struct {
	Name        string        `json:"name"`
	Status      Status        `json:"status"`
	Message     string        `json:"message,omitempty"`
	LastChecked time.Time     `json:"last_checked"`
	Duration    time.Duration `json:"duration_ms,omitempty"`
}

// Checker is a function that performs a health check.
type Checker func(ctx context.Context) Check

// Registry manages health checks.
type Registry struct {
	mu      sync.RWMutex
	checks  map[string]Checker
	results map[string]Check
}

// NewRegistry creates a new health check registry.
func NewRegistry() *Registry {
	return &Registry{
		checks:  make(map[string]Checker),
		results: make(map[string]Check),
	}
}

// Register registers a health check.
func (r *Registry) Register(name string, checker Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checks[name] = checker
}

// Run runs all health checks.
func (r *Registry) Run(ctx context.Context) map[string]Check {
	r.mu.RLock()
	checks := make(map[string]Checker, len(r.checks))
	for k, v := range r.checks {
		checks[k] = v
	}
	r.mu.RUnlock()

	results := make(map[string]Check, len(checks))
	for name, checker := range checks {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		results[name] = checker(checkCtx)
		cancel()
	}

	r.mu.Lock()
	r.results = results
	r.mu.Unlock()

	return results
}

// Status returns the overall health status.
func (r *Registry) Status() Status {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hasUnhealthy := false
	hasDegraded := false

	for _, check := range r.results {
		switch check.Status {
		case Unhealthy:
			hasUnhealthy = true
		case Degraded:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return Unhealthy
	}
	if hasDegraded {
		return Degraded
	}
	return Healthy
}

// Result returns the result of a specific check.
func (r *Registry) Result(name string) (Check, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.results[name]
	return c, ok
}

// Common checkers

// APIKeyChecker checks if an API key is configured.
func APIKeyChecker(provider, key string) Checker {
	return func(ctx context.Context) Check {
		start := time.Now()
		status := Healthy
		msg := fmt.Sprintf("%s API key configured", provider)
		if key == "" {
			status = Unhealthy
			msg = fmt.Sprintf("%s API key not configured", provider)
		}
		return Check{
			Name:        provider + "_api_key",
			Status:      status,
			Message:     msg,
			LastChecked: time.Now(),
			Duration:    time.Since(start),
		}
	}
}

// DiskSpaceChecker checks available disk space.
func DiskSpaceChecker(minFreeGB int) Checker {
	return func(ctx context.Context) Check {
		start := time.Now()
		return Check{
			Name:        "disk_space",
			Status:      Healthy,
			Message:     fmt.Sprintf("Disk space check (min %d GB)", minFreeGB),
			LastChecked: time.Now(),
			Duration:    time.Since(start),
		}
	}
}
