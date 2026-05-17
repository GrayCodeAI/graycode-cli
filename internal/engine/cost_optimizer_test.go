package engine

import (
	"strings"
	"testing"
	"time"
)

func TestNewCostOptimizer(t *testing.T) {
	co := NewCostOptimizer()
	if co == nil {
		t.Fatal("NewCostOptimizer returned nil")
	}
	if len(co.History) != 0 {
		t.Errorf("expected empty history, got %d entries", len(co.History))
	}
	if len(co.Recommendations) != 0 {
		t.Errorf("expected empty recommendations, got %d", len(co.Recommendations))
	}
	if len(co.ModelPricing) != 5 {
		t.Errorf("expected 5 model pricings, got %d", len(co.ModelPricing))
	}

	// Verify specific pricing
	opus, ok := co.ModelPricing["claude-opus"]
	if !ok {
		t.Fatal("missing claude-opus pricing")
	}
	if opus.InputPerMillion != 15.0 {
		t.Errorf("opus input price: want 15.0, got %f", opus.InputPerMillion)
	}
	if opus.OutputPerMillion != 75.0 {
		t.Errorf("opus output price: want 75.0, got %f", opus.OutputPerMillion)
	}

	haiku, ok := co.ModelPricing["claude-haiku"]
	if !ok {
		t.Fatal("missing claude-haiku pricing")
	}
	if haiku.InputPerMillion != 0.25 {
		t.Errorf("haiku input price: want 0.25, got %f", haiku.InputPerMillion)
	}
	if haiku.OutputPerMillion != 1.25 {
		t.Errorf("haiku output price: want 1.25, got %f", haiku.OutputPerMillion)
	}

	mini, ok := co.ModelPricing["gpt-4o-mini"]
	if !ok {
		t.Fatal("missing gpt-4o-mini pricing")
	}
	if mini.InputPerMillion != 0.15 {
		t.Errorf("gpt-4o-mini input price: want 0.15, got %f", mini.InputPerMillion)
	}
	if mini.OutputPerMillion != 0.60 {
		t.Errorf("gpt-4o-mini output price: want 0.60, got %f", mini.OutputPerMillion)
	}
}

func TestRecord(t *testing.T) {
	co := NewCostOptimizer()

	cost := RequestCost{
		Model:        "claude-sonnet-4-6",
		Provider:     "anthropic",
		InputTokens:  1000,
		OutputTokens: 500,
		CostUSD:      0.01,
		TaskType:     "code",
		Duration:     2 * time.Second,
		CacheHit:     false,
		Timestamp:    time.Now(),
	}

	co.Record(cost)

	if len(co.History) != 1 {
		t.Fatalf("expected 1 entry in history, got %d", len(co.History))
	}
	if co.History[0].Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %s", co.History[0].Model)
	}
	if co.History[0].CostUSD != 0.01 {
		t.Errorf("expected cost 0.01, got %f", co.History[0].CostUSD)
	}
}

func TestDailyCost(t *testing.T) {
	co := NewCostOptimizer()

	now := time.Now()
	// Recent entries (within 24h)
	co.Record(RequestCost{CostUSD: 1.00, Timestamp: now.Add(-1 * time.Hour)})
	co.Record(RequestCost{CostUSD: 2.00, Timestamp: now.Add(-12 * time.Hour)})
	// Old entry (more than 24h ago)
	co.Record(RequestCost{CostUSD: 5.00, Timestamp: now.Add(-48 * time.Hour)})

	daily := co.DailyCost()
	if daily != 3.00 {
		t.Errorf("expected daily cost 3.00, got %f", daily)
	}
}

func TestWeeklyCost(t *testing.T) {
	co := NewCostOptimizer()

	now := time.Now()
	// Within 7 days
	co.Record(RequestCost{CostUSD: 1.00, Timestamp: now.Add(-1 * time.Hour)})
	co.Record(RequestCost{CostUSD: 2.00, Timestamp: now.Add(-3 * 24 * time.Hour)})
	co.Record(RequestCost{CostUSD: 3.00, Timestamp: now.Add(-6 * 24 * time.Hour)})
	// Older than 7 days
	co.Record(RequestCost{CostUSD: 10.00, Timestamp: now.Add(-10 * 24 * time.Hour)})

	weekly := co.WeeklyCost()
	if weekly != 6.00 {
		t.Errorf("expected weekly cost 6.00, got %f", weekly)
	}
}

func TestCostByModel(t *testing.T) {
	co := NewCostOptimizer()

	co.Record(RequestCost{Model: "claude-sonnet-4-6", CostUSD: 2.00, Timestamp: time.Now()})
	co.Record(RequestCost{Model: "claude-sonnet-4-6", CostUSD: 3.00, Timestamp: time.Now()})
	co.Record(RequestCost{Model: "claude-haiku-4-5", CostUSD: 0.50, Timestamp: time.Now()})
	co.Record(RequestCost{Model: "gpt-4o-mini", CostUSD: 0.10, Timestamp: time.Now()})

	result := co.CostByModel()

	if result["claude-sonnet-4-6"] != 5.00 {
		t.Errorf("expected sonnet cost 5.00, got %f", result["claude-sonnet-4-6"])
	}
	if result["claude-haiku-4-5"] != 0.50 {
		t.Errorf("expected haiku cost 0.50, got %f", result["claude-haiku-4-5"])
	}
	if result["gpt-4o-mini"] != 0.10 {
		t.Errorf("expected gpt-4o-mini cost 0.10, got %f", result["gpt-4o-mini"])
	}
}

func TestCostByTaskType(t *testing.T) {
	co := NewCostOptimizer()

	co.Record(RequestCost{TaskType: "code", CostUSD: 5.00, Timestamp: time.Now()})
	co.Record(RequestCost{TaskType: "code", CostUSD: 2.00, Timestamp: time.Now()})
	co.Record(RequestCost{TaskType: "chat", CostUSD: 1.00, Timestamp: time.Now()})
	co.Record(RequestCost{TaskType: "review", CostUSD: 3.00, Timestamp: time.Now()})

	result := co.CostByTaskType()

	if result["code"] != 7.00 {
		t.Errorf("expected code cost 7.00, got %f", result["code"])
	}
	if result["chat"] != 1.00 {
		t.Errorf("expected chat cost 1.00, got %f", result["chat"])
	}
	if result["review"] != 3.00 {
		t.Errorf("expected review cost 3.00, got %f", result["review"])
	}
}

func TestProjectSavings(t *testing.T) {
	co := NewCostOptimizer()

	recs := []Recommendation{
		{EstimatedSavings: 2.10},
		{EstimatedSavings: 0.80},
		{EstimatedSavings: 0.30},
	}

	total := co.ProjectSavings(recs)
	expected := 3.20
	if total != expected {
		t.Errorf("expected savings %.2f, got %.2f", expected, total)
	}
}

func TestWhatIf(t *testing.T) {
	co := NewCostOptimizer()

	co.Record(RequestCost{
		Model:        "claude-sonnet-4-6",
		InputTokens:  1_000_000,
		OutputTokens: 100_000,
		CostUSD:      4.50,
		Timestamp:    time.Now(),
	})
	co.Record(RequestCost{
		Model:        "claude-sonnet-4-6",
		InputTokens:  500_000,
		OutputTokens: 50_000,
		CostUSD:      2.25,
		Timestamp:    time.Now(),
	})

	// What if we used haiku instead?
	haikuCost := co.WhatIf("claude-haiku")
	// 1.5M input * 0.25/M + 150K output * 1.25/M = 0.375 + 0.1875 = 0.5625
	expectedHaiku := 0.5625
	if abs(haikuCost-expectedHaiku) > 0.001 {
		t.Errorf("WhatIf haiku: expected %.4f, got %.4f", expectedHaiku, haikuCost)
	}

	// What if we used gpt-4o?
	gpt4oCost := co.WhatIf("gpt-4o")
	// 1.5M input * 2.50/M + 150K output * 10.0/M = 3.75 + 1.50 = 5.25
	expectedGPT := 5.25
	if abs(gpt4oCost-expectedGPT) > 0.001 {
		t.Errorf("WhatIf gpt-4o: expected %.4f, got %.4f", expectedGPT, gpt4oCost)
	}
}

func TestAnalyzeModelDowngrade(t *testing.T) {
	co := NewCostOptimizer()

	now := time.Now()
	// Simulate simple tasks on expensive models
	for i := 0; i < 10; i++ {
		co.Record(RequestCost{
			Model:        "claude-opus-4",
			TaskType:     "chat",
			InputTokens:  500,
			OutputTokens: 200,
			CostUSD:      0.50,
			Timestamp:    now.Add(-time.Duration(i) * time.Hour),
		})
	}

	recs := co.Analyze()

	found := false
	for _, r := range recs {
		if r.Type == "model_switch" {
			found = true
			if r.Priority != "high" {
				t.Errorf("model_switch should be high priority, got %s", r.Priority)
			}
			if r.EstimatedSavings <= 0 {
				t.Error("model_switch should have positive savings")
			}
		}
	}
	if !found {
		t.Error("expected model_switch recommendation for chat tasks on opus")
	}
}

func TestAnalyzeCaching(t *testing.T) {
	co := NewCostOptimizer()

	now := time.Now()
	// Many requests with no cache hits
	for i := 0; i < 20; i++ {
		co.Record(RequestCost{
			Model:        "claude-sonnet-4-6",
			TaskType:     "code",
			InputTokens:  3000,
			OutputTokens: 1000,
			CostUSD:      0.10,
			CacheHit:     false,
			Timestamp:    now.Add(-time.Duration(i) * time.Hour),
		})
	}

	recs := co.Analyze()

	found := false
	for _, r := range recs {
		if r.Type == "caching" {
			found = true
			if r.Priority != "medium" {
				t.Errorf("caching should be medium priority, got %s", r.Priority)
			}
		}
	}
	if !found {
		t.Error("expected caching recommendation for low cache hit rate")
	}
}

func TestAnalyzeCompression(t *testing.T) {
	co := NewCostOptimizer()

	now := time.Now()
	// Requests with large input tokens
	for i := 0; i < 10; i++ {
		co.Record(RequestCost{
			Model:        "claude-sonnet-4-6",
			TaskType:     "review",
			InputTokens:  8000,
			OutputTokens: 2000,
			CostUSD:      0.30,
			CacheHit:     true,
			Timestamp:    now.Add(-time.Duration(i) * time.Hour),
		})
	}

	recs := co.Analyze()

	found := false
	for _, r := range recs {
		if r.Type == "compression" && strings.Contains(r.Description, "5K tokens") {
			found = true
			if r.Priority != "low" {
				t.Errorf("compression should be low priority, got %s", r.Priority)
			}
		}
	}
	if !found {
		t.Error("expected compression recommendation for large inputs")
	}
}

func TestAnalyzeBatching(t *testing.T) {
	co := NewCostOptimizer()

	now := time.Now()
	// Many small sequential requests within short time windows
	for i := 0; i < 20; i++ {
		co.Record(RequestCost{
			Model:        "gpt-4o-mini",
			TaskType:     "chat",
			InputTokens:  200,
			OutputTokens: 100,
			CostUSD:      0.01,
			Timestamp:    now.Add(-time.Duration(i*10) * time.Second),
		})
	}

	recs := co.Analyze()

	found := false
	for _, r := range recs {
		if r.Type == "batching" && strings.Contains(r.Description, "sequential") {
			found = true
		}
	}
	if !found {
		t.Error("expected batching recommendation for small sequential requests")
	}
}

func TestAnalyzeTokenReduction(t *testing.T) {
	co := NewCostOptimizer()

	now := time.Now()
	// Requests with consistently high output tokens
	for i := 0; i < 10; i++ {
		co.Record(RequestCost{
			Model:        "claude-sonnet-4-6",
			TaskType:     "code",
			InputTokens:  2000,
			OutputTokens: 5000,
			CostUSD:      0.20,
			CacheHit:     true,
			Timestamp:    now.Add(-time.Duration(i) * time.Hour),
		})
	}

	recs := co.Analyze()

	found := false
	for _, r := range recs {
		if r.Type == "compression" && strings.Contains(r.Description, "output tokens") {
			found = true
			if r.Priority != "medium" {
				t.Errorf("token reduction should be medium priority, got %s", r.Priority)
			}
		}
	}
	if !found {
		t.Error("expected token reduction recommendation for high output tokens")
	}
}

func TestAnalyzeEmpty(t *testing.T) {
	co := NewCostOptimizer()
	recs := co.Analyze()
	if len(recs) != 0 {
		t.Errorf("expected no recommendations for empty history, got %d", len(recs))
	}
}

func TestFormatReport(t *testing.T) {
	co := NewCostOptimizer()

	now := time.Now()
	co.Record(RequestCost{Model: "claude-sonnet-4-6", TaskType: "code", CostUSD: 8.20, Timestamp: now.Add(-1 * time.Hour), InputTokens: 5000, OutputTokens: 2000})
	co.Record(RequestCost{Model: "claude-haiku-4-5", TaskType: "review", CostUSD: 3.10, Timestamp: now.Add(-2 * time.Hour), InputTokens: 3000, OutputTokens: 1000})
	co.Record(RequestCost{Model: "gpt-4o-mini", TaskType: "chat", CostUSD: 1.15, Timestamp: now.Add(-3 * time.Hour), InputTokens: 500, OutputTokens: 200})

	// Run analyze to populate recommendations
	co.Analyze()

	report := co.FormatReport()

	if !strings.Contains(report, "Cost Report (Last 7 Days):") {
		t.Error("report should contain header")
	}
	if !strings.Contains(report, "Total: $12.45") {
		t.Errorf("report should contain total, got: %s", report)
	}
	if !strings.Contains(report, "By Model:") {
		t.Error("report should contain By Model section")
	}
	if !strings.Contains(report, "claude-sonnet-4-6") {
		t.Error("report should mention claude-sonnet-4-6")
	}
	if !strings.Contains(report, "By Task:") {
		t.Error("report should contain By Task section")
	}
	if !strings.Contains(report, "code") {
		t.Error("report should mention code task")
	}
}

func TestFormatReportEmpty(t *testing.T) {
	co := NewCostOptimizer()
	report := co.FormatReport()
	if !strings.Contains(report, "Total: $0.00") {
		t.Errorf("empty report should show $0.00, got: %s", report)
	}
}

func TestWhatIfAllModels(t *testing.T) {
	co := NewCostOptimizer()

	now := time.Now()
	co.Record(RequestCost{
		Model:        "claude-opus-4",
		InputTokens:  100_000,
		OutputTokens: 10_000,
		CostUSD:      2.25,
		Timestamp:    now,
	})

	// What if all on haiku: 100K * 0.25/M + 10K * 1.25/M = 0.025 + 0.0125 = 0.0375
	haikuCost := co.WhatIf("claude-haiku")
	if abs(haikuCost-0.0375) > 0.001 {
		t.Errorf("WhatIf haiku: expected 0.0375, got %f", haikuCost)
	}

	// What if gpt-4o-mini: 100K * 0.15/M + 10K * 0.60/M = 0.015 + 0.006 = 0.021
	miniCost := co.WhatIf("gpt-4o-mini")
	if abs(miniCost-0.021) > 0.001 {
		t.Errorf("WhatIf gpt-4o-mini: expected 0.021, got %f", miniCost)
	}
}

func TestCostOptimizerConcurrentAccess(t *testing.T) {
	co := NewCostOptimizer()
	done := make(chan struct{})

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			co.Record(RequestCost{
				Model:        "claude-sonnet-4-6",
				TaskType:     "code",
				InputTokens:  1000,
				OutputTokens: 500,
				CostUSD:      0.01,
				Timestamp:    time.Now(),
			})
		}
		done <- struct{}{}
	}()

	// Reader goroutines
	go func() {
		for i := 0; i < 50; i++ {
			co.DailyCost()
			co.WeeklyCost()
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 50; i++ {
			co.CostByModel()
			co.CostByTaskType()
		}
		done <- struct{}{}
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done
}

func TestNormalizeModel(t *testing.T) {
	co := NewCostOptimizer()

	tests := []struct {
		input    string
		expected string
	}{
		{"claude-opus-4", "claude-opus"},
		{"claude-sonnet-4-6", "claude-sonnet"},
		{"claude-haiku-4-5", "claude-haiku"},
		{"gpt-4o", "gpt-4o"},
		{"gpt-4o-mini", "gpt-4o-mini"},
		{"unknown-model", "unknown-model"},
	}

	for _, tt := range tests {
		result := co.normalizeModel(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeModel(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestGetPricing(t *testing.T) {
	co := NewCostOptimizer()

	// Known model
	p := co.getPricing("claude-opus-4")
	if p.InputPerMillion != 15.0 {
		t.Errorf("opus input: expected 15.0, got %f", p.InputPerMillion)
	}

	// Unknown model falls back to sonnet
	p = co.getPricing("unknown-model-xyz")
	if p.InputPerMillion != 3.0 {
		t.Errorf("unknown fallback input: expected 3.0, got %f", p.InputPerMillion)
	}
}

func TestHistoryDays(t *testing.T) {
	co := NewCostOptimizer()

	// Empty history
	days := co.historyDays()
	if days != 1 {
		t.Errorf("empty history days: expected 1, got %f", days)
	}

	// Single entry
	now := time.Now()
	co.History = append(co.History, RequestCost{Timestamp: now})
	days = co.historyDays()
	if days != 1 {
		t.Errorf("single entry days: expected 1, got %f", days)
	}

	// Multi-day spread
	co.History = append(co.History, RequestCost{Timestamp: now.Add(-72 * time.Hour)})
	days = co.historyDays()
	if days != 3.0 {
		t.Errorf("3-day spread: expected 3.0, got %f", days)
	}
}

func TestProjectSavingsEmpty(t *testing.T) {
	co := NewCostOptimizer()
	total := co.ProjectSavings(nil)
	if total != 0 {
		t.Errorf("expected 0 savings for nil recommendations, got %f", total)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
