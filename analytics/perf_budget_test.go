package analytics

import (
	"strings"
	"testing"
	"time"
)

func TestNewPerfBudget(t *testing.T) {
	pb := NewPerfBudget()

	if pb == nil {
		t.Fatal("NewPerfBudget returned nil")
	}

	expectedSLOs := []string{"response_time", "accuracy", "cost_per_task", "token_efficiency", "tool_error_rate"}
	for _, name := range expectedSLOs {
		if _, ok := pb.SLOs[name]; !ok {
			t.Errorf("expected default SLO %q to exist", name)
		}
	}

	if len(pb.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(pb.Violations))
	}
}

func TestDefaultSLOValues(t *testing.T) {
	pb := NewPerfBudget()

	tests := []struct {
		metric string
		target float64
		name   string
	}{
		{"response_time", 5.0, "Response Time (P95)"},
		{"accuracy", 90.0, "Success Rate"},
		{"cost_per_task", 0.50, "Cost per Task"},
		{"token_efficiency", 5000.0, "Token Efficiency"},
		{"tool_error_rate", 5.0, "Tool Error Rate"},
	}

	for _, tt := range tests {
		slo := pb.SLOs[tt.metric]
		if slo.Target != tt.target {
			t.Errorf("SLO %s: expected target %f, got %f", tt.metric, tt.target, slo.Target)
		}
		if slo.Name != tt.name {
			t.Errorf("SLO %s: expected name %q, got %q", tt.metric, tt.name, slo.Name)
		}
		if slo.Status != "met" {
			t.Errorf("SLO %s: expected initial status 'met', got %q", tt.metric, slo.Status)
		}
	}
}

func TestPerfBudgetRecord(t *testing.T) {
	pb := NewPerfBudget()

	pb.Record("response_time", 2.0)
	pb.Record("response_time", 3.0)
	pb.Record("response_time", 4.0)

	slo := pb.SLOs["response_time"]
	if len(slo.Measurements) != 3 {
		t.Errorf("expected 3 measurements, got %d", len(slo.Measurements))
	}
}

func TestRecordUnknownMetric(t *testing.T) {
	pb := NewPerfBudget()
	// Should not panic
	pb.Record("unknown_metric", 1.0)
}

func TestCheckAllMet(t *testing.T) {
	pb := NewPerfBudget()

	// Record values within budget
	pb.Record("response_time", 2.0)
	pb.Record("response_time", 3.0)
	pb.Record("accuracy", 95.0)
	pb.Record("cost_per_task", 0.30)
	pb.Record("token_efficiency", 3000.0)
	pb.Record("tool_error_rate", 2.0)

	statuses := pb.Check()

	for metric, status := range statuses {
		if status != "met" {
			t.Errorf("expected SLO %q to be 'met', got %q", metric, status)
		}
	}
}

func TestCheckViolation(t *testing.T) {
	pb := NewPerfBudget()

	// Record values that exceed budget
	pb.Record("response_time", 6.0)
	pb.Record("response_time", 7.0)
	pb.Record("response_time", 8.0)

	statuses := pb.Check()
	if statuses["response_time"] != "violated" {
		t.Errorf("expected response_time to be 'violated', got %q", statuses["response_time"])
	}
}

func TestCheckAccuracyViolation(t *testing.T) {
	pb := NewPerfBudget()

	// Record accuracy below 90%
	pb.Record("accuracy", 80.0)
	pb.Record("accuracy", 85.0)
	pb.Record("accuracy", 82.0)

	statuses := pb.Check()
	if statuses["accuracy"] != "violated" {
		t.Errorf("expected accuracy to be 'violated', got %q", statuses["accuracy"])
	}
}

func TestCheckAtRisk(t *testing.T) {
	pb := NewPerfBudget()

	// Response time at 80-100% of budget (4.0-5.0) => at_risk
	pb.Record("response_time", 4.2)
	pb.Record("response_time", 4.3)
	pb.Record("response_time", 4.1)

	statuses := pb.Check()
	if statuses["response_time"] != "at_risk" {
		t.Errorf("expected response_time to be 'at_risk', got %q", statuses["response_time"])
	}
}

func TestGetViolations(t *testing.T) {
	pb := NewPerfBudget()

	before := time.Now().Add(-time.Second)

	// Create a violation
	pb.Record("response_time", 10.0)
	pb.Record("response_time", 12.0)

	violations := pb.GetViolations(before)
	if len(violations) == 0 {
		t.Fatal("expected at least one violation")
	}

	if violations[0].SLO != "Response Time (P95)" {
		t.Errorf("expected violation SLO to be 'Response Time (P95)', got %q", violations[0].SLO)
	}
}

func TestGetViolationsFiltering(t *testing.T) {
	pb := NewPerfBudget()

	// Create a violation
	pb.Record("response_time", 10.0)

	future := time.Now().Add(time.Hour)
	violations := pb.GetViolations(future)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations for future time, got %d", len(violations))
	}
}

func TestFormatDashboard(t *testing.T) {
	pb := NewPerfBudget()

	pb.Record("response_time", 3.2)
	pb.Record("accuracy", 94.0)
	pb.Record("cost_per_task", 0.38)
	pb.Record("token_efficiency", 4200.0)
	pb.Record("tool_error_rate", 3.2)

	dashboard := pb.FormatDashboard()

	expectedParts := []string{
		"Performance Budget:",
		"SLO",
		"Target",
		"Current",
		"Status",
		"Response Time (P95)",
		"Success Rate",
		"Cost per Task",
		"Token Efficiency",
		"Tool Error Rate",
		"Overall:",
	}

	for _, part := range expectedParts {
		if !strings.Contains(dashboard, part) {
			t.Errorf("dashboard missing expected content: %q", part)
		}
	}
}

func TestFormatDashboardStatusIndicators(t *testing.T) {
	pb := NewPerfBudget()

	pb.Record("response_time", 2.0)
	pb.Record("accuracy", 95.0)

	dashboard := pb.FormatDashboard()
	if !strings.Contains(dashboard, "MET") {
		t.Error("dashboard should show MET status")
	}
}

func TestFormatDashboardViolated(t *testing.T) {
	pb := NewPerfBudget()

	pb.Record("response_time", 10.0)

	dashboard := pb.FormatDashboard()
	if !strings.Contains(dashboard, "VIOLATED") {
		t.Error("dashboard should show VIOLATED status")
	}
}

func TestAddSLO(t *testing.T) {
	pb := NewPerfBudget()

	newSLO := SLO{
		Name:   "Memory Usage",
		Metric: "memory_mb",
		Target: 512.0,
		Window: time.Hour,
	}

	pb.AddSLO(newSLO)

	if _, ok := pb.SLOs["memory_mb"]; !ok {
		t.Fatal("custom SLO not found after AddSLO")
	}

	slo := pb.SLOs["memory_mb"]
	if slo.Name != "Memory Usage" {
		t.Errorf("expected name 'Memory Usage', got %q", slo.Name)
	}
	if slo.Target != 512.0 {
		t.Errorf("expected target 512.0, got %f", slo.Target)
	}
	if slo.Status != "met" {
		t.Errorf("expected initial status 'met', got %q", slo.Status)
	}
	if slo.Measurements == nil {
		t.Error("expected Measurements to be initialized")
	}
}

func TestProjectTrendImproving(t *testing.T) {
	pb := NewPerfBudget()

	// Response time decreasing (improving for lower-is-better)
	measurements := []float64{5.0, 4.8, 4.6, 4.5, 3.5, 3.2, 3.0, 2.8}
	for _, m := range measurements {
		pb.Record("response_time", m)
	}

	trend := pb.ProjectTrend("response_time")
	if trend != "improving" {
		t.Errorf("expected trend 'improving', got %q", trend)
	}
}

func TestProjectTrendDegrading(t *testing.T) {
	pb := NewPerfBudget()

	// Response time increasing (degrading for lower-is-better)
	measurements := []float64{2.0, 2.1, 2.2, 2.3, 3.5, 3.8, 4.0, 4.2}
	for _, m := range measurements {
		pb.Record("response_time", m)
	}

	trend := pb.ProjectTrend("response_time")
	if trend != "degrading" {
		t.Errorf("expected trend 'degrading', got %q", trend)
	}
}

func TestProjectTrendStable(t *testing.T) {
	pb := NewPerfBudget()

	// Response time mostly flat
	measurements := []float64{3.0, 3.0, 3.0, 3.0, 3.0, 3.0}
	for _, m := range measurements {
		pb.Record("response_time", m)
	}

	trend := pb.ProjectTrend("response_time")
	if trend != "stable" {
		t.Errorf("expected trend 'stable', got %q", trend)
	}
}

func TestProjectTrendAccuracyImproving(t *testing.T) {
	pb := NewPerfBudget()

	// Accuracy increasing (improving for higher-is-better)
	measurements := []float64{85.0, 86.0, 87.0, 88.0, 92.0, 93.0, 94.0, 95.0}
	for _, m := range measurements {
		pb.Record("accuracy", m)
	}

	trend := pb.ProjectTrend("accuracy")
	if trend != "improving" {
		t.Errorf("expected trend 'improving', got %q", trend)
	}
}

func TestProjectTrendUnknownMetric(t *testing.T) {
	pb := NewPerfBudget()

	trend := pb.ProjectTrend("nonexistent")
	if trend != "stable" {
		t.Errorf("expected 'stable' for unknown metric, got %q", trend)
	}
}

func TestProjectTrendInsufficientData(t *testing.T) {
	pb := NewPerfBudget()

	pb.Record("response_time", 3.0)
	pb.Record("response_time", 3.5)

	trend := pb.ProjectTrend("response_time")
	if trend != "stable" {
		t.Errorf("expected 'stable' for insufficient data, got %q", trend)
	}
}

func TestReset(t *testing.T) {
	pb := NewPerfBudget()

	pb.Record("response_time", 10.0)
	pb.Record("accuracy", 50.0)

	// Ensure we have data and violations before reset
	if len(pb.SLOs["response_time"].Measurements) == 0 {
		t.Fatal("expected measurements before reset")
	}
	if len(pb.Violations) == 0 {
		t.Fatal("expected violations before reset")
	}

	pb.Reset()

	// Verify everything is cleared
	for metric, slo := range pb.SLOs {
		if len(slo.Measurements) != 0 {
			t.Errorf("SLO %s: expected 0 measurements after reset, got %d", metric, len(slo.Measurements))
		}
		if slo.Current != 0 {
			t.Errorf("SLO %s: expected current=0 after reset, got %f", metric, slo.Current)
		}
		if slo.Status != "met" {
			t.Errorf("SLO %s: expected status 'met' after reset, got %q", metric, slo.Status)
		}
	}

	if len(pb.Violations) != 0 {
		t.Errorf("expected 0 violations after reset, got %d", len(pb.Violations))
	}
}

func TestConcurrentAccess(t *testing.T) {
	pb := NewPerfBudget()

	done := make(chan struct{})

	// Concurrent writes
	go func() {
		for i := 0; i < 100; i++ {
			pb.Record("response_time", float64(i)*0.1)
		}
		done <- struct{}{}
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < 100; i++ {
			pb.Check()
			pb.FormatDashboard()
			pb.ProjectTrend("response_time")
		}
		done <- struct{}{}
	}()

	<-done
	<-done
}

func TestPercentile(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	p50 := percentile(data, 50)
	if p50 < 5.0 || p50 > 6.0 {
		t.Errorf("expected P50 around 5.5, got %f", p50)
	}

	p95 := percentile(data, 95)
	if p95 < 9.0 || p95 > 10.0 {
		t.Errorf("expected P95 around 9.55, got %f", p95)
	}
}

func TestPercentileEmpty(t *testing.T) {
	result := percentile([]float64{}, 95)
	if result != 0 {
		t.Errorf("expected 0 for empty data, got %f", result)
	}
}

func TestAverage(t *testing.T) {
	data := []float64{10, 20, 30, 40, 50}
	avg := average(data)
	if avg != 30.0 {
		t.Errorf("expected average 30.0, got %f", avg)
	}
}

func TestAverageEmpty(t *testing.T) {
	avg := average([]float64{})
	if avg != 0 {
		t.Errorf("expected 0 for empty data, got %f", avg)
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{100, "100"},
		{1000, "1,000"},
		{4200, "4,200"},
		{12345, "12,345"},
		{1000000, "1,000,000"},
	}

	for _, tt := range tests {
		result := formatNumber(tt.input)
		if result != tt.expected {
			t.Errorf("formatNumber(%f) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestMultipleViolationsRecorded(t *testing.T) {
	pb := NewPerfBudget()

	pb.Record("response_time", 10.0)
	pb.Record("cost_per_task", 1.0)

	if len(pb.Violations) < 2 {
		t.Errorf("expected at least 2 violations, got %d", len(pb.Violations))
	}
}

func TestFormatDashboardOverallSummary(t *testing.T) {
	pb := NewPerfBudget()

	// All within budget
	pb.Record("response_time", 2.0)
	pb.Record("accuracy", 95.0)
	pb.Record("cost_per_task", 0.20)
	pb.Record("token_efficiency", 3000.0)
	pb.Record("tool_error_rate", 2.0)

	dashboard := pb.FormatDashboard()
	if !strings.Contains(dashboard, "5/5 SLOs met") {
		t.Errorf("expected '5/5 SLOs met' in dashboard, got:\n%s", dashboard)
	}
}
