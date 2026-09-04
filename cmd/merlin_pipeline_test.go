package cmd

import (
	"testing"

	contracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/types"
	verifycontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/verify"
)

func TestMerlinToReviewFindings_NilReport(t *testing.T) {
	t.Parallel()
	findings := MerlinToReviewFindings(nil)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for nil report, got %d", len(findings))
	}
}

func TestMerlinToReviewFindings_EmptyReport(t *testing.T) {
	t.Parallel()
	report := &verifycontracts.Report{
		Target: "https://example.com",
	}
	findings := MerlinToReviewFindings(report)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty report, got %d", len(findings))
	}
}

func TestMerlinToReviewFindings_WithFindings(t *testing.T) {
	t.Parallel()
	report := &verifycontracts.Report{
		Target: "https://example.com",
		Findings: []verifycontracts.Finding{
			{
				Check:    "security",
				URL:      "https://example.com/login",
				Severity: contracts.SeverityHigh,
				Message:  "Missing CSP header",
				Fix:      "Add Content-Security-Policy header",
			},
			{
				Check:    "a11y",
				URL:      "https://example.com/about",
				Severity: contracts.SeverityMedium,
				Message:  "Missing alt text on image",
			},
		},
	}

	findings := MerlinToReviewFindings(report)
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}

	if findings[0].Concern != "merlin:security" {
		t.Errorf("expected concern 'merlin:security', got %q", findings[0].Concern)
	}
	if findings[0].Severity != "high" {
		t.Errorf("expected severity 'high', got %q", findings[0].Severity)
	}
	if findings[0].Fix != "Add Content-Security-Policy header" {
		t.Errorf("expected fix, got %q", findings[0].Fix)
	}
	if findings[1].Concern != "merlin:a11y" {
		t.Errorf("expected concern 'merlin:a11y', got %q", findings[1].Concern)
	}
}

func TestMapMerlinSeverity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input contracts.Severity
		want  string
	}{
		{contracts.SeverityCritical, "critical"},
		{contracts.SeverityHigh, "high"},
		{contracts.SeverityMedium, "medium"},
		{contracts.SeverityLow, "low"},
		{contracts.Severity(999), "low"},
	}

	for _, tt := range tests {
		got := mapMerlinSeverity(tt.input)
		if got != tt.want {
			t.Errorf("mapMerlinSeverity(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMergeMerlinWithReview(t *testing.T) {
	t.Parallel()
	review := []ReviewFinding{
		{Concern: "security", Severity: "high", File: "auth.go", Line: 5, Message: "SQL injection"},
	}
	merlin := []ReviewFinding{
		{Concern: "merlin:security", Severity: "critical", File: "https://example.com", Message: "Missing CSP"},
		{Concern: "merlin:a11y", Severity: "medium", File: "https://example.com/about", Message: "No alt text"},
	}

	merged := MergeMerlinWithReview(review, merlin)
	if len(merged) != 3 {
		t.Errorf("expected 3 merged findings, got %d", len(merged))
	}
}

func TestMergeMerlinWithReview_Deduplication(t *testing.T) {
	t.Parallel()
	review := []ReviewFinding{
		{Concern: "security", Severity: "high", File: "auth.go", Message: "issue"},
	}
	merlin := []ReviewFinding{
		{Concern: "merlin:security", Severity: "high", File: "auth.go", Message: "issue"},
	}

	merged := MergeMerlinWithReview(review, merlin)
	if len(merged) != 1 {
		t.Errorf("expected 1 merged finding (deduped), got %d", len(merged))
	}
}

func TestDefaultMerlinPipelineConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultMerlinPipelineConfig("https://example.com")
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

func TestFormatMerlinReport_Nil(t *testing.T) {
	t.Parallel()
	output := formatMerlinReport(nil)
	if output == "" {
		t.Error("should produce output for nil report")
	}
}

func TestFormatMerlinReport_WithData(t *testing.T) {
	t.Parallel()
	report := &verifycontracts.Report{
		Target:      "https://example.com",
		CrawledURLs: 10,
		Stats: verifycontracts.Stats{
			FindingsTotal: 5,
			BySeverity:    map[contracts.Severity]int{contracts.SeverityHigh: 2, contracts.SeverityMedium: 3},
		},
	}
	output := formatMerlinReport(report)
	if output == "" {
		t.Error("should produce output")
	}
}
