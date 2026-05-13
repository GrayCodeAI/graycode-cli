package analytics

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func sampleSessions() []SessionData {
	base := time.Date(2024, 3, 15, 9, 0, 0, 0, time.UTC)
	return []SessionData{
		{
			ID: "s1", StartTime: base, Duration: 10 * time.Minute,
			Model: "claude-sonnet-4-6", Provider: "anthropic",
			TokensIn: 15000, TokensOut: 4000, CostUSD: 0.50,
			ToolCalls: 12, FilesModified: 3, TestsPassed: true, Success: true,
			TaskType: "code implementation",
		},
		{
			ID: "s2", StartTime: base.Add(1 * time.Hour), Duration: 15 * time.Minute,
			Model: "claude-sonnet-4-6", Provider: "anthropic",
			TokensIn: 20000, TokensOut: 5000, CostUSD: 0.65,
			ToolCalls: 8, FilesModified: 2, TestsPassed: true, Success: true,
			TaskType: "code implementation",
		},
		{
			ID: "s3", StartTime: base.Add(2 * time.Hour), Duration: 8 * time.Minute,
			Model: "claude-opus-4-6", Provider: "anthropic",
			TokensIn: 30000, TokensOut: 8000, CostUSD: 1.20,
			ToolCalls: 5, FilesModified: 1, TestsPassed: false, Success: false,
			TaskType: "debugging",
		},
		{
			ID: "s4", StartTime: base.Add(3 * time.Hour), Duration: 20 * time.Minute,
			Model: "claude-sonnet-4-6", Provider: "anthropic",
			TokensIn: 25000, TokensOut: 7000, CostUSD: 0.80,
			ToolCalls: 15, FilesModified: 5, TestsPassed: true, Success: true,
			TaskType: "code implementation",
		},
		{
			ID: "s5", StartTime: base.Add(4 * time.Hour), Duration: 12 * time.Minute,
			Model: "claude-sonnet-4-6", Provider: "anthropic",
			TokensIn: 18000, TokensOut: 4500, CostUSD: 0.55,
			ToolCalls: 10, FilesModified: 2, TestsPassed: true, Success: true,
			TaskType: "refactoring",
		},
		{
			ID: "s6", StartTime: base.Add(24 * time.Hour), Duration: 7 * time.Minute,
			Model: "claude-sonnet-4-6", Provider: "anthropic",
			TokensIn: 12000, TokensOut: 3000, CostUSD: 0.40,
			ToolCalls: 6, FilesModified: 1, TestsPassed: true, Success: true,
			TaskType: "code implementation",
		},
	}
}

func setupAnalytics() *SessionAnalytics {
	sa := NewSessionAnalytics()
	for _, s := range sampleSessions() {
		sa.Record(s)
	}
	return sa
}

func TestNewSessionAnalytics(t *testing.T) {
	sa := NewSessionAnalytics()
	if sa == nil {
		t.Fatal("NewSessionAnalytics returned nil")
	}
	if len(sa.Sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sa.Sessions))
	}
}

func TestRecord(t *testing.T) {
	sa := NewSessionAnalytics()
	data := SessionData{
		ID:    "test-1",
		Model: "claude-sonnet-4-6",
	}
	sa.Record(data)
	if len(sa.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sa.Sessions))
	}
	if sa.Sessions[0].ID != "test-1" {
		t.Fatalf("expected ID test-1, got %s", sa.Sessions[0].ID)
	}
}

func TestRecordConcurrent(t *testing.T) {
	sa := NewSessionAnalytics()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sa.Record(SessionData{
				ID:    "concurrent-" + time.Now().String(),
				Model: "test-model",
			})
		}(i)
	}
	wg.Wait()
	if len(sa.Sessions) != 100 {
		t.Fatalf("expected 100 sessions, got %d", len(sa.Sessions))
	}
}

func TestDailyReport(t *testing.T) {
	sa := setupAnalytics()
	date := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	report := sa.DailyReport(date)

	// Check required elements
	if !strings.Contains(report, "Daily Report: 2024-03-15") {
		t.Error("report missing date header")
	}
	if !strings.Contains(report, "Sessions: 5") {
		t.Errorf("report should show 5 sessions, got:\n%s", report)
	}
	if !strings.Contains(report, "Success:") {
		t.Error("report missing success line")
	}
	if !strings.Contains(report, "Cost:") {
		t.Error("report missing cost line")
	}
	if !strings.Contains(report, "Top model:") {
		t.Error("report missing top model")
	}
	if !strings.Contains(report, "claude-sonnet-4-6") {
		t.Error("report should reference claude-sonnet-4-6 as top model")
	}
}

func TestDailyReportEmpty(t *testing.T) {
	sa := setupAnalytics()
	// Query a date with no sessions
	date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	report := sa.DailyReport(date)
	if !strings.Contains(report, "No sessions recorded") {
		t.Error("expected no sessions message")
	}
}

func TestWeeklyTrend(t *testing.T) {
	sa := setupAnalytics()
	trend := sa.WeeklyTrend()

	if !strings.Contains(trend, "Weekly Trend") {
		t.Error("missing header")
	}
	if !strings.Contains(trend, "Date") {
		t.Error("missing date column")
	}
	if !strings.Contains(trend, "Sessions") {
		t.Error("missing sessions column")
	}
	if !strings.Contains(trend, "Cost") {
		t.Error("missing cost column")
	}
}

func TestModelComparison(t *testing.T) {
	sa := setupAnalytics()
	comparison := sa.ModelComparison()

	if !strings.Contains(comparison, "Model Comparison") {
		t.Error("missing header")
	}
	if !strings.Contains(comparison, "claude-sonnet-4-6") {
		t.Error("missing sonnet model")
	}
	if !strings.Contains(comparison, "claude-opus-4-6") {
		t.Error("missing opus model")
	}
	if !strings.Contains(comparison, "Success") {
		t.Error("missing success column")
	}
}

func TestModelComparisonEmpty(t *testing.T) {
	sa := NewSessionAnalytics()
	comparison := sa.ModelComparison()
	if !strings.Contains(comparison, "No model data") {
		t.Error("expected no data message")
	}
}

func TestProductivityScore(t *testing.T) {
	sa := setupAnalytics()
	score := sa.ProductivityScore()

	if score < 0 || score > 100 {
		t.Fatalf("score out of range: %f", score)
	}
	// With mostly successful sessions and reasonable efficiency, score should be decent
	if score < 20 {
		t.Fatalf("expected reasonable score, got %f", score)
	}
}

func TestProductivityScoreEmpty(t *testing.T) {
	sa := NewSessionAnalytics()
	score := sa.ProductivityScore()
	if score != 0 {
		t.Fatalf("expected 0 for empty analytics, got %f", score)
	}
}

func TestUsagePatterns(t *testing.T) {
	sa := setupAnalytics()
	patterns := sa.UsagePatterns()

	if len(patterns) == 0 {
		t.Fatal("expected some patterns")
	}

	foundActive := false
	foundAvg := false
	foundModel := false
	for _, p := range patterns {
		if strings.Contains(p, "Most active:") {
			foundActive = true
		}
		if strings.Contains(p, "Avg session:") {
			foundAvg = true
		}
		if strings.Contains(p, "Preferred model:") {
			foundModel = true
		}
	}

	if !foundActive {
		t.Error("missing 'Most active' pattern")
	}
	if !foundAvg {
		t.Error("missing 'Avg session' pattern")
	}
	if !foundModel {
		t.Error("missing 'Preferred model' pattern")
	}
}

func TestUsagePatternsEmpty(t *testing.T) {
	sa := NewSessionAnalytics()
	patterns := sa.UsagePatterns()
	if len(patterns) != 1 || !strings.Contains(patterns[0], "No usage data") {
		t.Error("expected no data message")
	}
}

func TestCostProjection(t *testing.T) {
	sa := setupAnalytics()
	projection := sa.CostProjection(30)

	if projection <= 0 {
		t.Fatalf("expected positive projection, got %f", projection)
	}

	// Projection for 0 days should be 0
	if sa.CostProjection(0) != 0 {
		t.Error("expected 0 for 0 days")
	}
}

func TestCostProjectionEmpty(t *testing.T) {
	sa := NewSessionAnalytics()
	if sa.CostProjection(30) != 0 {
		t.Error("expected 0 for empty analytics")
	}
}

func TestFormatOverview(t *testing.T) {
	sa := setupAnalytics()
	overview := sa.FormatOverview()

	checks := []string{
		"Session Analytics Overview",
		"Total sessions:",
		"Total duration:",
		"Success rate:",
		"Total cost:",
		"Total tokens:",
		"Tool calls:",
		"Files modified:",
		"Productivity:",
	}
	for _, check := range checks {
		if !strings.Contains(overview, check) {
			t.Errorf("overview missing: %s\nGot:\n%s", check, overview)
		}
	}
}

func TestFormatOverviewEmpty(t *testing.T) {
	sa := NewSessionAnalytics()
	overview := sa.FormatOverview()
	if !strings.Contains(overview, "No session data") {
		t.Error("expected no data message for empty analytics")
	}
}

func TestExportJSON(t *testing.T) {
	sa := setupAnalytics()
	exported := sa.Export("json")

	var sessions []SessionData
	if err := json.Unmarshal([]byte(exported), &sessions); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(sessions) != 6 {
		t.Fatalf("expected 6 sessions in export, got %d", len(sessions))
	}
	if sessions[0].ID != "s1" {
		t.Errorf("expected first session ID s1, got %s", sessions[0].ID)
	}
}

func TestExportCSV(t *testing.T) {
	sa := setupAnalytics()
	exported := sa.Export("csv")

	lines := strings.Split(strings.TrimSpace(exported), "\n")
	// Header + 6 data rows
	if len(lines) != 7 {
		t.Fatalf("expected 7 CSV lines (header + 6 rows), got %d", len(lines))
	}

	// Check header
	if !strings.Contains(lines[0], "id") || !strings.Contains(lines[0], "model") {
		t.Error("CSV header missing expected columns")
	}

	// Check a data row contains expected values
	if !strings.Contains(lines[1], "s1") {
		t.Error("first data row should contain session s1")
	}
}

func TestExportUnsupported(t *testing.T) {
	sa := setupAnalytics()
	result := sa.Export("xml")
	if !strings.Contains(result, "unsupported format") {
		t.Error("expected unsupported format message")
	}
}

func TestExportCaseInsensitive(t *testing.T) {
	sa := setupAnalytics()
	jsonResult := sa.Export("JSON")
	var sessions []SessionData
	if err := json.Unmarshal([]byte(jsonResult), &sessions); err != nil {
		t.Fatalf("Export(\"JSON\") should work case-insensitively: %v", err)
	}
}

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{500, "500"},
		{1500, "1K"},
		{125000, "125K"},
		{1500000, "1.5M"},
	}
	for _, tt := range tests {
		got := formatTokenCount(tt.input)
		if got != tt.expected {
			t.Errorf("formatTokenCount(%d) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestFmtSessionDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h 30m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
	}
	for _, tt := range tests {
		got := fmtSessionDuration(tt.input)
		if got != tt.expected {
			t.Errorf("fmtSessionDuration(%v) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestShortModelName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-sonnet-4-6", "sonnet"},
		{"claude-opus-4-6", "opus"},
		{"claude-haiku-4-6", "haiku"},
		{"gpt-4-turbo", "gpt"},
		{"custom-model", "custom"},
	}
	for _, tt := range tests {
		got := shortModelName(tt.input)
		if got != tt.expected {
			t.Errorf("shortModelName(%s) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestTopKey(t *testing.T) {
	m := map[string]int{
		"a": 3,
		"b": 7,
		"c": 5,
	}
	got := topKey(m)
	if got != "b" {
		t.Errorf("expected 'b', got '%s'", got)
	}
}

func TestTopKeyEmpty(t *testing.T) {
	m := map[string]int{}
	got := topKey(m)
	if got != "" {
		t.Errorf("expected empty string, got '%s'", got)
	}
}
