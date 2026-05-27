package cmd

import (
	"testing"

	inspectLib "github.com/GrayCodeAI/inspect"
)

func TestInspectToReviewFindings_NilReport(t *testing.T) {
	t.Parallel()
	findings := InspectToReviewFindings(nil)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for nil report, got %d", len(findings))
	}
}

func TestInspectToReviewFindings_EmptyReport(t *testing.T) {
	t.Parallel()
	report := &inspectLib.Report{
		Target: "https://example.com",
	}
	findings := InspectToReviewFindings(report)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty report, got %d", len(findings))
	}
}

func TestInspectToReviewFindings_WithFindings(t *testing.T) {
	t.Parallel()
	report := &inspectLib.Report{
		Target: "https://example.com",
		Findings: []inspectLib.Finding{
			{
				Check:    "security",
				URL:      "https://example.com/login",
				Severity: inspectLib.SeverityHigh,
				Message:  "Missing CSP header",
				Fix:      "Add Content-Security-Policy header",
			},
			{
				Check:    "a11y",
				URL:      "https://example.com/about",
				Severity: inspectLib.SeverityMedium,
				Message:  "Missing alt text on image",
			},
		},
	}

	findings := InspectToReviewFindings(report)
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}

	if findings[0].Concern != "inspect:security" {
		t.Errorf("expected concern 'inspect:security', got %q", findings[0].Concern)
	}
	if findings[0].Severity != "high" {
		t.Errorf("expected severity 'high', got %q", findings[0].Severity)
	}
	if findings[0].Fix != "Add Content-Security-Policy header" {
		t.Errorf("expected fix, got %q", findings[0].Fix)
	}
	if findings[1].Concern != "inspect:a11y" {
		t.Errorf("expected concern 'inspect:a11y', got %q", findings[1].Concern)
	}
}

func TestMapInspectSeverity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input inspectLib.Severity
		want  string
	}{
		{inspectLib.SeverityCritical, "critical"},
		{inspectLib.SeverityHigh, "high"},
		{inspectLib.SeverityMedium, "medium"},
		{inspectLib.SeverityLow, "low"},
		{inspectLib.Severity(999), "low"},
	}

	for _, tt := range tests {
		got := mapInspectSeverity(tt.input)
		if got != tt.want {
			t.Errorf("mapInspectSeverity(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMergeInspectWithReview(t *testing.T) {
	t.Parallel()
	review := []ReviewFinding{
		{Concern: "security", Severity: "high", File: "auth.go", Line: 5, Message: "SQL injection"},
	}
	inspect := []ReviewFinding{
		{Concern: "inspect:security", Severity: "critical", File: "https://example.com", Message: "Missing CSP"},
		{Concern: "inspect:a11y", Severity: "medium", File: "https://example.com/about", Message: "No alt text"},
	}

	merged := MergeInspectWithReview(review, inspect)
	if len(merged) != 3 {
		t.Errorf("expected 3 merged findings, got %d", len(merged))
	}
}

func TestMergeInspectWithReview_Deduplication(t *testing.T) {
	t.Parallel()
	review := []ReviewFinding{
		{Concern: "security", Severity: "high", File: "auth.go", Message: "issue"},
	}
	inspect := []ReviewFinding{
		{Concern: "inspect:security", Severity: "high", File: "auth.go", Message: "issue"},
	}

	merged := MergeInspectWithReview(review, inspect)
	if len(merged) != 1 {
		t.Errorf("expected 1 merged finding (deduped), got %d", len(merged))
	}
}

func TestDefaultInspectPipelineConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultInspectPipelineConfig("https://example.com")
	if cfg.Target != "https://example.com" {
		t.Errorf("expected target URL, got %q", cfg.Target)
	}
	if cfg.Depth <= 0 {
		t.Error("depth should be positive")
	}
	if len(cfg.Checks) == 0 {
		t.Error("should have default checks")
	}
}

func TestFormatInspectReport_Nil(t *testing.T) {
	t.Parallel()
	output := formatInspectReport(nil)
	if output == "" {
		t.Error("should produce output for nil report")
	}
}

func TestFormatInspectReport_WithData(t *testing.T) {
	t.Parallel()
	report := &inspectLib.Report{
		Target:      "https://example.com",
		CrawledURLs: 10,
		Stats: inspectLib.ReportStats{
			FindingsTotal: 5,
			BySeverity:    map[string]int{"high": 2, "medium": 3},
		},
	}
	output := formatInspectReport(report)
	if output == "" {
		t.Error("should produce output")
	}
}
