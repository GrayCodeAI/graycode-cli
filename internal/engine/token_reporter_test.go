package engine

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewTokenReporter(t *testing.T) {
	tr := NewTokenReporter(200000)

	if tr.SessionBudget != 200000 {
		t.Errorf("expected SessionBudget=200000, got %d", tr.SessionBudget)
	}
	if tr.SessionSpent != 0 {
		t.Errorf("expected SessionSpent=0, got %d", tr.SessionSpent)
	}
	if len(tr.Entries) != 0 {
		t.Errorf("expected empty Entries, got %d", len(tr.Entries))
	}
	if len(tr.Alerts) != 0 {
		t.Errorf("expected empty Alerts, got %d", len(tr.Alerts))
	}
}

func TestTokenReporterRecord(t *testing.T) {
	tr := NewTokenReporter(100000)

	tr.Record(500, 200, "sonnet", "Read", 0.01)

	if tr.SessionSpent != 700 {
		t.Errorf("expected SessionSpent=700, got %d", tr.SessionSpent)
	}
	if len(tr.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(tr.Entries))
	}

	e := tr.Entries[0]
	if e.InputTokens != 500 {
		t.Errorf("expected InputTokens=500, got %d", e.InputTokens)
	}
	if e.OutputTokens != 200 {
		t.Errorf("expected OutputTokens=200, got %d", e.OutputTokens)
	}
	if e.Model != "sonnet" {
		t.Errorf("expected Model=sonnet, got %s", e.Model)
	}
	if e.ToolName != "Read" {
		t.Errorf("expected ToolName=Read, got %s", e.ToolName)
	}
	if e.CostUSD != 0.01 {
		t.Errorf("expected CostUSD=0.01, got %f", e.CostUSD)
	}
	if e.Cumulative != 700 {
		t.Errorf("expected Cumulative=700, got %d", e.Cumulative)
	}
}

func TestRecordCumulative(t *testing.T) {
	tr := NewTokenReporter(100000)

	tr.Record(100, 50, "sonnet", "Read", 0.001)
	tr.Record(200, 100, "sonnet", "Edit", 0.002)
	tr.Record(300, 150, "haiku", "Bash", 0.001)

	if tr.SessionSpent != 900 {
		t.Errorf("expected SessionSpent=900, got %d", tr.SessionSpent)
	}

	if tr.Entries[0].Cumulative != 150 {
		t.Errorf("expected first Cumulative=150, got %d", tr.Entries[0].Cumulative)
	}
	if tr.Entries[1].Cumulative != 450 {
		t.Errorf("expected second Cumulative=450, got %d", tr.Entries[1].Cumulative)
	}
	if tr.Entries[2].Cumulative != 900 {
		t.Errorf("expected third Cumulative=900, got %d", tr.Entries[2].Cumulative)
	}
}

func TestGetRemaining(t *testing.T) {
	tr := NewTokenReporter(10000)

	if tr.GetRemaining() != 10000 {
		t.Errorf("expected 10000 remaining, got %d", tr.GetRemaining())
	}

	tr.Record(3000, 2000, "sonnet", "Read", 0.05)

	if tr.GetRemaining() != 5000 {
		t.Errorf("expected 5000 remaining, got %d", tr.GetRemaining())
	}

	// Overspend should return 0.
	tr.Record(4000, 2000, "sonnet", "Edit", 0.06)

	if tr.GetRemaining() != 0 {
		t.Errorf("expected 0 remaining when overspent, got %d", tr.GetRemaining())
	}
}

func TestGetUsageRate(t *testing.T) {
	tr := NewTokenReporter(100000)
	// Set startTime to 2 minutes ago.
	tr.mu.Lock()
	tr.startTime = time.Now().Add(-2 * time.Minute)
	tr.mu.Unlock()

	tr.Record(1000, 500, "sonnet", "Read", 0.01)

	rate := tr.GetUsageRate()
	// Should be approximately 750 tokens/minute (1500 tokens / 2 minutes).
	if rate < 700 || rate > 800 {
		t.Errorf("expected rate ~750, got %.1f", rate)
	}
}

func TestGetUsageRateZeroElapsed(t *testing.T) {
	tr := NewTokenReporter(100000)
	// When no time has elapsed, rate should be 0.
	rate := tr.GetUsageRate()
	if rate < 0 {
		t.Errorf("expected non-negative rate, got %.1f", rate)
	}
}

func TestProjectCompletion(t *testing.T) {
	tr := NewTokenReporter(10000)
	tr.mu.Lock()
	tr.startTime = time.Now().Add(-5 * time.Minute)
	tr.mu.Unlock()

	tr.Record(1000, 500, "sonnet", "Read", 0.01)
	// 1500 tokens in 5 min = 300 tok/min.
	// Remaining: 8500. ETA: 8500/300 = ~28.3 min.

	proj := tr.ProjectCompletion()
	expectedMin := 28 * time.Minute
	expectedMax := 29 * time.Minute

	if proj < expectedMin || proj > expectedMax {
		t.Errorf("expected projection between 28-29 min, got %v", proj)
	}
}

func TestProjectCompletionExhausted(t *testing.T) {
	tr := NewTokenReporter(1000)
	tr.mu.Lock()
	tr.startTime = time.Now().Add(-1 * time.Minute)
	tr.mu.Unlock()

	tr.Record(800, 300, "sonnet", "Read", 0.01)
	// Already over budget.

	proj := tr.ProjectCompletion()
	if proj != 0 {
		t.Errorf("expected 0 duration when budget exhausted, got %v", proj)
	}
}

func TestRenderLive(t *testing.T) {
	tr := NewTokenReporter(200000)
	tr.mu.Lock()
	tr.startTime = time.Now().Add(-10 * time.Minute)
	tr.mu.Unlock()

	tr.Record(30000, 12000, "sonnet", "Read", 1.0)
	tr.Record(2000, 1200, "sonnet", "Edit", 0.23)

	output := tr.RenderLive()

	if !strings.Contains(output, "Tokens:") {
		t.Error("expected Tokens: in output")
	}
	if !strings.Contains(output, "200,000") {
		t.Errorf("expected formatted budget, got: %s", output)
	}
	if !strings.Contains(output, "Rate:") {
		t.Error("expected Rate: in output")
	}
	if !strings.Contains(output, "tok/min") {
		t.Error("expected tok/min in output")
	}
	if !strings.Contains(output, "ETA:") {
		t.Error("expected ETA: in output")
	}
	if !strings.Contains(output, "Cost:") {
		t.Error("expected Cost: in output")
	}
	if !strings.Contains(output, "Edit") {
		t.Error("expected last tool name 'Edit' in output")
	}
}

func TestRenderBreakdown(t *testing.T) {
	tr := NewTokenReporter(100000)

	tr.Record(5000, 2000, "sonnet", "Read", 0.05)
	tr.Record(3000, 1500, "sonnet", "Edit", 0.03)
	tr.Record(2000, 1000, "haiku", "Bash", 0.01)
	tr.Record(1000, 500, "haiku", "Read", 0.005)

	output := tr.RenderBreakdown()

	if !strings.Contains(output, "Token Breakdown:") {
		t.Error("expected header")
	}
	if !strings.Contains(output, "Input:") {
		t.Error("expected Input: in breakdown")
	}
	if !strings.Contains(output, "Output:") {
		t.Error("expected Output: in breakdown")
	}
	if !strings.Contains(output, "By model:") {
		t.Error("expected By model: in breakdown")
	}
	if !strings.Contains(output, "sonnet") {
		t.Error("expected sonnet in breakdown")
	}
	if !strings.Contains(output, "haiku") {
		t.Error("expected haiku in breakdown")
	}
	if !strings.Contains(output, "By tool:") {
		t.Error("expected By tool: in breakdown")
	}
	if !strings.Contains(output, "Read") {
		t.Error("expected Read in breakdown")
	}
	if !strings.Contains(output, "Edit") {
		t.Error("expected Edit in breakdown")
	}
	if !strings.Contains(output, "Bash") {
		t.Error("expected Bash in breakdown")
	}
}

func TestRenderBreakdownEmpty(t *testing.T) {
	tr := NewTokenReporter(100000)
	output := tr.RenderBreakdown()

	if !strings.Contains(output, "No token usage recorded") {
		t.Errorf("expected empty message, got: %s", output)
	}
}

func TestCheckBudget(t *testing.T) {
	tr := NewTokenReporter(10000)

	// Below 50% - no alert.
	tr.Record(2000, 2000, "sonnet", "Read", 0.02)
	alert := tr.CheckBudget()
	if alert != nil {
		t.Error("expected no alert below 50%")
	}

	// At 50% - info.
	tr.Record(500, 500, "sonnet", "Edit", 0.01)
	alert = tr.CheckBudget()
	if alert == nil {
		t.Fatal("expected info alert at 50%")
	}
	if alert.Level != "info" {
		t.Errorf("expected level=info, got %s", alert.Level)
	}

	// At 75% - warning.
	tr.Record(1500, 1000, "sonnet", "Bash", 0.02)
	alert = tr.CheckBudget()
	if alert == nil {
		t.Fatal("expected warning alert at 75%")
	}
	if alert.Level != "warning" {
		t.Errorf("expected level=warning, got %s", alert.Level)
	}

	// At 90% - critical.
	tr.Record(1000, 500, "sonnet", "Read", 0.01)
	alert = tr.CheckBudget()
	if alert == nil {
		t.Fatal("expected critical alert at 90%")
	}
	if alert.Level != "critical" {
		t.Errorf("expected level=critical, got %s", alert.Level)
	}
}

func TestCheckBudgetNoDuplicateAlerts(t *testing.T) {
	tr := NewTokenReporter(10000)

	// Push past 50%.
	tr.Record(3000, 3000, "sonnet", "Read", 0.03)

	// First check should trigger info.
	alert := tr.CheckBudget()
	if alert == nil || alert.Level != "info" {
		t.Fatal("expected info alert first time")
	}

	// Second check at same level should not create a new alert.
	alertCount := len(tr.Alerts)
	alert = tr.CheckBudget()
	if len(tr.Alerts) != alertCount {
		t.Error("should not duplicate alerts")
	}
}

func TestFormatAlert(t *testing.T) {
	tr := NewTokenReporter(100000)

	alert := &BudgetAlert{
		Level:      "warning",
		Message:    "Three-quarters of token budget consumed.",
		Triggered:  time.Now(),
		Percentage: 75.5,
	}

	formatted := tr.FormatAlert(alert)
	if !strings.Contains(formatted, "[WARNING]") {
		t.Errorf("expected [WARNING], got: %s", formatted)
	}
	if !strings.Contains(formatted, "75.5%") {
		t.Errorf("expected 75.5%%, got: %s", formatted)
	}
	if !strings.Contains(formatted, "Three-quarters") {
		t.Errorf("expected message text, got: %s", formatted)
	}
}

func TestFormatAlertNil(t *testing.T) {
	tr := NewTokenReporter(100000)
	if tr.FormatAlert(nil) != "" {
		t.Error("expected empty string for nil alert")
	}
}

func TestGetHistory(t *testing.T) {
	tr := NewTokenReporter(100000)

	for i := 0; i < 10; i++ {
		tr.Record(100, 50, "sonnet", "Read", 0.001)
	}

	history := tr.GetHistory(3)
	if len(history) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(history))
	}

	// Should be the last 3 entries.
	if history[0].Cumulative != 1200 { // 8th entry: 8*150 = 1200
		t.Errorf("expected cumulative=1200, got %d", history[0].Cumulative)
	}
	if history[2].Cumulative != 1500 { // 10th entry: 10*150 = 1500
		t.Errorf("expected cumulative=1500, got %d", history[2].Cumulative)
	}
}

func TestGetHistoryMoreThanAvailable(t *testing.T) {
	tr := NewTokenReporter(100000)
	tr.Record(100, 50, "sonnet", "Read", 0.001)
	tr.Record(200, 100, "sonnet", "Edit", 0.002)

	history := tr.GetHistory(10)
	if len(history) != 2 {
		t.Errorf("expected 2 entries when asking for more than available, got %d", len(history))
	}
}

func TestGetHistoryZero(t *testing.T) {
	tr := NewTokenReporter(100000)
	tr.Record(100, 50, "sonnet", "Read", 0.001)

	history := tr.GetHistory(0)
	if history != nil {
		t.Errorf("expected nil for last=0, got %v", history)
	}
}

func TestTokenReporterReset(t *testing.T) {
	tr := NewTokenReporter(100000)
	tr.Record(5000, 3000, "sonnet", "Read", 0.05)
	tr.Record(5000, 3000, "sonnet", "Read", 0.05)

	tr.Reset()

	if tr.SessionSpent != 0 {
		t.Errorf("expected SessionSpent=0 after reset, got %d", tr.SessionSpent)
	}
	if len(tr.Entries) != 0 {
		t.Errorf("expected empty entries after reset, got %d", len(tr.Entries))
	}
	if len(tr.Alerts) != 0 {
		t.Errorf("expected empty alerts after reset, got %d", len(tr.Alerts))
	}
	if tr.SessionBudget != 100000 {
		t.Errorf("expected budget preserved after reset, got %d", tr.SessionBudget)
	}
}

func TestTokenReporterConcurrentAccess(t *testing.T) {
	tr := NewTokenReporter(1000000)
	var wg sync.WaitGroup

	// Concurrent writes.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.Record(100, 50, "sonnet", "Read", 0.001)
		}()
	}

	// Concurrent reads.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.GetRemaining()
			tr.GetUsageRate()
			tr.ProjectCompletion()
			tr.RenderLive()
			tr.RenderBreakdown()
			tr.GetHistory(5)
			tr.CheckBudget()
		}()
	}

	wg.Wait()

	if tr.SessionSpent != 50*150 {
		t.Errorf("expected SessionSpent=%d, got %d", 50*150, tr.SessionSpent)
	}
}

func TestTrFormatNumber(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{5, "5"},
		{42, "42"},
		{999, "999"},
		{1000, "1,000"},
		{12345, "12,345"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
		{200000, "200,000"},
	}

	for _, tt := range tests {
		got := trFormatNumber(tt.input)
		if got != tt.expected {
			t.Errorf("trFormatNumber(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestTrFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "0m"},
		{5 * time.Minute, "5m"},
		{30 * time.Minute, "30m"},
		{60 * time.Minute, "1h 0m"},
		{90 * time.Minute, "1h 30m"},
		{124 * time.Minute, "2h 4m"},
	}

	for _, tt := range tests {
		got := trFormatDuration(tt.input)
		if got != tt.expected {
			t.Errorf("trFormatDuration(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestAlertTriggeredDuringRecord(t *testing.T) {
	tr := NewTokenReporter(10000)

	// Recording past 50% should auto-generate an alert.
	tr.Record(3000, 2500, "sonnet", "Read", 0.04)

	if len(tr.Alerts) != 1 {
		t.Fatalf("expected 1 alert after crossing 50%%, got %d", len(tr.Alerts))
	}
	if tr.Alerts[0].Level != "info" {
		t.Errorf("expected info alert, got %s", tr.Alerts[0].Level)
	}
}

func TestMultipleThresholdsCrossedAtOnce(t *testing.T) {
	tr := NewTokenReporter(10000)

	// Jump straight past 90%.
	tr.Record(5000, 4500, "sonnet", "Read", 0.08)

	// Should trigger all three levels.
	if len(tr.Alerts) != 3 {
		t.Fatalf("expected 3 alerts when jumping past 90%%, got %d", len(tr.Alerts))
	}

	levels := map[string]bool{}
	for _, a := range tr.Alerts {
		levels[a.Level] = true
	}
	if !levels["info"] || !levels["warning"] || !levels["critical"] {
		t.Errorf("expected all three alert levels, got: %v", levels)
	}
}

func TestRenderBarEdgeCases(t *testing.T) {
	tr := NewTokenReporter(100)

	bar := tr.renderBar(0, 10)
	if bar != "░░░░░░░░░░" {
		t.Errorf("expected all empty, got %q", bar)
	}

	bar = tr.renderBar(100, 10)
	if bar != "██████████" {
		t.Errorf("expected all filled, got %q", bar)
	}

	bar = tr.renderBar(50, 10)
	if bar != "█████░░░░░" {
		t.Errorf("expected half filled, got %q", bar)
	}
}
