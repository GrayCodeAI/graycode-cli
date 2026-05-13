package routing

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// FallbackProviderHealth tracks detailed health state of a provider in the fallback chain.
type FallbackProviderHealth struct {
	Name                string
	Status              string // "healthy", "degraded", "down"
	LastSuccess         time.Time
	LastFailure         time.Time
	ConsecutiveFailures int
	CooldownUntil       *time.Time
	Latency             time.Duration
}

// ProviderConfig describes a provider entry in the fallback chain.
type ProviderConfig struct {
	Name             string
	Model            string
	Priority         int
	Weight           float64
	MaxRetries       int
	CooldownDuration time.Duration
}

// FallbackResult captures the outcome of a provider selection with fallback.
type FallbackResult struct {
	Provider  string
	Model     string
	Attempts  int
	Fallbacks []string
	Duration  time.Duration
}

// FallbackChain manages multi-provider fallback with health tracking.
type FallbackChain struct {
	Providers      []ProviderConfig
	HealthStatus   map[string]*FallbackProviderHealth
	ActiveProvider string
	mu             sync.RWMutex
}

const (
	statusHealthy  = "healthy"
	statusDegraded = "degraded"
	statusDown     = "down"

	degradedThreshold = 2
	downThreshold     = 5
)

// NewFallbackChain creates a new fallback chain from the given provider configs.
// Providers are sorted by priority (lower number = higher priority).
func NewFallbackChain(providers []ProviderConfig) *FallbackChain {
	sorted := make([]ProviderConfig, len(providers))
	copy(sorted, providers)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	health := make(map[string]*FallbackProviderHealth, len(providers))
	for _, p := range sorted {
		health[p.Name] = &FallbackProviderHealth{
			Name:   p.Name,
			Status: statusHealthy,
		}
	}

	active := ""
	if len(sorted) > 0 {
		active = sorted[0].Name
	}

	return &FallbackChain{
		Providers:      sorted,
		HealthStatus:   health,
		ActiveProvider: active,
	}
}

// SelectProvider picks the highest-priority healthy provider, skipping those in cooldown.
func (fc *FallbackChain) SelectProvider() (*ProviderConfig, error) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	now := time.Now()
	for i := range fc.Providers {
		p := &fc.Providers[i]
		h, ok := fc.HealthStatus[p.Name]
		if !ok {
			continue
		}
		if h.Status == statusDown {
			// Check if cooldown has expired
			if h.CooldownUntil != nil && now.Before(*h.CooldownUntil) {
				continue
			}
		}
		if h.CooldownUntil != nil && now.Before(*h.CooldownUntil) {
			continue
		}
		return p, nil
	}

	return nil, fmt.Errorf("all providers are down or in cooldown")
}

// RecordSuccess records a successful call to a provider and resets failure state.
func (fc *FallbackChain) RecordSuccess(provider string, latency time.Duration) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	h, ok := fc.HealthStatus[provider]
	if !ok {
		return
	}

	h.ConsecutiveFailures = 0
	h.LastSuccess = time.Now()
	h.Latency = latency
	h.Status = statusHealthy
	h.CooldownUntil = nil
	fc.ActiveProvider = provider
}

// RecordFailure records a failed call to a provider and applies cooldown if threshold exceeded.
func (fc *FallbackChain) RecordFailure(provider string, err error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	h, ok := fc.HealthStatus[provider]
	if !ok {
		return
	}

	h.ConsecutiveFailures++
	h.LastFailure = time.Now()

	switch {
	case h.ConsecutiveFailures >= downThreshold:
		h.Status = statusDown
		cd := fc.cooldownFor(provider)
		until := time.Now().Add(cd)
		h.CooldownUntil = &until
	case h.ConsecutiveFailures >= degradedThreshold:
		h.Status = statusDegraded
	}
}

// GetFallback finds the next healthy provider after the current one in the chain.
func (fc *FallbackChain) GetFallback(currentProvider string) (*ProviderConfig, error) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	now := time.Now()
	found := false
	for i := range fc.Providers {
		p := &fc.Providers[i]
		if p.Name == currentProvider {
			found = true
			continue
		}
		if !found {
			continue
		}
		h, ok := fc.HealthStatus[p.Name]
		if !ok {
			continue
		}
		if h.CooldownUntil != nil && now.Before(*h.CooldownUntil) {
			continue
		}
		if h.Status == statusDown {
			if h.CooldownUntil == nil || now.Before(*h.CooldownUntil) {
				continue
			}
		}
		return p, nil
	}

	// Wrap around: check providers before currentProvider
	for i := range fc.Providers {
		p := &fc.Providers[i]
		if p.Name == currentProvider {
			break
		}
		h, ok := fc.HealthStatus[p.Name]
		if !ok {
			continue
		}
		if h.CooldownUntil != nil && now.Before(*h.CooldownUntil) {
			continue
		}
		if h.Status == statusDown {
			if h.CooldownUntil == nil || now.Before(*h.CooldownUntil) {
				continue
			}
		}
		return p, nil
	}

	return nil, fmt.Errorf("no fallback provider available")
}

// IsHealthy returns true if the named provider is currently healthy and not in cooldown.
func (fc *FallbackChain) IsHealthy(provider string) bool {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	h, ok := fc.HealthStatus[provider]
	if !ok {
		return false
	}
	if h.CooldownUntil != nil && time.Now().Before(*h.CooldownUntil) {
		return false
	}
	return h.Status == statusHealthy
}

// FormatStatus returns a human-readable status report of all providers.
func (fc *FallbackChain) FormatStatus() string {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("Provider Status:\n")
	sb.WriteString("─────────────────────────\n")

	for i, p := range fc.Providers {
		h := fc.HealthStatus[p.Name]
		statusTag := strings.ToUpper(h.Status)
		line := fmt.Sprintf("%d. %s [%s]", i+1, p.Name, statusTag)

		switch h.Status {
		case statusHealthy:
			line += fmt.Sprintf(" latency: %s, priority: %d", formatDuration(h.Latency), p.Priority)
		case statusDegraded:
			line += fmt.Sprintf(" latency: %s, %d failures", formatDuration(h.Latency), h.ConsecutiveFailures)
		case statusDown:
			if h.CooldownUntil != nil {
				line += fmt.Sprintf(" cooldown until %s (%d failures)",
					h.CooldownUntil.Format("15:04"), h.ConsecutiveFailures)
			} else {
				line += fmt.Sprintf(" (%d failures)", h.ConsecutiveFailures)
			}
		}

		sb.WriteString(line)
		sb.WriteString("\n")
		_ = i
	}

	sb.WriteString("\nActive: ")
	sb.WriteString(fc.ActiveProvider)
	sb.WriteString("\n")

	sb.WriteString("Fallback chain: ")
	names := make([]string, len(fc.Providers))
	for i, p := range fc.Providers {
		names[i] = p.Name
	}
	sb.WriteString(strings.Join(names, " → "))
	sb.WriteString("\n")

	return sb.String()
}

// RecoverProvider manually marks a provider as healthy and clears its cooldown.
func (fc *FallbackChain) RecoverProvider(provider string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	h, ok := fc.HealthStatus[provider]
	if !ok {
		return
	}

	h.Status = statusHealthy
	h.ConsecutiveFailures = 0
	h.CooldownUntil = nil
}

// AllDown returns true if every provider is either down or in cooldown.
func (fc *FallbackChain) AllDown() bool {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	now := time.Now()
	for _, p := range fc.Providers {
		h, ok := fc.HealthStatus[p.Name]
		if !ok {
			continue
		}
		if h.Status != statusDown {
			if h.CooldownUntil == nil || !now.Before(*h.CooldownUntil) {
				return false
			}
		} else {
			// Down but cooldown expired means it can be retried
			if h.CooldownUntil != nil && !now.Before(*h.CooldownUntil) {
				return false
			}
			if h.CooldownUntil == nil {
				// Down with no cooldown is still considered unavailable
				continue
			}
		}
	}
	return true
}

// BestLatency returns the provider config with the lowest recent latency
// among healthy providers. Returns nil if no providers are healthy.
func (fc *FallbackChain) BestLatency() *ProviderConfig {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	var best *ProviderConfig
	var bestLatency time.Duration

	for i := range fc.Providers {
		p := &fc.Providers[i]
		h, ok := fc.HealthStatus[p.Name]
		if !ok {
			continue
		}
		if h.Status == statusDown {
			continue
		}
		if h.Latency == 0 {
			continue
		}
		if best == nil || h.Latency < bestLatency {
			best = p
			bestLatency = h.Latency
		}
	}

	return best
}

// cooldownFor returns the cooldown duration for a provider from its config.
func (fc *FallbackChain) cooldownFor(provider string) time.Duration {
	for _, p := range fc.Providers {
		if p.Name == provider {
			if p.CooldownDuration > 0 {
				return p.CooldownDuration
			}
			return 60 * time.Second // default cooldown
		}
	}
	return 60 * time.Second
}

// formatDuration formats a duration for display in status output.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "n/a"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
