package routing

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func testProviders() []ProviderConfig {
	return []ProviderConfig{
		{
			Name:             "anthropic",
			Model:            "claude-opus-4-20250514",
			Priority:         1,
			Weight:           1.0,
			MaxRetries:       3,
			CooldownDuration: 30 * time.Second,
		},
		{
			Name:             "openai",
			Model:            "gpt-4o",
			Priority:         2,
			Weight:           0.8,
			MaxRetries:       3,
			CooldownDuration: 30 * time.Second,
		},
		{
			Name:             "ollama",
			Model:            "llama3",
			Priority:         3,
			Weight:           0.5,
			MaxRetries:       2,
			CooldownDuration: 60 * time.Second,
		},
	}
}

func TestNewFallbackChain(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	if len(fc.Providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(fc.Providers))
	}

	// Should be sorted by priority
	if fc.Providers[0].Name != "anthropic" {
		t.Errorf("expected first provider to be anthropic, got %s", fc.Providers[0].Name)
	}
	if fc.Providers[1].Name != "openai" {
		t.Errorf("expected second provider to be openai, got %s", fc.Providers[1].Name)
	}
	if fc.Providers[2].Name != "ollama" {
		t.Errorf("expected third provider to be ollama, got %s", fc.Providers[2].Name)
	}

	// All providers should be healthy
	for _, p := range fc.Providers {
		h := fc.HealthStatus[p.Name]
		if h.Status != statusHealthy {
			t.Errorf("provider %s should be healthy, got %s", p.Name, h.Status)
		}
	}

	// Active provider should be highest priority
	if fc.ActiveProvider != "anthropic" {
		t.Errorf("expected active provider to be anthropic, got %s", fc.ActiveProvider)
	}
}

func TestNewFallbackChainSortsByPriority(t *testing.T) {
	// Provide in reverse order
	providers := []ProviderConfig{
		{Name: "low", Priority: 10},
		{Name: "high", Priority: 1},
		{Name: "mid", Priority: 5},
	}

	fc := NewFallbackChain(providers)
	if fc.Providers[0].Name != "high" {
		t.Errorf("expected first provider to be high priority, got %s", fc.Providers[0].Name)
	}
	if fc.Providers[1].Name != "mid" {
		t.Errorf("expected second provider to be mid priority, got %s", fc.Providers[1].Name)
	}
	if fc.Providers[2].Name != "low" {
		t.Errorf("expected third provider to be low priority, got %s", fc.Providers[2].Name)
	}
}

func TestSelectProvider(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	p, err := fc.SelectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "anthropic" {
		t.Errorf("expected anthropic, got %s", p.Name)
	}
}

func TestSelectProviderSkipsCooldown(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	// Put anthropic in cooldown
	until := time.Now().Add(10 * time.Minute)
	fc.HealthStatus["anthropic"].CooldownUntil = &until
	fc.HealthStatus["anthropic"].Status = statusDown

	p, err := fc.SelectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "openai" {
		t.Errorf("expected openai (next healthy), got %s", p.Name)
	}
}

func TestSelectProviderAllDown(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	until := time.Now().Add(10 * time.Minute)
	for name := range fc.HealthStatus {
		fc.HealthStatus[name].Status = statusDown
		fc.HealthStatus[name].CooldownUntil = &until
	}

	_, err := fc.SelectProvider()
	if err == nil {
		t.Fatal("expected error when all providers are down")
	}
}

func TestRecordSuccess(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	// Simulate some failures first
	for i := 0; i < 3; i++ {
		fc.RecordFailure("anthropic", fmt.Errorf("timeout"))
	}

	// Now record success
	fc.RecordSuccess("anthropic", 500*time.Millisecond)

	h := fc.HealthStatus["anthropic"]
	if h.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 consecutive failures after success, got %d", h.ConsecutiveFailures)
	}
	if h.Status != statusHealthy {
		t.Errorf("expected healthy status after success, got %s", h.Status)
	}
	if h.Latency != 500*time.Millisecond {
		t.Errorf("expected latency 500ms, got %v", h.Latency)
	}
	if h.CooldownUntil != nil {
		t.Error("expected cooldown to be cleared after success")
	}
	if fc.ActiveProvider != "anthropic" {
		t.Errorf("expected active provider to be anthropic, got %s", fc.ActiveProvider)
	}
}

func TestRecordFailureDegradedThreshold(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	for i := 0; i < degradedThreshold; i++ {
		fc.RecordFailure("anthropic", fmt.Errorf("error %d", i))
	}

	h := fc.HealthStatus["anthropic"]
	if h.Status != statusDegraded {
		t.Errorf("expected degraded status after %d failures, got %s", degradedThreshold, h.Status)
	}
	if h.ConsecutiveFailures != degradedThreshold {
		t.Errorf("expected %d failures, got %d", degradedThreshold, h.ConsecutiveFailures)
	}
}

func TestRecordFailureDownThreshold(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	for i := 0; i < downThreshold; i++ {
		fc.RecordFailure("anthropic", fmt.Errorf("error %d", i))
	}

	h := fc.HealthStatus["anthropic"]
	if h.Status != statusDown {
		t.Errorf("expected down status after %d failures, got %s", downThreshold, h.Status)
	}
	if h.CooldownUntil == nil {
		t.Error("expected cooldown to be set when down")
	}
	if time.Until(*h.CooldownUntil) <= 0 {
		t.Error("expected cooldown to be in the future")
	}
}

func TestGetFallback(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	p, err := fc.GetFallback("anthropic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "openai" {
		t.Errorf("expected openai as fallback for anthropic, got %s", p.Name)
	}

	p, err = fc.GetFallback("openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "ollama" {
		t.Errorf("expected ollama as fallback for openai, got %s", p.Name)
	}
}

func TestGetFallbackWrapsAround(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	// Last provider should wrap around to first
	p, err := fc.GetFallback("ollama")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "anthropic" {
		t.Errorf("expected anthropic as wraparound fallback for ollama, got %s", p.Name)
	}
}

func TestGetFallbackSkipsDown(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	// Mark openai as down with cooldown
	until := time.Now().Add(10 * time.Minute)
	fc.HealthStatus["openai"].Status = statusDown
	fc.HealthStatus["openai"].CooldownUntil = &until

	p, err := fc.GetFallback("anthropic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "ollama" {
		t.Errorf("expected ollama (skipping down openai), got %s", p.Name)
	}
}

func TestGetFallbackNoAvailable(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	until := time.Now().Add(10 * time.Minute)
	fc.HealthStatus["openai"].Status = statusDown
	fc.HealthStatus["openai"].CooldownUntil = &until
	fc.HealthStatus["ollama"].Status = statusDown
	fc.HealthStatus["ollama"].CooldownUntil = &until

	_, err := fc.GetFallback("anthropic")
	if err == nil {
		t.Fatal("expected error when no fallback available")
	}
}

func TestIsHealthy(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	if !fc.IsHealthy("anthropic") {
		t.Error("expected anthropic to be healthy initially")
	}

	// Mark as degraded - should not be considered healthy
	fc.mu.Lock()
	fc.HealthStatus["anthropic"].Status = statusDegraded
	fc.mu.Unlock()

	if fc.IsHealthy("anthropic") {
		t.Error("expected degraded provider to not be healthy")
	}

	// Unknown provider
	if fc.IsHealthy("unknown") {
		t.Error("expected unknown provider to not be healthy")
	}
}

func TestIsHealthyWithCooldown(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	fc.mu.Lock()
	until := time.Now().Add(10 * time.Minute)
	fc.HealthStatus["anthropic"].CooldownUntil = &until
	fc.mu.Unlock()

	if fc.IsHealthy("anthropic") {
		t.Error("expected provider in cooldown to not be healthy")
	}
}

func TestFormatStatus(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	fc.RecordSuccess("anthropic", 1200*time.Millisecond)
	fc.RecordSuccess("openai", 3400*time.Millisecond)

	// Make ollama down
	for i := 0; i < downThreshold; i++ {
		fc.RecordFailure("ollama", fmt.Errorf("connection refused"))
	}

	status := fc.FormatStatus()

	if !strings.Contains(status, "Provider Status:") {
		t.Error("expected status header")
	}
	if !strings.Contains(status, "anthropic") {
		t.Error("expected anthropic in status")
	}
	if !strings.Contains(status, "HEALTHY") {
		t.Error("expected HEALTHY tag")
	}
	if !strings.Contains(status, "DOWN") {
		t.Error("expected DOWN tag for ollama")
	}
	if !strings.Contains(status, "Active:") {
		t.Error("expected Active section")
	}
	if !strings.Contains(status, "Fallback chain:") {
		t.Error("expected Fallback chain section")
	}
	if !strings.Contains(status, "→") {
		t.Error("expected arrow separator in fallback chain")
	}
}

func TestRecoverProvider(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	// Drive provider to down state
	for i := 0; i < downThreshold; i++ {
		fc.RecordFailure("anthropic", fmt.Errorf("error"))
	}

	if fc.IsHealthy("anthropic") {
		t.Fatal("expected provider to be down before recovery")
	}

	fc.RecoverProvider("anthropic")

	if !fc.IsHealthy("anthropic") {
		t.Error("expected provider to be healthy after recovery")
	}

	h := fc.HealthStatus["anthropic"]
	if h.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 failures after recovery, got %d", h.ConsecutiveFailures)
	}
	if h.CooldownUntil != nil {
		t.Error("expected cooldown cleared after recovery")
	}
}

func TestRecoverUnknownProvider(t *testing.T) {
	fc := NewFallbackChain(testProviders())
	// Should not panic
	fc.RecoverProvider("nonexistent")
}

func TestAllDown(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	if fc.AllDown() {
		t.Error("expected AllDown to be false initially")
	}

	// Take all providers down
	until := time.Now().Add(10 * time.Minute)
	fc.mu.Lock()
	for name := range fc.HealthStatus {
		fc.HealthStatus[name].Status = statusDown
		fc.HealthStatus[name].CooldownUntil = &until
	}
	fc.mu.Unlock()

	if !fc.AllDown() {
		t.Error("expected AllDown to be true when all providers are down with cooldown")
	}
}

func TestAllDownReturnsFalseIfOneHealthy(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	until := time.Now().Add(10 * time.Minute)
	fc.mu.Lock()
	fc.HealthStatus["anthropic"].Status = statusDown
	fc.HealthStatus["anthropic"].CooldownUntil = &until
	fc.HealthStatus["openai"].Status = statusDown
	fc.HealthStatus["openai"].CooldownUntil = &until
	// ollama stays healthy
	fc.mu.Unlock()

	if fc.AllDown() {
		t.Error("expected AllDown to be false when ollama is still healthy")
	}
}

func TestBestLatency(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	fc.RecordSuccess("anthropic", 1200*time.Millisecond)
	fc.RecordSuccess("openai", 800*time.Millisecond)
	fc.RecordSuccess("ollama", 2000*time.Millisecond)

	best := fc.BestLatency()
	if best == nil {
		t.Fatal("expected a provider from BestLatency")
	}
	if best.Name != "openai" {
		t.Errorf("expected openai (800ms) as best latency, got %s", best.Name)
	}
}

func TestBestLatencySkipsDown(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	fc.RecordSuccess("anthropic", 1200*time.Millisecond)
	fc.RecordSuccess("openai", 200*time.Millisecond)
	fc.RecordSuccess("ollama", 2000*time.Millisecond)

	// Mark the fastest as down
	fc.mu.Lock()
	fc.HealthStatus["openai"].Status = statusDown
	fc.mu.Unlock()

	best := fc.BestLatency()
	if best == nil {
		t.Fatal("expected a provider from BestLatency")
	}
	if best.Name != "anthropic" {
		t.Errorf("expected anthropic as best latency (openai is down), got %s", best.Name)
	}
}

func TestBestLatencyNoRecorded(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	best := fc.BestLatency()
	if best != nil {
		t.Errorf("expected nil when no latency recorded, got %s", best.Name)
	}
}

func TestCooldownExpiry(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	// Set cooldown in the past (already expired)
	past := time.Now().Add(-1 * time.Second)
	fc.mu.Lock()
	fc.HealthStatus["anthropic"].Status = statusDown
	fc.HealthStatus["anthropic"].CooldownUntil = &past
	fc.mu.Unlock()

	// Provider should be selectable since cooldown expired
	p, err := fc.SelectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "anthropic" {
		t.Errorf("expected anthropic (cooldown expired), got %s", p.Name)
	}
}

func TestConcurrentAccess(t *testing.T) {
	fc := NewFallbackChain(testProviders())
	done := make(chan struct{})

	// Run concurrent operations
	go func() {
		for i := 0; i < 100; i++ {
			fc.RecordSuccess("anthropic", time.Duration(i)*time.Millisecond)
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			fc.RecordFailure("openai", fmt.Errorf("err %d", i))
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_, _ = fc.SelectProvider()
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			fc.IsHealthy("anthropic")
			fc.IsHealthy("openai")
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			fc.AllDown()
			fc.BestLatency()
			fc.FormatStatus()
		}
		done <- struct{}{}
	}()

	for i := 0; i < 5; i++ {
		<-done
	}
}

func TestFallbackChainIntegration(t *testing.T) {
	fc := NewFallbackChain(testProviders())

	// Simulate: primary goes down, system falls back
	// 1. Start with anthropic
	p, _ := fc.SelectProvider()
	if p.Name != "anthropic" {
		t.Fatalf("expected initial provider anthropic, got %s", p.Name)
	}

	// 2. Anthropic starts failing
	for i := 0; i < downThreshold; i++ {
		fc.RecordFailure("anthropic", fmt.Errorf("rate limited"))
	}

	// 3. System should fall back to openai
	p, _ = fc.SelectProvider()
	if p.Name != "openai" {
		t.Errorf("expected fallback to openai, got %s", p.Name)
	}

	// 4. OpenAI also degrades
	for i := 0; i < downThreshold; i++ {
		fc.RecordFailure("openai", fmt.Errorf("server error"))
	}

	// 5. System should use ollama
	p, _ = fc.SelectProvider()
	if p.Name != "ollama" {
		t.Errorf("expected fallback to ollama, got %s", p.Name)
	}

	// 6. Recover anthropic manually
	fc.RecoverProvider("anthropic")
	p, _ = fc.SelectProvider()
	if p.Name != "anthropic" {
		t.Errorf("expected recovered anthropic, got %s", p.Name)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "n/a"},
		{500 * time.Millisecond, "500ms"},
		{1200 * time.Millisecond, "1.2s"},
		{3 * time.Second, "3.0s"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
