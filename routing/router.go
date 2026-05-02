package routing

import (
	"fmt"
	"sync"
	"time"
)

// RoutingStrategy determines how the router selects providers.
type RoutingStrategy string

const (
	StrategyLatency  RoutingStrategy = "latency"
	StrategyCost     RoutingStrategy = "cost"
	StrategyBalanced RoutingStrategy = "balanced"
)

// LatencyClass categorizes model response speed.
type LatencyClass string

const (
	LatencyFast   LatencyClass = "fast"
	LatencyMedium LatencyClass = "medium"
	LatencySlow   LatencyClass = "slow"
)

// Capabilities describes what a model supports.
type Capabilities struct {
	Streaming       bool `json:"streaming"`
	FunctionCalling bool `json:"function_calling"`
	Vision          bool `json:"vision"`
	JSON            bool `json:"json"`
	Thinking        bool `json:"thinking"`
}

// ProviderHealth tracks the health state of a provider.
type ProviderHealth struct {
	Available        bool      `json:"available"`
	LastCheck        time.Time `json:"last_check"`
	LastSuccess      time.Time `json:"last_success"`
	ConsecutiveFails int       `json:"consecutive_fails"`
	AvgLatencyMs     float64   `json:"avg_latency_ms"`
}

// CircuitState represents circuit breaker state.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // normal
	CircuitOpen                         // rejecting
	CircuitHalfOpen                     // testing
)

const (
	circuitOpenThreshold  = 3
	circuitRecoveryDelay  = 30 * time.Second
	circuitHalfOpenPasses = 3
	latencyEMAAlpha       = 0.3
)

// Router provides health-aware provider routing with fallback.
type Router struct {
	mu            sync.RWMutex
	health        map[string]*ProviderHealth
	circuits      map[string]*circuitBreaker
	fallbackChain []string
	strategy      RoutingStrategy
}

type circuitBreaker struct {
	state        CircuitState
	failures     int
	lastFailure  time.Time
	halfOpenPass int
}

// NewRouter creates a new provider router with a default fallback chain.
func NewRouter(strategy RoutingStrategy) *Router {
	return &Router{
		health:   make(map[string]*ProviderHealth),
		circuits: make(map[string]*circuitBreaker),
		fallbackChain: []string{
			"anthropic", "openai", "gemini", "openrouter", "groq", "deepseek",
		},
		strategy: strategy,
	}
}

// SetFallbackChain sets the provider fallback order.
func (r *Router) SetFallbackChain(chain []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallbackChain = chain
}

// SelectProvider chooses the best available provider, falling back if needed.
func (r *Router) SelectProvider(preferred string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if preferred != "" && r.isAvailable(preferred) {
		return preferred, nil
	}

	for _, provider := range r.fallbackChain {
		if r.isAvailable(provider) {
			return provider, nil
		}
	}

	// All providers down, return preferred anyway
	if preferred != "" {
		return preferred, nil
	}
	return "", fmt.Errorf("no available providers")
}

// SelectProviderForModel chooses the best provider for a specific model.
func (r *Router) SelectProviderForModel(modelName string) (string, ModelInfo, error) {
	info, found := Find(modelName)
	if !found {
		return "", ModelInfo{}, fmt.Errorf("model %q not found in catalog", modelName)
	}

	provider, err := r.SelectProvider(info.Provider)
	if err != nil {
		return "", ModelInfo{}, err
	}
	return provider, info, nil
}

// RecordSuccess records a successful API call for a provider.
func (r *Router) RecordSuccess(provider string, latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	h := r.getOrCreateHealth(provider)
	h.Available = true
	h.LastSuccess = time.Now()
	h.LastCheck = time.Now()
	h.ConsecutiveFails = 0

	latencyMs := float64(latency.Milliseconds())
	if h.AvgLatencyMs == 0 {
		h.AvgLatencyMs = latencyMs
	} else {
		h.AvgLatencyMs = latencyEMAAlpha*latencyMs + (1-latencyEMAAlpha)*h.AvgLatencyMs
	}

	cb := r.getOrCreateCircuit(provider)
	if cb.state == CircuitHalfOpen {
		cb.halfOpenPass++
		if cb.halfOpenPass >= circuitHalfOpenPasses {
			cb.state = CircuitClosed
			cb.failures = 0
			cb.halfOpenPass = 0
		}
	} else if cb.state == CircuitOpen {
		cb.state = CircuitClosed
		cb.failures = 0
	}
}

// RecordFailure records a failed API call for a provider.
func (r *Router) RecordFailure(provider string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	h := r.getOrCreateHealth(provider)
	h.ConsecutiveFails++
	h.LastCheck = time.Now()

	cb := r.getOrCreateCircuit(provider)
	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= circuitOpenThreshold {
		cb.state = CircuitOpen
		h.Available = false
	}
}

// HealthStatus returns health info for all tracked providers.
func (r *Router) HealthStatus() map[string]*ProviderHealth {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]*ProviderHealth, len(r.health))
	for k, v := range r.health {
		copy := *v
		out[k] = &copy
	}
	return out
}

// Score returns a routing score for a provider (lower is better).
func (r *Router) Score(provider string) float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	h, ok := r.health[provider]
	if !ok {
		return 100.0 // unknown provider, neutral score
	}

	var score float64
	switch r.strategy {
	case StrategyLatency:
		score = h.AvgLatencyMs / 1000.0
	case StrategyCost:
		if m, ok := Recommended(provider); ok {
			score = m.InputPrice
		}
	case StrategyBalanced:
		latencyScore := h.AvgLatencyMs / 1000.0
		costScore := 0.0
		if m, ok := Recommended(provider); ok {
			costScore = m.InputPrice
		}
		score = latencyScore*0.5 + costScore*0.5
	}

	// Penalty for recent failures
	score += float64(h.ConsecutiveFails) * 10.0

	return score
}

func (r *Router) isAvailable(provider string) bool {
	cb, ok := r.circuits[provider]
	if !ok {
		return true
	}

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFailure) > circuitRecoveryDelay {
			cb.state = CircuitHalfOpen
			cb.halfOpenPass = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	}
	return true
}

func (r *Router) getOrCreateHealth(provider string) *ProviderHealth {
	h, ok := r.health[provider]
	if !ok {
		h = &ProviderHealth{Available: true}
		r.health[provider] = h
	}
	return h
}

func (r *Router) getOrCreateCircuit(provider string) *circuitBreaker {
	cb, ok := r.circuits[provider]
	if !ok {
		cb = &circuitBreaker{state: CircuitClosed}
		r.circuits[provider] = cb
	}
	return cb
}
