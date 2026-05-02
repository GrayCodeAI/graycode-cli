package cmd

import (
	"strings"
	"testing"
	"time"
)

func TestRunMetricExtractsScore(t *testing.T) {
	// Use echo to produce a known numeric output
	score, output, err := RunMetric("echo 'accuracy: 0.95'", 10*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0.95 {
		t.Errorf("expected score 0.95, got %f", score)
	}
	if !strings.Contains(output, "accuracy") {
		t.Errorf("expected output to contain 'accuracy', got %q", output)
	}
}

func TestRunMetricFailsOnBadCommand(t *testing.T) {
	_, _, err := RunMetric("false", 10*time.Second)
	if err == nil {
		t.Error("expected error for failing command")
	}
}

func TestRunMetricFailsNoNumber(t *testing.T) {
	_, _, err := RunMetric("echo 'no numbers here'", 10*time.Second)
	if err == nil {
		t.Error("expected error when no numeric value in output")
	}
	if !strings.Contains(err.Error(), "no numeric value") {
		t.Errorf("expected 'no numeric value' error, got: %v", err)
	}
}

func TestExtractLastFloat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
		err   bool
	}{
		{"score: 0.95", 0.95, false},
		{"iter 1: 0.80\niter 2: 0.92", 0.92, false},
		{"BenchmarkSort-8  12345 ns/op", 12345, false},
		{"loss=1.5e-3", 0.0015, false},
		{"no numbers", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := extractLastFloat(tt.input)
		if tt.err && err == nil {
			t.Errorf("extractLastFloat(%q): expected error", tt.input)
			continue
		}
		if !tt.err && err != nil {
			t.Errorf("extractLastFloat(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if !tt.err && got != tt.want {
			t.Errorf("extractLastFloat(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestFormatResearchReport(t *testing.T) {
	results := []ResearchResult{
		{Iteration: 1, Score: 0.80, Improved: false, Description: "baseline", Duration: 2 * time.Second},
		{Iteration: 2, Score: 0.85, Improved: true, Description: "improved 0.80 -> 0.85", Duration: 3 * time.Second},
		{Iteration: 3, Score: 0.83, Improved: false, Description: "no improvement", Duration: 2 * time.Second},
	}

	report := FormatResearchReport(results)
	if !strings.Contains(report, "Research Report") {
		t.Error("report should contain header")
	}
	if !strings.Contains(report, "3 iterations") {
		t.Error("report should contain iteration count")
	}
	if !strings.Contains(report, "1 improvements") {
		t.Error("report should contain improvement count")
	}
	if !strings.Contains(report, "0.85") {
		t.Error("report should contain best score")
	}
}

func TestFormatResearchReportEmpty(t *testing.T) {
	report := FormatResearchReport(nil)
	if !strings.Contains(report, "No research iterations") {
		t.Error("empty report should indicate no iterations")
	}
}

func TestDefaultResearchConfig(t *testing.T) {
	cfg := DefaultResearchConfig()
	if cfg.Budget != 5*time.Minute {
		t.Errorf("expected 5m budget, got %s", cfg.Budget)
	}
	if cfg.MaxIterations != 50 {
		t.Errorf("expected 50 max iterations, got %d", cfg.MaxIterations)
	}
	if !cfg.KeepBest {
		t.Error("expected KeepBest to be true")
	}
}
