package engine

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// SmartRetry provides intelligent retry handling that learns from failure patterns
// and adapts retry strategies per-provider.
type SmartRetry struct {
	Strategies     map[string]*RetryStrategy
	FailureHistory []FailureRecord
	MaxHistorySize int
	mu             sync.RWMutex
}

// RetryStrategy defines the retry behavior for a specific provider.
type RetryStrategy struct {
	Provider          string
	BaseDelay         time.Duration
	MaxDelay          time.Duration
	MaxRetries        int
	BackoffMultiplier float64
	JitterPct         float64  // 0-100, adds randomness
	RetryOn           []string // error patterns to retry on
	AbortOn           []string // error patterns to immediately fail on
}

// FailureRecord captures information about a failure for learning.
type FailureRecord struct {
	Provider      string
	Model         string
	ErrorType     string // "rate_limit", "timeout", "server_error", "invalid_request", "auth"
	ErrorMsg      string
	Timestamp     time.Time
	RetryCount    int
	Recovered     bool
	RecoveryDelay time.Duration
}

// RetryDecision describes what the caller should do after a failure.
type RetryDecision struct {
	ShouldRetry      bool
	Delay            time.Duration
	Reason           string
	FallbackProvider string // switch provider if this one is failing
	FallbackModel    string
}

// NewSmartRetry creates a SmartRetry with default strategies per provider.
func NewSmartRetry() *SmartRetry {
	sr := &SmartRetry{
		Strategies:     make(map[string]*RetryStrategy),
		FailureHistory: make([]FailureRecord, 0, 100),
		MaxHistorySize: 1000,
	}

	// Anthropic: base 1s, max 60s, multiply 2x, retry on 429/529/500, abort on 401/400
	sr.Strategies["anthropic"] = &RetryStrategy{
		Provider:          "anthropic",
		BaseDelay:         1 * time.Second,
		MaxDelay:          60 * time.Second,
		MaxRetries:        5,
		BackoffMultiplier: 2.0,
		JitterPct:         20,
		RetryOn:           []string{"429", "529", "500", "rate_limit", "overloaded", "server_error"},
		AbortOn:           []string{"401", "400", "unauthorized", "invalid_request"},
	}

	// OpenAI: base 500ms, max 30s, multiply 2x, retry on 429/500/503
	sr.Strategies["openai"] = &RetryStrategy{
		Provider:          "openai",
		BaseDelay:         500 * time.Millisecond,
		MaxDelay:          30 * time.Second,
		MaxRetries:        5,
		BackoffMultiplier: 2.0,
		JitterPct:         25,
		RetryOn:           []string{"429", "500", "503", "rate_limit", "server_error"},
		AbortOn:           []string{"401", "400", "unauthorized", "invalid_request"},
	}

	// Ollama: base 2s, max 10s, multiply 1.5x (local, shorter timeouts)
	sr.Strategies["ollama"] = &RetryStrategy{
		Provider:          "ollama",
		BaseDelay:         2 * time.Second,
		MaxDelay:          10 * time.Second,
		MaxRetries:        3,
		BackoffMultiplier: 1.5,
		JitterPct:         10,
		RetryOn:           []string{"500", "503", "timeout", "server_error", "network"},
		AbortOn:           []string{"400", "invalid_request"},
	}

	return sr
}

// Decide evaluates whether a request should be retried based on the provider,
// error, and attempt count. It classifies the error, checks strategy, calculates
// delay with jitter, and may suggest fallback providers.
func (sr *SmartRetry) Decide(provider, model string, err error, attempt int) *RetryDecision {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	errorType := sr.classifyErrorInternal(err)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	strategy := sr.getStrategy(provider)

	// Record failure
	record := FailureRecord{
		Provider:   provider,
		Model:      model,
		ErrorType:  errorType,
		ErrorMsg:   errMsg,
		Timestamp:  time.Now(),
		RetryCount: attempt,
		Recovered:  false,
	}
	sr.addRecord(record)

	// Check abort patterns first
	if sr.matchesPatterns(errMsg, errorType, strategy.AbortOn) {
		return &RetryDecision{
			ShouldRetry: false,
			Delay:       0,
			Reason:      fmt.Sprintf("error matches abort pattern: %s", errorType),
		}
	}

	// Check max retries
	if attempt >= strategy.MaxRetries {
		// Check if we should fallback
		shouldFallback, altProvider := sr.shouldFallbackInternal(provider)
		if shouldFallback {
			return &RetryDecision{
				ShouldRetry:      false,
				Delay:            0,
				Reason:           fmt.Sprintf("max retries (%d) exceeded, suggesting fallback", strategy.MaxRetries),
				FallbackProvider: altProvider,
			}
		}
		return &RetryDecision{
			ShouldRetry: false,
			Delay:       0,
			Reason:      fmt.Sprintf("max retries (%d) exceeded", strategy.MaxRetries),
		}
	}

	// Check retry patterns
	if !sr.matchesPatterns(errMsg, errorType, strategy.RetryOn) {
		return &RetryDecision{
			ShouldRetry: false,
			Delay:       0,
			Reason:      fmt.Sprintf("error type %q not in retry patterns", errorType),
		}
	}

	// Calculate delay
	delay := sr.calculateDelayInternal(strategy, attempt)

	// Check if we should suggest fallback even while retrying
	shouldFallback, altProvider := sr.shouldFallbackInternal(provider)
	decision := &RetryDecision{
		ShouldRetry: true,
		Delay:       delay,
		Reason:      fmt.Sprintf("retrying %s error (attempt %d/%d)", errorType, attempt+1, strategy.MaxRetries),
	}
	if shouldFallback {
		decision.FallbackProvider = altProvider
	}

	return decision
}

// ClassifyError categorizes an error into a known error type.
func (sr *SmartRetry) ClassifyError(err error) string {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.classifyErrorInternal(err)
}

func (sr *SmartRetry) classifyErrorInternal(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := strings.ToLower(err.Error())

	// Rate limit
	if strings.Contains(msg, "429") || strings.Contains(msg, "too many requests") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "rate_limit") {
		return "rate_limit"
	}

	// Overloaded (check before server_error since 529 could match both)
	if strings.Contains(msg, "529") || strings.Contains(msg, "overloaded") {
		return "overloaded"
	}

	// Timeout
	if strings.Contains(msg, "context deadline") || strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out") {
		return "timeout"
	}

	// Auth
	if strings.Contains(msg, "401") || strings.Contains(msg, "403") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "forbidden") {
		return "auth"
	}

	// Invalid request
	if strings.Contains(msg, "400") || strings.Contains(msg, "invalid") || strings.Contains(msg, "bad request") {
		return "invalid_request"
	}

	// Server error
	if strings.Contains(msg, "500") || strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "internal server error") {
		return "server_error"
	}

	// Network
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "eof") || strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset") || strings.Contains(msg, "no such host") {
		return "network"
	}

	return "unknown"
}

// CalculateDelay computes the delay for a given attempt using exponential backoff with jitter.
func (sr *SmartRetry) CalculateDelay(strategy *RetryStrategy, attempt int) time.Duration {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.calculateDelayInternal(strategy, attempt)
}

func (sr *SmartRetry) calculateDelayInternal(strategy *RetryStrategy, attempt int) time.Duration {
	// Exponential backoff: baseDelay * multiplier^attempt
	multiplier := math.Pow(strategy.BackoffMultiplier, float64(attempt))
	delay := time.Duration(float64(strategy.BaseDelay) * multiplier)

	// Cap at maxDelay
	if delay > strategy.MaxDelay {
		delay = strategy.MaxDelay
	}

	// Apply jitter: ±JitterPct%
	if strategy.JitterPct > 0 {
		jitterFraction := strategy.JitterPct / 100.0
		jitterRange := float64(delay) * jitterFraction
		jitter := (rand.Float64()*2 - 1) * jitterRange // between -jitterRange and +jitterRange
		delay = time.Duration(float64(delay) + jitter)
		// Ensure delay doesn't go below 0
		if delay < 0 {
			delay = time.Duration(float64(strategy.BaseDelay) * 0.5)
		}
	}

	return delay
}

// ShouldFallback checks if a provider has too many recent failures and suggests an alternative.
func (sr *SmartRetry) ShouldFallback(provider string) (bool, string) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.shouldFallbackInternal(provider)
}

func (sr *SmartRetry) shouldFallbackInternal(provider string) (bool, string) {
	cutoff := time.Now().Add(-5 * time.Minute)
	failCount := 0

	for i := len(sr.FailureHistory) - 1; i >= 0; i-- {
		rec := sr.FailureHistory[i]
		if rec.Timestamp.Before(cutoff) {
			break
		}
		if rec.Provider == provider && !rec.Recovered {
			failCount++
		}
	}

	if failCount > 3 {
		alt := sr.suggestAlternative(provider)
		if alt != "" {
			return true, alt
		}
	}

	return false, ""
}

func (sr *SmartRetry) suggestAlternative(provider string) string {
	// Suggest an alternative provider that is not failing
	alternatives := map[string][]string{
		"anthropic": {"openai", "ollama"},
		"openai":    {"anthropic", "ollama"},
		"ollama":    {"anthropic", "openai"},
	}

	alts, ok := alternatives[provider]
	if !ok {
		return ""
	}

	cutoff := time.Now().Add(-5 * time.Minute)
	for _, alt := range alts {
		failCount := 0
		for _, rec := range sr.FailureHistory {
			if rec.Provider == alt && rec.Timestamp.After(cutoff) && !rec.Recovered {
				failCount++
			}
		}
		if failCount <= 3 {
			return alt
		}
	}

	return ""
}

// AdaptStrategy learns from failure history and adjusts the strategy for a provider.
func (sr *SmartRetry) AdaptStrategy(provider string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	strategy := sr.getStrategy(provider)
	cutoff := time.Now().Add(-10 * time.Minute)

	rateLimitCount := 0
	timeoutCount := 0
	totalRecent := 0

	for _, rec := range sr.FailureHistory {
		if rec.Provider != provider || rec.Timestamp.Before(cutoff) {
			continue
		}
		totalRecent++
		switch rec.ErrorType {
		case "rate_limit":
			rateLimitCount++
		case "timeout":
			timeoutCount++
		}
	}

	if totalRecent == 0 {
		return
	}

	// If rate limits are frequent (>50% of errors), increase base delay
	if float64(rateLimitCount)/float64(totalRecent) > 0.5 {
		newDelay := time.Duration(float64(strategy.BaseDelay) * 1.5)
		if newDelay <= strategy.MaxDelay {
			strategy.BaseDelay = newDelay
		}
	}

	// If timeouts are frequent (>50% of errors), reduce max retries
	if float64(timeoutCount)/float64(totalRecent) > 0.5 {
		if strategy.MaxRetries > 2 {
			strategy.MaxRetries--
		}
	}
}

// GetProviderHealth returns the health status of all known providers.
func (sr *SmartRetry) GetProviderHealth() map[string]string {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	health := make(map[string]string)
	cutoff := time.Now().Add(-5 * time.Minute)

	for provider := range sr.Strategies {
		failCount := 0
		for _, rec := range sr.FailureHistory {
			if rec.Provider == provider && rec.Timestamp.After(cutoff) && !rec.Recovered {
				failCount++
			}
		}

		switch {
		case failCount == 0:
			health[provider] = "healthy"
		case failCount <= 3:
			health[provider] = "degraded"
		default:
			health[provider] = "failing"
		}
	}

	return health
}

// FormatStatus returns a human-readable summary of provider health.
func (sr *SmartRetry) FormatStatus() string {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	cutoff := time.Now().Add(-5 * time.Minute)

	var sb strings.Builder
	sb.WriteString("Provider Status:\n")

	for provider, strategy := range sr.Strategies {
		failCount := 0
		rateLimitCount := 0
		var lastDelay time.Duration

		for _, rec := range sr.FailureHistory {
			if rec.Provider == provider && rec.Timestamp.After(cutoff) && !rec.Recovered {
				failCount++
				if rec.ErrorType == "rate_limit" {
					rateLimitCount++
				}
			}
		}

		switch {
		case failCount == 0:
			sb.WriteString(fmt.Sprintf("  %s: healthy (0 failures/5min)\n", provider))
		case failCount <= 3:
			lastDelay = sr.calculateDelayInternal(strategy, failCount-1)
			if rateLimitCount > 0 {
				sb.WriteString(fmt.Sprintf("  %s: degraded (%d rate limits/5min, backing off %s)\n",
					provider, rateLimitCount, lastDelay.Round(time.Millisecond)))
			} else {
				sb.WriteString(fmt.Sprintf("  %s: degraded (%d failures/5min)\n", provider, failCount))
			}
		default:
			lastDelay = sr.calculateDelayInternal(strategy, failCount-1)
			if rateLimitCount > 0 {
				sb.WriteString(fmt.Sprintf("  %s: failing (%d rate limits/5min, backing off %s)\n",
					provider, rateLimitCount, lastDelay.Round(time.Millisecond)))
			} else {
				sb.WriteString(fmt.Sprintf("  %s: failing (%d failures/5min)\n", provider, failCount))
			}
		}
	}

	return sb.String()
}

// RecordRecovery marks that a provider has recovered from failures.
func (sr *SmartRetry) RecordRecovery(provider, model string, recoveryDelay time.Duration) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	// Mark recent unrecovered failures as recovered
	for i := len(sr.FailureHistory) - 1; i >= 0; i-- {
		rec := &sr.FailureHistory[i]
		if rec.Provider == provider && rec.Model == model && !rec.Recovered {
			rec.Recovered = true
			rec.RecoveryDelay = recoveryDelay
		}
	}
}

// getStrategy returns the strategy for a provider, or a default if not found.
func (sr *SmartRetry) getStrategy(provider string) *RetryStrategy {
	if s, ok := sr.Strategies[provider]; ok {
		return s
	}
	// Default strategy for unknown providers
	defaultStrategy := &RetryStrategy{
		Provider:          provider,
		BaseDelay:         1 * time.Second,
		MaxDelay:          30 * time.Second,
		MaxRetries:        3,
		BackoffMultiplier: 2.0,
		JitterPct:         15,
		RetryOn:           []string{"429", "500", "503", "rate_limit", "server_error", "timeout"},
		AbortOn:           []string{"401", "400", "unauthorized", "invalid_request"},
	}
	sr.Strategies[provider] = defaultStrategy
	return defaultStrategy
}

// matchesPatterns checks if the error message or error type matches any of the patterns.
func (sr *SmartRetry) matchesPatterns(errMsg, errorType string, patterns []string) bool {
	lowMsg := strings.ToLower(errMsg)
	lowType := strings.ToLower(errorType)

	for _, pattern := range patterns {
		lowPattern := strings.ToLower(pattern)
		if strings.Contains(lowMsg, lowPattern) || strings.Contains(lowType, lowPattern) {
			return true
		}
	}
	return false
}

// addRecord adds a failure record to the history, trimming if necessary.
func (sr *SmartRetry) addRecord(record FailureRecord) {
	sr.FailureHistory = append(sr.FailureHistory, record)
	if len(sr.FailureHistory) > sr.MaxHistorySize {
		// Trim oldest 10%
		trimSize := sr.MaxHistorySize / 10
		sr.FailureHistory = sr.FailureHistory[trimSize:]
	}
}

// ResetProvider clears failure history for a specific provider.
func (sr *SmartRetry) ResetProvider(provider string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	filtered := make([]FailureRecord, 0, len(sr.FailureHistory))
	for _, rec := range sr.FailureHistory {
		if rec.Provider != provider {
			filtered = append(filtered, rec)
		}
	}
	sr.FailureHistory = filtered
}
