package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	analytics "github.com/GrayCodeAI/graycode-cli/internal/observability"
)

func TestCostAnalyze_JSON_Empty(t *testing.T) {
	old := costAnalyzeJSON
	t.Cleanup(func() { costAnalyzeJSON = old })
	costAnalyzeJSON = true

	var buf bytes.Buffer
	costAnalyzeCmd.SetOut(&buf)
	costAnalyzeCmd.SetErr(&buf)

	if err := costAnalyzeCmd.RunE(costAnalyzeCmd, nil); err != nil {
		t.Fatalf("RunE returned error: %v", err)
	}

	var report analytics.OptimizationReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("decoding JSON output %q: %v", buf.String(), err)
	}
	if report.TotalSpend != 0 {
		t.Errorf("expected zero total_spend, got %f", report.TotalSpend)
	}
	if report.ByModel == nil || len(report.ByModel) != 0 {
		t.Errorf("expected empty by_model map, got %v", report.ByModel)
	}
}

func TestCostSummary_JSON_Empty(t *testing.T) {
	old := costSummaryJSON
	t.Cleanup(func() { costSummaryJSON = old })
	costSummaryJSON = true

	var buf bytes.Buffer
	costSummaryCmd.SetOut(&buf)
	costSummaryCmd.SetErr(&buf)

	if err := costSummaryCmd.RunE(costSummaryCmd, nil); err != nil {
		t.Fatalf("RunE returned error: %v", err)
	}

	var report analytics.OptimizationReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("decoding JSON output %q: %v", buf.String(), err)
	}
	if report.YieldRate != 0 {
		t.Errorf("expected zero yield_rate, got %f", report.YieldRate)
	}
}

func TestCostAnalyze_Text_NotJSON(t *testing.T) {
	old := costAnalyzeJSON
	t.Cleanup(func() { costAnalyzeJSON = old })
	costAnalyzeJSON = false

	var buf bytes.Buffer
	costAnalyzeCmd.SetOut(&buf)
	costAnalyzeCmd.SetErr(&buf)

	if err := costAnalyzeCmd.RunE(costAnalyzeCmd, nil); err != nil {
		t.Fatalf("RunE returned error: %v", err)
	}

	out := buf.String()
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Error("text mode should not start with '{'")
	}
	if !strings.Contains(out, "No cost data collected") {
		t.Errorf("expected experimental notice, got: %q", out)
	}
}
