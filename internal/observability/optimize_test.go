package analytics

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestClassifyTask_Simple(t *testing.T) {
	tests := []struct {
		prompt   string
		response string
	}{
		{"Is this correct?", "Yes."},
		{"Which port?", "Port 8080."},
		{"How many items?", "There are 42 items in the list."},
		{"Status?", "All systems operational."},
	}
	for _, tt := range tests {
		got := ClassifyTask(tt.prompt, tt.response)
		if got != "simple" {
			t.Errorf("ClassifyTask(%q, %q) = %q, want simple", tt.prompt, tt.response, got)
		}
	}
}

func TestClassifyTask_Generation(t *testing.T) {
	prompt := "implement a function that sorts a list"
	response := "Here's the implementation:\n```go\nfunc Sort(items []int) []int {\n\tsort.Ints(items)\n\treturn items\n}\n```"
	got := ClassifyTask(prompt, response)
	if got != "generation" {
		t.Errorf("ClassifyTask(generation prompt) = %q, want generation", got)
	}
}

func TestClassifyTask_Debug(t *testing.T) {
	tests := []struct {
		prompt string
	}{
		{"fix the bug in the auth handler"},
		{"debug this error: nil pointer dereference"},
		{"this function is broken, can you look at it"},
		{"the test is failing with a panic"},
	}
	for _, tt := range tests {
		response := "The issue is in line 42. Here's the fix:\n```go\nif err != nil {\n\treturn err\n}\n```"
		got := ClassifyTask(tt.prompt, response)
		if got != "debug" {
			t.Errorf("ClassifyTask(%q, ...) = %q, want debug", tt.prompt, got)
		}
	}
}

func TestClassifyTask_Refactor(t *testing.T) {
	prompt := "refactor this handler to use dependency injection"
	response := "Here's the refactored version:\n```go\ntype Handler struct {\n\tdb DB\n}\n```\nMore code follows..."
	got := ClassifyTask(prompt, response)
	if got != "refactor" {
		t.Errorf("ClassifyTask(refactor prompt) = %q, want refactor", got)
	}
}

func TestClassifyTask_Review(t *testing.T) {
	prompt := "review this pull request for issues"
	response := "I found several issues: 1) Missing error handling on line 10. 2) Race condition in the cache access. The overall structure looks good but needs the above fixes."
	got := ClassifyTask(prompt, response)
	if got != "review" {
		t.Errorf("ClassifyTask(review prompt) = %q, want review", got)
	}
}

func TestClassifyTask_Chat(t *testing.T) {
	prompt := "explain how goroutines work"
	response := "Goroutines are lightweight threads managed by the Go runtime. They are multiplexed onto OS threads and are very cheap to create. Here's how they work in detail with scheduling, stack management, and communication patterns."
	got := ClassifyTask(prompt, response)
	if got != "chat" {
		t.Errorf("ClassifyTask(chat prompt) = %q, want chat", got)
	}
}

func TestSuggestModel_Simple(t *testing.T) {
	got := SuggestModel("simple", "opus")
	if got != "haiku" {
		t.Errorf("SuggestModel(simple) = %q, want haiku", got)
	}
}

func TestSuggestModel_Generation(t *testing.T) {
	got := SuggestModel("generation", "sonnet")
	if got != "opus" {
		t.Errorf("SuggestModel(generation) = %q, want opus", got)
	}
}

func TestSuggestModel_Chat(t *testing.T) {
	got := SuggestModel("chat", "opus")
	if got != "sonnet" {
		t.Errorf("SuggestModel(chat) = %q, want sonnet", got)
	}
}

func TestSuggestModel_Debug(t *testing.T) {
	got := SuggestModel("debug", "opus")
	if got != "sonnet" {
		t.Errorf("SuggestModel(debug) = %q, want sonnet", got)
	}
}

func TestSuggestModel_Review(t *testing.T) {
	got := SuggestModel("review", "opus")
	if got != "sonnet" {
		t.Errorf("SuggestModel(review) = %q, want sonnet", got)
	}
}

func TestSuggestModel_Refactor(t *testing.T) {
	got := SuggestModel("refactor", "opus")
	if got != "sonnet" {
		t.Errorf("SuggestModel(refactor) = %q, want sonnet", got)
	}
}

func TestSuggestModel_Unknown(t *testing.T) {
	got := SuggestModel("unknown_type", "opus")
	if got != "sonnet" {
		t.Errorf("SuggestModel(unknown) = %q, want sonnet", got)
	}
}

func TestAnalyze_Empty(t *testing.T) {
	report := Analyze(nil)
	if report.TotalSpend != 0 {
		t.Errorf("empty: TotalSpend = %f, want 0", report.TotalSpend)
	}
	if report.YieldRate != 0 {
		t.Errorf("empty: YieldRate = %f, want 0", report.YieldRate)
	}
	if len(report.Recommendations) != 0 {
		t.Errorf("empty: got %d recommendations, want 0", len(report.Recommendations))
	}
	if len(report.ByModel) != 0 {
		t.Errorf("empty: got %d models, want 0", len(report.ByModel))
	}
	if len(report.ByTaskType) != 0 {
		t.Errorf("empty: got %d task types, want 0", len(report.ByTaskType))
	}
}

func TestAnalyze_YieldRate(t *testing.T) {
	entries := []CostEntry{
		{Model: "sonnet", TaskType: "generation", CostUSD: 0.10, Kept: true, InputTokens: 500, OutputTokens: 500},
		{Model: "sonnet", TaskType: "generation", CostUSD: 0.10, Kept: true, InputTokens: 500, OutputTokens: 500},
		{Model: "sonnet", TaskType: "generation", CostUSD: 0.10, Kept: false, InputTokens: 500, OutputTokens: 500},
		{Model: "sonnet", TaskType: "debug", CostUSD: 0.10, Kept: false, InputTokens: 500, OutputTokens: 500},
	}

	report := Analyze(entries)

	if report.TotalSpend != 0.40 {
		t.Errorf("TotalSpend = %f, want 0.40", report.TotalSpend)
	}
	if report.ProductiveSpend != 0.20 {
		t.Errorf("ProductiveSpend = %f, want 0.20", report.ProductiveSpend)
	}
	if report.AbandonedSpend != 0.20 {
		t.Errorf("AbandonedSpend = %f, want 0.20", report.AbandonedSpend)
	}
	if math.Abs(report.YieldRate-0.50) > 0.001 {
		t.Errorf("YieldRate = %f, want 0.50", report.YieldRate)
	}
}

func TestAnalyze_WastedSpend(t *testing.T) {
	entries := []CostEntry{
		// Simple task on expensive model = wasted.
		{Model: "opus", TaskType: "simple", CostUSD: 0.50, Kept: true, InputTokens: 100, OutputTokens: 20},
		// Simple task on cheap model = not wasted.
		{Model: "haiku", TaskType: "simple", CostUSD: 0.001, Kept: true, InputTokens: 100, OutputTokens: 20},
		// Complex task on expensive model = not wasted.
		{Model: "opus", TaskType: "generation", CostUSD: 1.00, Kept: true, InputTokens: 2000, OutputTokens: 3000},
	}

	report := Analyze(entries)

	if math.Abs(report.WastedSpend-0.50) > 0.001 {
		t.Errorf("WastedSpend = %f, want 0.50", report.WastedSpend)
	}
}

func TestAnalyze_ByModel(t *testing.T) {
	entries := []CostEntry{
		{Model: "opus", TaskType: "generation", CostUSD: 1.00, Kept: true, InputTokens: 1000, OutputTokens: 2000},
		{Model: "opus", TaskType: "debug", CostUSD: 0.50, Kept: true, InputTokens: 500, OutputTokens: 1000},
		{Model: "sonnet", TaskType: "simple", CostUSD: 0.05, Kept: true, InputTokens: 200, OutputTokens: 100},
	}

	report := Analyze(entries)

	if len(report.ByModel) != 2 {
		t.Fatalf("expected 2 models, got %d", len(report.ByModel))
	}

	opus := report.ByModel["opus"]
	if opus == nil {
		t.Fatal("missing opus model")
	}
	if opus.Calls != 2 {
		t.Errorf("opus calls = %d, want 2", opus.Calls)
	}
	if math.Abs(opus.TotalCost-1.50) > 0.001 {
		t.Errorf("opus total cost = %f, want 1.50", opus.TotalCost)
	}

	sonnet := report.ByModel["sonnet"]
	if sonnet == nil {
		t.Fatal("missing sonnet model")
	}
	if sonnet.Calls != 1 {
		t.Errorf("sonnet calls = %d, want 1", sonnet.Calls)
	}
}

func TestAnalyze_ByTaskType(t *testing.T) {
	entries := []CostEntry{
		{Model: "sonnet", TaskType: "simple", CostUSD: 0.01, Kept: true, InputTokens: 50, OutputTokens: 20},
		{Model: "sonnet", TaskType: "simple", CostUSD: 0.01, Kept: true, InputTokens: 50, OutputTokens: 20},
		{Model: "opus", TaskType: "generation", CostUSD: 1.00, Kept: true, InputTokens: 2000, OutputTokens: 3000},
	}

	report := Analyze(entries)

	if len(report.ByTaskType) != 2 {
		t.Fatalf("expected 2 task types, got %d", len(report.ByTaskType))
	}

	simple := report.ByTaskType["simple"]
	if simple == nil {
		t.Fatal("missing simple task type")
	}
	if simple.Calls != 2 {
		t.Errorf("simple calls = %d, want 2", simple.Calls)
	}
	if simple.SuggestedModel != "haiku" {
		t.Errorf("simple suggested model = %q, want haiku", simple.SuggestedModel)
	}

	gen := report.ByTaskType["generation"]
	if gen == nil {
		t.Fatal("missing generation task type")
	}
	if gen.SuggestedModel != "opus" {
		t.Errorf("generation suggested model = %q, want opus", gen.SuggestedModel)
	}
}

func TestAnalyze_Recommendations_DowngradeSimple(t *testing.T) {
	entries := []CostEntry{
		{Model: "opus", TaskType: "simple", CostUSD: 0.50, Kept: true, InputTokens: 100, OutputTokens: 20},
		{Model: "opus", TaskType: "simple", CostUSD: 0.50, Kept: true, InputTokens: 100, OutputTokens: 20},
	}

	report := Analyze(entries)

	found := false
	for _, rec := range report.Recommendations {
		if rec.Type == "downgrade" && strings.Contains(rec.Description, "simple tasks") {
			found = true
			if rec.Savings <= 0 {
				t.Errorf("expected positive savings, got %f", rec.Savings)
			}
		}
	}
	if !found {
		t.Error("expected a downgrade recommendation for simple tasks on expensive models")
	}
}

func TestAnalyze_Recommendations_AbandonedSpend(t *testing.T) {
	entries := []CostEntry{
		{Model: "sonnet", TaskType: "generation", CostUSD: 1.00, Kept: false, InputTokens: 2000, OutputTokens: 3000},
		{Model: "sonnet", TaskType: "generation", CostUSD: 1.00, Kept: false, InputTokens: 2000, OutputTokens: 3000},
		{Model: "sonnet", TaskType: "generation", CostUSD: 1.00, Kept: false, InputTokens: 2000, OutputTokens: 3000},
		{Model: "sonnet", TaskType: "generation", CostUSD: 0.10, Kept: true, InputTokens: 200, OutputTokens: 300},
	}

	report := Analyze(entries)

	// Abandoned rate > 20%, so should trigger batch recommendation.
	found := false
	for _, rec := range report.Recommendations {
		if rec.Type == "batch" && strings.Contains(rec.Description, "abandoned") {
			found = true
		}
	}
	if !found {
		t.Error("expected a batch recommendation for high abandoned spend")
	}
}

func TestAnalyze_Recommendations_SortedBySavings(t *testing.T) {
	entries := []CostEntry{
		// Simple on expensive: should generate downgrade rec.
		{Model: "opus", TaskType: "simple", CostUSD: 2.00, Kept: true, InputTokens: 100, OutputTokens: 20},
		// Abandoned: should generate batch rec.
		{Model: "sonnet", TaskType: "generation", CostUSD: 5.00, Kept: false, InputTokens: 5000, OutputTokens: 5000},
		{Model: "sonnet", TaskType: "generation", CostUSD: 0.50, Kept: true, InputTokens: 500, OutputTokens: 500},
	}

	report := Analyze(entries)

	if len(report.Recommendations) < 2 {
		t.Fatalf("expected at least 2 recommendations, got %d", len(report.Recommendations))
	}

	// Verify sorted by savings descending.
	for i := 1; i < len(report.Recommendations); i++ {
		if report.Recommendations[i].Savings > report.Recommendations[i-1].Savings {
			t.Errorf("recommendations not sorted by savings: [%d].Savings=%f > [%d].Savings=%f",
				i, report.Recommendations[i].Savings, i-1, report.Recommendations[i-1].Savings)
		}
	}
}

func TestAnalyze_MixedEntries(t *testing.T) {
	now := time.Now()
	entries := []CostEntry{
		{SessionID: "s1", Model: "opus", TaskType: "simple", InputTokens: 100, OutputTokens: 20, CostUSD: 0.50, Kept: true, Timestamp: now, Duration: time.Second},
		{SessionID: "s1", Model: "opus", TaskType: "generation", InputTokens: 2000, OutputTokens: 5000, CostUSD: 3.00, Kept: true, Timestamp: now, Duration: 5 * time.Second},
		{SessionID: "s1", Model: "sonnet", TaskType: "debug", InputTokens: 1000, OutputTokens: 2000, CostUSD: 0.30, Kept: false, Timestamp: now, Duration: 3 * time.Second},
		{SessionID: "s2", Model: "haiku", TaskType: "simple", InputTokens: 50, OutputTokens: 10, CostUSD: 0.001, Kept: true, Timestamp: now, Duration: 500 * time.Millisecond},
		{SessionID: "s2", Model: "sonnet", TaskType: "chat", InputTokens: 500, OutputTokens: 800, CostUSD: 0.15, Kept: true, Timestamp: now, Duration: 2 * time.Second},
		{SessionID: "s2", Model: "gpt-4", TaskType: "review", InputTokens: 3000, OutputTokens: 1500, CostUSD: 1.20, Kept: false, Timestamp: now, Duration: 4 * time.Second},
	}

	report := Analyze(entries)

	expectedTotal := 0.50 + 3.00 + 0.30 + 0.001 + 0.15 + 1.20
	if math.Abs(report.TotalSpend-expectedTotal) > 0.001 {
		t.Errorf("TotalSpend = %f, want %f", report.TotalSpend, expectedTotal)
	}

	expectedProductive := 0.50 + 3.00 + 0.001 + 0.15
	if math.Abs(report.ProductiveSpend-expectedProductive) > 0.001 {
		t.Errorf("ProductiveSpend = %f, want %f", report.ProductiveSpend, expectedProductive)
	}

	expectedAbandoned := 0.30 + 1.20
	if math.Abs(report.AbandonedSpend-expectedAbandoned) > 0.001 {
		t.Errorf("AbandonedSpend = %f, want %f", report.AbandonedSpend, expectedAbandoned)
	}

	// Wasted = simple tasks on expensive models = opus simple entry.
	if math.Abs(report.WastedSpend-0.50) > 0.001 {
		t.Errorf("WastedSpend = %f, want 0.50", report.WastedSpend)
	}

	expectedYield := expectedProductive / expectedTotal
	if math.Abs(report.YieldRate-expectedYield) > 0.001 {
		t.Errorf("YieldRate = %f, want %f", report.YieldRate, expectedYield)
	}

	if len(report.ByModel) != 4 {
		t.Errorf("expected 4 models, got %d", len(report.ByModel))
	}
	if len(report.ByTaskType) != 5 {
		t.Errorf("expected 5 task types, got %d", len(report.ByTaskType))
	}
}

func TestAnalyze_AllKept(t *testing.T) {
	entries := []CostEntry{
		{Model: "sonnet", TaskType: "generation", CostUSD: 1.00, Kept: true, InputTokens: 1000, OutputTokens: 2000},
		{Model: "sonnet", TaskType: "debug", CostUSD: 0.50, Kept: true, InputTokens: 500, OutputTokens: 1000},
	}

	report := Analyze(entries)

	if report.AbandonedSpend != 0 {
		t.Errorf("AbandonedSpend = %f, want 0", report.AbandonedSpend)
	}
	if math.Abs(report.YieldRate-1.0) > 0.001 {
		t.Errorf("YieldRate = %f, want 1.0", report.YieldRate)
	}
}

func TestAnalyze_AllAbandoned(t *testing.T) {
	entries := []CostEntry{
		{Model: "sonnet", TaskType: "generation", CostUSD: 1.00, Kept: false, InputTokens: 1000, OutputTokens: 2000},
		{Model: "sonnet", TaskType: "debug", CostUSD: 0.50, Kept: false, InputTokens: 500, OutputTokens: 1000},
	}

	report := Analyze(entries)

	if report.ProductiveSpend != 0 {
		t.Errorf("ProductiveSpend = %f, want 0", report.ProductiveSpend)
	}
	if report.YieldRate != 0 {
		t.Errorf("YieldRate = %f, want 0", report.YieldRate)
	}
}

func TestFormatOptimizationReport(t *testing.T) {
	report := &OptimizationReport{
		TotalSpend:      5.00,
		WastedSpend:     0.50,
		AbandonedSpend:  1.20,
		ProductiveSpend: 3.30,
		YieldRate:       0.66,
		ByModel: map[string]*ModelSpend{
			"opus":   {Model: "opus", TotalCost: 3.50, Calls: 5, AvgTokens: 3000},
			"sonnet": {Model: "sonnet", TotalCost: 1.50, Calls: 10, AvgTokens: 1500},
		},
		ByTaskType: map[string]*TaskSpend{
			"simple":     {TaskType: "simple", TotalCost: 0.50, Calls: 3, SuggestedModel: "haiku"},
			"generation": {TaskType: "generation", TotalCost: 4.50, Calls: 12, SuggestedModel: "opus"},
		},
		Recommendations: []Recommendation{
			{Type: "downgrade", Description: "Use cheaper models for simple tasks", Savings: 0.45},
		},
	}

	output := FormatOptimizationReport(report)

	if !strings.Contains(output, "Cost Optimization Report") {
		t.Error("output missing report header")
	}
	if !strings.Contains(output, "$5.0000") {
		t.Error("output missing total spend")
	}
	if !strings.Contains(output, "opus") {
		t.Error("output missing opus model")
	}
	if !strings.Contains(output, "sonnet") {
		t.Error("output missing sonnet model")
	}
	if !strings.Contains(output, "downgrade") {
		t.Error("output missing recommendation type")
	}
	if !strings.Contains(output, "66.0%") {
		t.Error("output missing yield rate")
	}
}

func TestIsExpensiveModel(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"opus", true},
		{"claude-opus-4-20250514", true},
		{"gpt-4", true},
		{"gpt-4-turbo", true},
		{"sonnet", false},
		{"haiku", false},
		{"gpt-4o-mini", false},
		{"gpt-4o", false},
		{"gpt-3.5-turbo", false},
	}
	for _, tt := range tests {
		got := isExpensiveModel(tt.model)
		if got != tt.expected {
			t.Errorf("isExpensiveModel(%q) = %v, want %v", tt.model, got, tt.expected)
		}
	}
}

func TestNormalizeModelName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-opus-4-20250514", "opus"},
		{"claude-sonnet-4-20250514", "sonnet"},
		{"claude-haiku-3-20250307", "haiku"},
		{"gpt-4o-mini", "gpt-4o-mini"},
		{"some-unknown-model", "some-unknown-model"},
	}
	for _, tt := range tests {
		got := normalizeModelName(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeModelName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
