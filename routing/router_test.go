package routing

import (
	"testing"
	"time"
)

func TestNewRouter(t *testing.T) {
	r := NewRouter(StrategyBalanced)
	if r == nil {
		t.Fatal("expected non-nil router")
	}
	if len(r.fallbackChain) == 0 {
		t.Error("expected non-empty fallback chain")
	}
}

func TestRouter_SelectProvider_Preferred(t *testing.T) {
	r := NewRouter(StrategyLatency)

	provider, err := r.SelectProvider("anthropic")
	if err != nil {
		t.Fatalf("SelectProvider error: %v", err)
	}
	if provider != "anthropic" {
		t.Errorf("expected anthropic, got %s", provider)
	}
}

func TestRouter_SelectProvider_Fallback(t *testing.T) {
	r := NewRouter(StrategyLatency)

	// Mark preferred as down
	for i := 0; i < 3; i++ {
		r.RecordFailure("anthropic", nil)
	}

	provider, err := r.SelectProvider("anthropic")
	if err != nil {
		t.Fatalf("SelectProvider error: %v", err)
	}
	if provider == "anthropic" {
		t.Error("should have fallen back from anthropic")
	}
	if provider != "openai" {
		t.Errorf("expected openai as first fallback, got %s", provider)
	}
}

func TestRouter_CircuitBreaker(t *testing.T) {
	r := NewRouter(StrategyLatency)

	// Record failures to open circuit
	for i := 0; i < 3; i++ {
		r.RecordFailure("anthropic", nil)
	}

	health := r.HealthStatus()
	if h, ok := health["anthropic"]; ok {
		if h.Available {
			t.Error("provider should be unavailable after circuit opens")
		}
		if h.ConsecutiveFails != 3 {
			t.Errorf("expected 3 consecutive fails, got %d", h.ConsecutiveFails)
		}
	} else {
		t.Error("expected health entry for anthropic")
	}
}

func TestRouter_CircuitBreaker_Recovery(t *testing.T) {
	r := NewRouter(StrategyLatency)

	// Open circuit
	for i := 0; i < 3; i++ {
		r.RecordFailure("anthropic", nil)
	}

	// Record success should close circuit
	r.RecordSuccess("anthropic", 100*time.Millisecond)

	health := r.HealthStatus()
	if h := health["anthropic"]; h != nil {
		if !h.Available {
			t.Error("provider should be available after recovery")
		}
		if h.ConsecutiveFails != 0 {
			t.Errorf("expected 0 fails after recovery, got %d", h.ConsecutiveFails)
		}
	}
}

func TestRouter_RecordSuccess_Latency(t *testing.T) {
	r := NewRouter(StrategyLatency)

	r.RecordSuccess("anthropic", 200*time.Millisecond)
	r.RecordSuccess("anthropic", 100*time.Millisecond)

	health := r.HealthStatus()
	h := health["anthropic"]
	if h == nil {
		t.Fatal("expected health entry")
	}
	if h.AvgLatencyMs == 0 {
		t.Error("expected non-zero avg latency")
	}
	// EMA should be between 100 and 200
	if h.AvgLatencyMs < 100 || h.AvgLatencyMs > 200 {
		t.Errorf("EMA should be between 100 and 200, got %f", h.AvgLatencyMs)
	}
}

func TestRouter_Score(t *testing.T) {
	r := NewRouter(StrategyLatency)

	r.RecordSuccess("anthropic", 100*time.Millisecond)
	r.RecordSuccess("openai", 500*time.Millisecond)

	scoreA := r.Score("anthropic")
	scoreO := r.Score("openai")
	if scoreA >= scoreO {
		t.Errorf("anthropic (faster) should have lower score: %f vs %f", scoreA, scoreO)
	}
}

func TestRouter_Score_WithFailures(t *testing.T) {
	r := NewRouter(StrategyLatency)

	r.RecordSuccess("anthropic", 100*time.Millisecond)
	r.RecordFailure("anthropic", nil)
	r.RecordFailure("anthropic", nil)

	scoreA := r.Score("anthropic")
	if scoreA <= 10 {
		t.Errorf("score should include failure penalty, got %f", scoreA)
	}
}

func TestRouter_SelectProviderForModel(t *testing.T) {
	r := NewRouter(StrategyBalanced)

	provider, info, err := r.SelectProviderForModel("gpt-4o")
	if err != nil {
		t.Fatalf("SelectProviderForModel error: %v", err)
	}
	if provider != "openai" {
		t.Errorf("expected openai, got %s", provider)
	}
	if info.Name != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", info.Name)
	}
}

func TestRouter_SelectProviderForModel_Unknown(t *testing.T) {
	r := NewRouter(StrategyBalanced)

	_, _, err := r.SelectProviderForModel("nonexistent-model")
	if err == nil {
		t.Error("expected error for unknown model")
	}
}

func TestRouter_SetFallbackChain(t *testing.T) {
	r := NewRouter(StrategyLatency)
	r.SetFallbackChain([]string{"gemini", "openai"})

	// Mark preferred as down
	for i := 0; i < 3; i++ {
		r.RecordFailure("anthropic", nil)
	}

	provider, _ := r.SelectProvider("anthropic")
	if provider != "gemini" {
		t.Errorf("expected gemini as first fallback after chain change, got %s", provider)
	}
}
