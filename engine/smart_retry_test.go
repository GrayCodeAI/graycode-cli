package engine

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestClassifyError(t *testing.T) {
	sr := NewSmartRetry()

	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"rate limit 429", fmt.Errorf("HTTP 429: too many requests"), "rate_limit"},
		{"rate limit text", fmt.Errorf("rate limit exceeded"), "rate_limit"},
		{"rate_limit underscore", fmt.Errorf("rate_limit reached"), "rate_limit"},
		{"timeout context", fmt.Errorf("context deadline exceeded"), "timeout"},
		{"timeout text", fmt.Errorf("request timeout after 30s"), "timeout"},
		{"timed out", fmt.Errorf("connection timed out"), "timeout"},
		{"server error 500", fmt.Errorf("HTTP 500: internal server error"), "server_error"},
		{"server error 502", fmt.Errorf("bad gateway 502"), "server_error"},
		{"server error 503", fmt.Errorf("service unavailable 503"), "server_error"},
		{"overloaded 529", fmt.Errorf("HTTP 529: overloaded"), "overloaded"},
		{"overloaded text", fmt.Errorf("API is overloaded"), "overloaded"},
		{"invalid request 400", fmt.Errorf("HTTP 400: bad request"), "invalid_request"},
		{"invalid text", fmt.Errorf("invalid model parameter"), "invalid_request"},
		{"auth 401", fmt.Errorf("HTTP 401: unauthorized"), "auth"},
		{"auth 403", fmt.Errorf("HTTP 403: forbidden"), "auth"},
		{"auth text", fmt.Errorf("unauthorized access"), "auth"},
		{"network connection refused", fmt.Errorf("dial tcp: connection refused"), "network"},
		{"network eof", fmt.Errorf("unexpected EOF"), "network"},
		{"network broken pipe", fmt.Errorf("write: broken pipe"), "network"},
		{"unknown error", fmt.Errorf("something completely different happened"), "unknown"},
		{"nil error", nil, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sr.ClassifyError(tt.err)
			if result != tt.expected {
				t.Errorf("ClassifyError(%v) = %q, want %q", tt.err, result, tt.expected)
			}
		})
	}
}

func TestCalculateDelay(t *testing.T) {
	sr := NewSmartRetry()

	strategy := &RetryStrategy{
		Provider:          "test",
		BaseDelay:         1 * time.Second,
		MaxDelay:          60 * time.Second,
		MaxRetries:        5,
		BackoffMultiplier: 2.0,
		JitterPct:         0, // No jitter for deterministic testing
		RetryOn:           []string{"rate_limit"},
		AbortOn:           []string{"auth"},
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},  // 1s * 2^0 = 1s
		{1, 2 * time.Second},  // 1s * 2^1 = 2s
		{2, 4 * time.Second},  // 1s * 2^2 = 4s
		{3, 8 * time.Second},  // 1s * 2^3 = 8s
		{4, 16 * time.Second}, // 1s * 2^4 = 16s
		{5, 32 * time.Second}, // 1s * 2^5 = 32s
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			result := sr.CalculateDelay(strategy, tt.attempt)
			if result != tt.expected {
				t.Errorf("CalculateDelay(attempt=%d) = %v, want %v", tt.attempt, result, tt.expected)
			}
		})
	}
}

func TestCalculateDelayMaxCap(t *testing.T) {
	sr := NewSmartRetry()

	strategy := &RetryStrategy{
		Provider:          "test",
		BaseDelay:         1 * time.Second,
		MaxDelay:          10 * time.Second,
		MaxRetries:        10,
		BackoffMultiplier: 2.0,
		JitterPct:         0,
	}

	// attempt 10 would be 1024s without cap, but should be capped at 10s
	result := sr.CalculateDelay(strategy, 10)
	if result != 10*time.Second {
		t.Errorf("CalculateDelay should be capped at MaxDelay, got %v", result)
	}
}

func TestJitterStaysWithinBounds(t *testing.T) {
	sr := NewSmartRetry()

	strategy := &RetryStrategy{
		Provider:          "test",
		BaseDelay:         1 * time.Second,
		MaxDelay:          60 * time.Second,
		MaxRetries:        5,
		BackoffMultiplier: 2.0,
		JitterPct:         25, // ±25%
	}

	// Run many times to test jitter bounds
	for i := 0; i < 1000; i++ {
		delay := sr.CalculateDelay(strategy, 2) // base delay = 4s at attempt 2
		baseExpected := 4 * time.Second
		minExpected := time.Duration(float64(baseExpected) * 0.75) // -25%
		maxExpected := time.Duration(float64(baseExpected) * 1.25) // +25%

		if delay < minExpected || delay > maxExpected {
			t.Errorf("CalculateDelay with jitter = %v, want between %v and %v",
				delay, minExpected, maxExpected)
		}
	}
}

func TestShouldFallbackTriggersAfterThreshold(t *testing.T) {
	sr := NewSmartRetry()

	// Add 4 failures for anthropic (threshold is >3)
	for i := 0; i < 4; i++ {
		sr.mu.Lock()
		sr.addRecord(FailureRecord{
			Provider:  "anthropic",
			Model:     "claude-3",
			ErrorType: "rate_limit",
			ErrorMsg:  "429 too many requests",
			Timestamp: time.Now(),
			Recovered: false,
		})
		sr.mu.Unlock()
	}

	shouldFallback, alt := sr.ShouldFallback("anthropic")
	if !shouldFallback {
		t.Error("ShouldFallback should return true after >3 failures")
	}
	if alt == "" {
		t.Error("ShouldFallback should suggest an alternative provider")
	}
	if alt == "anthropic" {
		t.Error("ShouldFallback should not suggest the same provider")
	}
}

func TestShouldFallbackDoesNotTriggerBelowThreshold(t *testing.T) {
	sr := NewSmartRetry()

	// Add 2 failures (below threshold)
	for i := 0; i < 2; i++ {
		sr.mu.Lock()
		sr.addRecord(FailureRecord{
			Provider:  "anthropic",
			Model:     "claude-3",
			ErrorType: "rate_limit",
			Timestamp: time.Now(),
			Recovered: false,
		})
		sr.mu.Unlock()
	}

	shouldFallback, _ := sr.ShouldFallback("anthropic")
	if shouldFallback {
		t.Error("ShouldFallback should not trigger with only 2 failures")
	}
}

func TestAbortOnPatternsStopRetries(t *testing.T) {
	sr := NewSmartRetry()

	// Auth errors should abort immediately for anthropic
	decision := sr.Decide("anthropic", "claude-3", fmt.Errorf("HTTP 401: unauthorized"), 0)
	if decision.ShouldRetry {
		t.Error("Should not retry on auth error (abort pattern)")
	}
	if decision.Reason == "" {
		t.Error("Decision should include a reason")
	}

	// Invalid request should abort
	decision = sr.Decide("openai", "gpt-4", fmt.Errorf("HTTP 400: invalid model"), 0)
	if decision.ShouldRetry {
		t.Error("Should not retry on invalid request (abort pattern)")
	}
}

func TestRetryOnPatternsAllowRetries(t *testing.T) {
	sr := NewSmartRetry()

	// Rate limit should be retried
	decision := sr.Decide("anthropic", "claude-3", fmt.Errorf("HTTP 429: too many requests"), 0)
	if !decision.ShouldRetry {
		t.Errorf("Should retry on rate limit error, got reason: %s", decision.Reason)
	}
	if decision.Delay <= 0 {
		t.Error("Retry delay should be positive")
	}

	// Server error should be retried
	decision = sr.Decide("openai", "gpt-4", fmt.Errorf("HTTP 500: internal server error"), 0)
	if !decision.ShouldRetry {
		t.Errorf("Should retry on server error, got reason: %s", decision.Reason)
	}
}

func TestMaxRetriesEnforcement(t *testing.T) {
	sr := NewSmartRetry()

	// Anthropic has MaxRetries=5, so attempt 5 should fail
	decision := sr.Decide("anthropic", "claude-3", fmt.Errorf("HTTP 429: too many requests"), 5)
	if decision.ShouldRetry {
		t.Error("Should not retry when max retries exceeded")
	}

	// Attempt 4 should still work (0-indexed, so attempt 4 = 5th try)
	sr2 := NewSmartRetry()
	decision = sr2.Decide("anthropic", "claude-3", fmt.Errorf("HTTP 429: too many requests"), 4)
	if !decision.ShouldRetry {
		t.Error("Should still retry at attempt 4 (below max 5)")
	}
}

func TestStrategyAdaptation(t *testing.T) {
	sr := NewSmartRetry()

	originalBaseDelay := sr.Strategies["anthropic"].BaseDelay

	// Add many rate limit errors
	sr.mu.Lock()
	for i := 0; i < 10; i++ {
		sr.addRecord(FailureRecord{
			Provider:  "anthropic",
			ErrorType: "rate_limit",
			Timestamp: time.Now(),
			Recovered: false,
		})
	}
	sr.mu.Unlock()

	sr.AdaptStrategy("anthropic")

	newBaseDelay := sr.Strategies["anthropic"].BaseDelay
	if newBaseDelay <= originalBaseDelay {
		t.Errorf("AdaptStrategy should increase base delay for rate-limited provider, got %v (was %v)",
			newBaseDelay, originalBaseDelay)
	}
}

func TestStrategyAdaptationTimeout(t *testing.T) {
	sr := NewSmartRetry()

	originalMaxRetries := sr.Strategies["openai"].MaxRetries

	// Add many timeout errors
	sr.mu.Lock()
	for i := 0; i < 10; i++ {
		sr.addRecord(FailureRecord{
			Provider:  "openai",
			ErrorType: "timeout",
			Timestamp: time.Now(),
			Recovered: false,
		})
	}
	sr.mu.Unlock()

	sr.AdaptStrategy("openai")

	newMaxRetries := sr.Strategies["openai"].MaxRetries
	if newMaxRetries >= originalMaxRetries {
		t.Errorf("AdaptStrategy should reduce max retries for timeout-heavy provider, got %d (was %d)",
			newMaxRetries, originalMaxRetries)
	}
}

func TestProviderHealthAssessment(t *testing.T) {
	sr := NewSmartRetry()

	// No failures = healthy
	health := sr.GetProviderHealth()
	for provider, status := range health {
		if status != "healthy" {
			t.Errorf("Provider %s should be healthy with no failures, got %s", provider, status)
		}
	}

	// Add 2 failures for openai (degraded: 1-3 failures)
	sr.mu.Lock()
	for i := 0; i < 2; i++ {
		sr.addRecord(FailureRecord{
			Provider:  "openai",
			ErrorType: "rate_limit",
			Timestamp: time.Now(),
			Recovered: false,
		})
	}
	sr.mu.Unlock()

	health = sr.GetProviderHealth()
	if health["openai"] != "degraded" {
		t.Errorf("OpenAI should be degraded with 2 failures, got %s", health["openai"])
	}
	if health["anthropic"] != "healthy" {
		t.Errorf("Anthropic should still be healthy, got %s", health["anthropic"])
	}

	// Add 5 more failures for openai (failing: >3 failures)
	sr.mu.Lock()
	for i := 0; i < 5; i++ {
		sr.addRecord(FailureRecord{
			Provider:  "openai",
			ErrorType: "server_error",
			Timestamp: time.Now(),
			Recovered: false,
		})
	}
	sr.mu.Unlock()

	health = sr.GetProviderHealth()
	if health["openai"] != "failing" {
		t.Errorf("OpenAI should be failing with 7 failures, got %s", health["openai"])
	}
}

func TestSmartRetryConcurrentAccess(t *testing.T) {
	sr := NewSmartRetry()

	var wg sync.WaitGroup
	errCount := 100

	// Concurrent Decide calls
	for i := 0; i < errCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			providers := []string{"anthropic", "openai", "ollama"}
			provider := providers[idx%3]
			sr.Decide(provider, "test-model", fmt.Errorf("HTTP 429: rate limit"), idx%5)
		}(i)
	}

	// Concurrent health checks
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sr.GetProviderHealth()
		}()
	}

	// Concurrent classify
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sr.ClassifyError(fmt.Errorf("timeout"))
		}()
	}

	// Concurrent fallback checks
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sr.ShouldFallback("anthropic")
		}()
	}

	// Concurrent adapt
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sr.AdaptStrategy("openai")
		}()
	}

	wg.Wait()

	// If we get here without a race condition, the test passes
	sr.mu.RLock()
	histLen := len(sr.FailureHistory)
	sr.mu.RUnlock()

	if histLen == 0 {
		t.Error("Expected some failure records to be recorded")
	}
}

func TestFormatStatus(t *testing.T) {
	sr := NewSmartRetry()

	status := sr.FormatStatus()
	if status == "" {
		t.Error("FormatStatus should return non-empty string")
	}
	if !retryContains(status, "Provider Status:") {
		t.Error("FormatStatus should contain header")
	}
	if !retryContains(status, "healthy") {
		t.Error("FormatStatus should show healthy providers")
	}
}

func TestRecordRecovery(t *testing.T) {
	sr := NewSmartRetry()

	// Add a failure
	sr.mu.Lock()
	sr.addRecord(FailureRecord{
		Provider:  "anthropic",
		Model:     "claude-3",
		ErrorType: "rate_limit",
		Timestamp: time.Now(),
		Recovered: false,
	})
	sr.mu.Unlock()

	// Record recovery
	sr.RecordRecovery("anthropic", "claude-3", 5*time.Second)

	sr.mu.RLock()
	recovered := false
	for _, rec := range sr.FailureHistory {
		if rec.Provider == "anthropic" && rec.Recovered {
			recovered = true
			if rec.RecoveryDelay != 5*time.Second {
				t.Errorf("RecoveryDelay should be 5s, got %v", rec.RecoveryDelay)
			}
		}
	}
	sr.mu.RUnlock()

	if !recovered {
		t.Error("RecordRecovery should mark failures as recovered")
	}
}

func TestResetProvider(t *testing.T) {
	sr := NewSmartRetry()

	// Add failures for multiple providers
	sr.mu.Lock()
	sr.addRecord(FailureRecord{Provider: "anthropic", Timestamp: time.Now()})
	sr.addRecord(FailureRecord{Provider: "openai", Timestamp: time.Now()})
	sr.addRecord(FailureRecord{Provider: "anthropic", Timestamp: time.Now()})
	sr.mu.Unlock()

	sr.ResetProvider("anthropic")

	sr.mu.RLock()
	for _, rec := range sr.FailureHistory {
		if rec.Provider == "anthropic" {
			t.Error("ResetProvider should remove all records for that provider")
		}
	}
	openaiFound := false
	for _, rec := range sr.FailureHistory {
		if rec.Provider == "openai" {
			openaiFound = true
		}
	}
	sr.mu.RUnlock()

	if !openaiFound {
		t.Error("ResetProvider should not affect other providers")
	}
}

func TestUnknownProviderGetsDefaultStrategy(t *testing.T) {
	sr := NewSmartRetry()

	// Deciding for unknown provider should still work
	decision := sr.Decide("groq", "llama-3", fmt.Errorf("HTTP 429: rate limit"), 0)
	if !decision.ShouldRetry {
		t.Error("Unknown provider should get default strategy that retries on 429")
	}

	// Check strategy was created
	sr.mu.RLock()
	_, exists := sr.Strategies["groq"]
	sr.mu.RUnlock()
	if !exists {
		t.Error("Default strategy should be created for unknown provider")
	}
}

func TestHistoryTrimming(t *testing.T) {
	sr := NewSmartRetry()
	sr.MaxHistorySize = 10

	sr.mu.Lock()
	for i := 0; i < 15; i++ {
		sr.addRecord(FailureRecord{
			Provider:  "anthropic",
			ErrorType: "rate_limit",
			Timestamp: time.Now(),
		})
	}
	sr.mu.Unlock()

	sr.mu.RLock()
	histLen := len(sr.FailureHistory)
	sr.mu.RUnlock()

	if histLen > sr.MaxHistorySize {
		t.Errorf("History should be trimmed to MaxHistorySize, got %d", histLen)
	}
}

func TestDecisionIncludesFallbackWhenProviderFailing(t *testing.T) {
	sr := NewSmartRetry()

	// Saturate anthropic with failures
	sr.mu.Lock()
	for i := 0; i < 5; i++ {
		sr.addRecord(FailureRecord{
			Provider:  "anthropic",
			Model:     "claude-3",
			ErrorType: "rate_limit",
			Timestamp: time.Now(),
			Recovered: false,
		})
	}
	sr.mu.Unlock()

	// Next retry decision should include fallback suggestion
	decision := sr.Decide("anthropic", "claude-3", fmt.Errorf("HTTP 429: rate limit"), 0)
	if decision.FallbackProvider == "" {
		t.Error("Decision should suggest fallback when provider has many failures")
	}
}

func TestOllamaShorterTimeouts(t *testing.T) {
	sr := NewSmartRetry()

	strategy := sr.Strategies["ollama"]
	if strategy.MaxDelay != 10*time.Second {
		t.Errorf("Ollama MaxDelay should be 10s, got %v", strategy.MaxDelay)
	}
	if strategy.MaxRetries != 3 {
		t.Errorf("Ollama MaxRetries should be 3, got %d", strategy.MaxRetries)
	}
	if strategy.BackoffMultiplier != 1.5 {
		t.Errorf("Ollama BackoffMultiplier should be 1.5, got %f", strategy.BackoffMultiplier)
	}
}

func retryContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
