package analytics

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func newTestDashboard() *Dashboard {
	d := NewDashboard()
	now := time.Now()

	// Add tool usage
	d.ToolUsage["Edit"] = 234
	d.ToolUsage["Read"] = 189
	d.ToolUsage["Bash"] = 156
	d.ToolUsage["Write"] = 45

	// Record several sessions across different days
	sessions := []SessionSummary{
		{
			ID: "sess-001", Date: now.AddDate(0, 0, -6), Duration: 10 * time.Minute,
			Model: "claude-sonnet-4-6", Provider: "anthropic",
			TokensUsed: 50000, CostUSD: 1.20, ToolCalls: 15, FilesModified: 3, Success: true,
		},
		{
			ID: "sess-002", Date: now.AddDate(0, 0, -5), Duration: 8 * time.Minute,
			Model: "claude-sonnet-4-6", Provider: "anthropic",
			TokensUsed: 40000, CostUSD: 0.95, ToolCalls: 10, FilesModified: 2, Success: true,
		},
		{
			ID: "sess-003", Date: now.AddDate(0, 0, -4), Duration: 15 * time.Minute,
			Model: "claude-haiku-4-5", Provider: "anthropic",
			TokensUsed: 80000, CostUSD: 0.40, ToolCalls: 20, FilesModified: 5, Success: true,
		},
		{
			ID: "sess-004", Date: now.AddDate(0, 0, -3), Duration: 5 * time.Minute,
			Model: "gpt-4o-mini", Provider: "openai",
			TokensUsed: 20000, CostUSD: 0.10, ToolCalls: 5, FilesModified: 1, Success: false,
		},
		{
			ID: "sess-005", Date: now.AddDate(0, 0, -2), Duration: 20 * time.Minute,
			Model: "claude-sonnet-4-6", Provider: "anthropic",
			TokensUsed: 120000, CostUSD: 2.80, ToolCalls: 30, FilesModified: 8, Success: true,
		},
		{
			ID: "sess-006", Date: now.AddDate(0, 0, -1), Duration: 12 * time.Minute,
			Model: "claude-haiku-4-5", Provider: "anthropic",
			TokensUsed: 60000, CostUSD: 0.30, ToolCalls: 12, FilesModified: 4, Success: true,
		},
		{
			ID: "sess-007", Date: now, Duration: 7 * time.Minute,
			Model: "claude-sonnet-4-6", Provider: "anthropic",
			TokensUsed: 35000, CostUSD: 0.85, ToolCalls: 8, FilesModified: 2, Success: true,
		},
	}

	for _, s := range sessions {
		d.RecordSession(s)
	}
	return d
}

func TestNewDashboard(t *testing.T) {
	d := NewDashboard()
	if d == nil {
		t.Fatal("NewDashboard returned nil")
	}
	if d.Sessions == nil {
		t.Error("Sessions should be initialized")
	}
	if d.DailyStats == nil {
		t.Error("DailyStats should be initialized")
	}
	if d.ModelUsage == nil {
		t.Error("ModelUsage should be initialized")
	}
	if d.ToolUsage == nil {
		t.Error("ToolUsage should be initialized")
	}
	if d.TotalCostUSD != 0 {
		t.Error("TotalCostUSD should be 0")
	}
	if d.TotalTokens != 0 {
		t.Error("TotalTokens should be 0")
	}
	if d.ActiveDays != 0 {
		t.Error("ActiveDays should be 0")
	}
}

func TestRecordSession(t *testing.T) {
	d := NewDashboard()
	now := time.Now()

	s := SessionSummary{
		ID: "test-1", Date: now, Duration: 5 * time.Minute,
		Model: "claude-sonnet-4-6", Provider: "anthropic",
		TokensUsed: 10000, CostUSD: 0.50, ToolCalls: 5, FilesModified: 2, Success: true,
	}
	d.RecordSession(s)

	if len(d.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(d.Sessions))
	}
	if d.TotalCostUSD != 0.50 {
		t.Errorf("expected cost 0.50, got %f", d.TotalCostUSD)
	}
	if d.TotalTokens != 10000 {
		t.Errorf("expected 10000 tokens, got %d", d.TotalTokens)
	}
	if d.ActiveDays != 1 {
		t.Errorf("expected 1 active day, got %d", d.ActiveDays)
	}

	// Record another session on the same day
	s2 := SessionSummary{
		ID: "test-2", Date: now, Duration: 3 * time.Minute,
		Model: "claude-sonnet-4-6", Provider: "anthropic",
		TokensUsed: 5000, CostUSD: 0.25, ToolCalls: 3, FilesModified: 1, Success: true,
	}
	d.RecordSession(s2)

	if len(d.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(d.Sessions))
	}
	if d.ActiveDays != 1 {
		t.Errorf("same day should not increase active days, got %d", d.ActiveDays)
	}
	if d.TotalCostUSD != 0.75 {
		t.Errorf("expected total cost 0.75, got %f", d.TotalCostUSD)
	}

	// Record on a different day
	s3 := SessionSummary{
		ID: "test-3", Date: now.AddDate(0, 0, -1), Duration: 7 * time.Minute,
		Model: "gpt-4o-mini", Provider: "openai",
		TokensUsed: 8000, CostUSD: 0.10, ToolCalls: 2, FilesModified: 1, Success: false,
	}
	d.RecordSession(s3)

	if d.ActiveDays != 2 {
		t.Errorf("expected 2 active days, got %d", d.ActiveDays)
	}

	// Check model usage
	if ms, ok := d.ModelUsage["claude-sonnet-4-6"]; !ok {
		t.Error("claude-sonnet-4-6 not in model usage")
	} else {
		if ms.Requests != 2 {
			t.Errorf("expected 2 requests for sonnet, got %d", ms.Requests)
		}
		if ms.Tokens != 15000 {
			t.Errorf("expected 15000 tokens for sonnet, got %d", ms.Tokens)
		}
	}

	if ms, ok := d.ModelUsage["gpt-4o-mini"]; !ok {
		t.Error("gpt-4o-mini not in model usage")
	} else {
		if ms.Requests != 1 {
			t.Errorf("expected 1 request for gpt-4o-mini, got %d", ms.Requests)
		}
	}
}

func TestRenderOverview(t *testing.T) {
	d := newTestDashboard()
	output := d.RenderOverview()

	if output == "" {
		t.Fatal("RenderOverview returned empty string")
	}

	// Check that key elements are present
	checks := []string{
		"graycode Analytics Dashboard",
		"Total Sessions:",
		"Active Days:",
		"Total Tokens:",
		"Total Cost:",
		"Avg Session:",
		"Success Rate:",
		"Last 7 Days:",
		"Top Models:",
		"Top Tools:",
		"claude-sonnet-4-6",
		"Edit",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("RenderOverview missing expected text: %q", check)
		}
	}

	// Check borders present
	if !strings.Contains(output, "+=") {
		t.Error("RenderOverview missing top border")
	}
}

func TestRenderOverviewEmpty(t *testing.T) {
	d := NewDashboard()
	output := d.RenderOverview()

	if output == "" {
		t.Fatal("RenderOverview on empty dashboard returned empty string")
	}
	if !strings.Contains(output, "Total Sessions: 0") {
		t.Error("empty dashboard should show 0 sessions")
	}
}

func TestRenderCostChart(t *testing.T) {
	d := newTestDashboard()
	output := d.RenderCostChart(7)

	if output == "" {
		t.Fatal("RenderCostChart returned empty string")
	}
	if !strings.Contains(output, "Daily Cost Chart") {
		t.Error("RenderCostChart missing header")
	}
	if !strings.Contains(output, "$") {
		t.Error("RenderCostChart missing cost values")
	}
	// Should contain bar characters
	if !strings.Contains(output, "#") {
		t.Error("RenderCostChart missing bar characters")
	}
}

func TestRenderCostChartEmpty(t *testing.T) {
	d := NewDashboard()
	output := d.RenderCostChart(7)

	if !strings.Contains(output, "No data available") {
		t.Error("empty chart should show no data message")
	}
}

func TestRenderTokenChart(t *testing.T) {
	d := newTestDashboard()
	output := d.RenderTokenChart(7)

	if output == "" {
		t.Fatal("RenderTokenChart returned empty string")
	}
	if !strings.Contains(output, "Daily Token Usage") {
		t.Error("RenderTokenChart missing header")
	}
	// Should contain formatted token values
	if !strings.Contains(output, "k") {
		t.Error("RenderTokenChart missing token values (expected 'k' suffix)")
	}
}

func TestRenderTokenChartEmpty(t *testing.T) {
	d := NewDashboard()
	output := d.RenderTokenChart(7)

	if !strings.Contains(output, "No data available") {
		t.Error("empty token chart should show no data message")
	}
}

func TestRenderModelBreakdown(t *testing.T) {
	d := newTestDashboard()
	output := d.RenderModelBreakdown()

	if output == "" {
		t.Fatal("RenderModelBreakdown returned empty string")
	}
	if !strings.Contains(output, "Model Usage Breakdown") {
		t.Error("missing header")
	}
	if !strings.Contains(output, "claude-sonnet-4-6") {
		t.Error("missing claude-sonnet-4-6")
	}
	if !strings.Contains(output, "claude-haiku-4-5") {
		t.Error("missing claude-haiku-4-5")
	}
	if !strings.Contains(output, "gpt-4o-mini") {
		t.Error("missing gpt-4o-mini")
	}
	if !strings.Contains(output, "%") {
		t.Error("missing percentage indicators")
	}
	if !strings.Contains(output, "Requests:") {
		t.Error("missing Requests line")
	}
}

func TestRenderModelBreakdownEmpty(t *testing.T) {
	d := NewDashboard()
	output := d.RenderModelBreakdown()

	if !strings.Contains(output, "No model data available") {
		t.Error("empty breakdown should show no data message")
	}
}

func TestRenderRecentSessions(t *testing.T) {
	d := newTestDashboard()
	output := d.RenderRecentSessions(5)

	if output == "" {
		t.Fatal("RenderRecentSessions returned empty string")
	}
	if !strings.Contains(output, "Recent Sessions") {
		t.Error("missing header")
	}
	if !strings.Contains(output, "ID") {
		t.Error("missing table header")
	}
	if !strings.Contains(output, "sess-007") {
		t.Error("missing most recent session")
	}
	if !strings.Contains(output, "OK") {
		t.Error("missing success status")
	}
	if !strings.Contains(output, "FAIL") {
		t.Error("missing failure status")
	}
	if !strings.Contains(output, "Showing 5 of 7") {
		t.Error("missing session count line")
	}
}

func TestRenderRecentSessionsLessThanN(t *testing.T) {
	d := NewDashboard()
	d.RecordSession(SessionSummary{
		ID: "only-one", Date: time.Now(), Duration: 3 * time.Minute,
		Model: "test-model", Provider: "test",
		TokensUsed: 1000, CostUSD: 0.01, ToolCalls: 1, FilesModified: 0, Success: true,
	})

	output := d.RenderRecentSessions(10)
	if !strings.Contains(output, "only-one") {
		t.Error("should show the single session")
	}
	if !strings.Contains(output, "Showing 1 of 1") {
		t.Error("should indicate 1 of 1 sessions")
	}
}

func TestExportImport(t *testing.T) {
	d := newTestDashboard()

	data, err := d.Export()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Export returned empty data")
	}

	// Verify it's valid JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Export produced invalid JSON: %v", err)
	}

	// Import into a new dashboard
	d2 := NewDashboard()
	if err := d2.Import(data); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify key fields match
	if len(d2.Sessions) != len(d.Sessions) {
		t.Errorf("sessions count mismatch: %d vs %d", len(d2.Sessions), len(d.Sessions))
	}
	if d2.TotalTokens != d.TotalTokens {
		t.Errorf("total tokens mismatch: %d vs %d", d2.TotalTokens, d.TotalTokens)
	}
	if d2.TotalCostUSD != d.TotalCostUSD {
		t.Errorf("total cost mismatch: %f vs %f", d2.TotalCostUSD, d.TotalCostUSD)
	}
	if d2.ActiveDays != d.ActiveDays {
		t.Errorf("active days mismatch: %d vs %d", d2.ActiveDays, d.ActiveDays)
	}
	if len(d2.ModelUsage) != len(d.ModelUsage) {
		t.Errorf("model usage count mismatch: %d vs %d", len(d2.ModelUsage), len(d.ModelUsage))
	}
	if len(d2.ToolUsage) != len(d.ToolUsage) {
		t.Errorf("tool usage count mismatch: %d vs %d", len(d2.ToolUsage), len(d.ToolUsage))
	}
}

func TestImportInvalidJSON(t *testing.T) {
	d := NewDashboard()
	err := d.Import([]byte("not json"))
	if err == nil {
		t.Error("Import should fail on invalid JSON")
	}
	if !strings.Contains(err.Error(), "importing dashboard data") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	d := newTestDashboard()

	data1, _ := d.Export()
	d2 := NewDashboard()
	_ = d2.Import(data1)
	data2, _ := d2.Export()

	// Both exports should produce equivalent JSON
	var m1, m2 map[string]interface{}
	json.Unmarshal(data1, &m1)
	json.Unmarshal(data2, &m2)

	j1, _ := json.Marshal(m1)
	j2, _ := json.Marshal(m2)

	if string(j1) != string(j2) {
		t.Error("round-trip export/import produced different data")
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{500, "500"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{50000, "50.0k"},
		{1000000, "1.0M"},
		{2400000, "2.4M"},
	}
	for _, tc := range tests {
		result := formatTokens(tc.input)
		if result != tc.expected {
			t.Errorf("formatTokens(%d) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{12 * time.Minute, "12m"},
		{90 * time.Minute, "1h30m"},
		{2*time.Hour + 15*time.Minute, "2h15m"},
	}
	for _, tc := range tests {
		result := formatDuration(tc.input)
		if result != tc.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestCenterText(t *testing.T) {
	result := centerText("hello", 11)
	if result != "   hello   " {
		t.Errorf("centerText(\"hello\", 11) = %q, want %q", result, "   hello   ")
	}

	result = centerText("hi", 10)
	if len(result) != 10 {
		t.Errorf("centerText should produce string of exact width, got %d", len(result))
	}
}

func TestPadRight(t *testing.T) {
	result := padRight("hello", 10)
	if result != "hello     " {
		t.Errorf("padRight(\"hello\", 10) = %q", result)
	}
	if len(result) != 10 {
		t.Errorf("padRight should produce string of exact width, got %d", len(result))
	}
}

func TestDashboardModelLatencyAndErrorRate(t *testing.T) {
	d := NewDashboard()
	d.ModelUsage["test-model"] = DashModelStats{
		Requests:     100,
		Tokens:       500000,
		CostUSD:      10.00,
		AvgLatencyMs: 450,
		ErrorRate:    0.05,
	}

	output := d.RenderModelBreakdown()
	if !strings.Contains(output, "450ms") {
		t.Error("RenderModelBreakdown should show latency")
	}
	if !strings.Contains(output, "5.0%") {
		t.Error("RenderModelBreakdown should show error rate")
	}
}

func TestMultipleSessionsSameDay(t *testing.T) {
	d := NewDashboard()
	baseTime := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		d.RecordSession(SessionSummary{
			ID:         fmt.Sprintf("s-%d", i),
			Date:       baseTime.Add(time.Duration(i) * time.Hour),
			Duration:   time.Duration(5+i) * time.Minute,
			Model:      "claude-sonnet-4-6",
			Provider:   "anthropic",
			TokensUsed: 10000 + i*1000,
			CostUSD:    0.50 + float64(i)*0.10,
			ToolCalls:  5 + i,
			Success:    true,
		})
	}

	if d.ActiveDays != 1 {
		t.Errorf("all sessions same day, expected 1 active day, got %d", d.ActiveDays)
	}
	if len(d.DailyStats) != 1 {
		t.Errorf("expected 1 daily stat entry, got %d", len(d.DailyStats))
	}
	if d.DailyStats[0].Sessions != 5 {
		t.Errorf("expected 5 sessions in day stat, got %d", d.DailyStats[0].Sessions)
	}
}
