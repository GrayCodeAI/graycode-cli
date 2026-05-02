package cmd

import (
	"strings"
	"testing"
)

func TestDefaultConcerns(t *testing.T) {
	concerns := DefaultConcerns()
	if len(concerns) < 3 {
		t.Fatalf("expected at least 3 default concerns, got %d", len(concerns))
	}

	names := map[string]bool{}
	for _, c := range concerns {
		if c.Name == "" {
			t.Error("concern has empty name")
		}
		if c.Prompt == "" {
			t.Errorf("concern %q has empty prompt", c.Name)
		}
		names[c.Name] = true
	}
	for _, expected := range []string{"security", "bugs", "performance"} {
		if !names[expected] {
			t.Errorf("expected concern %q not found", expected)
		}
	}
}

func TestFormatReviewReport_NoFindings(t *testing.T) {
	report := FormatReviewReport(nil)
	if report != "No issues found." {
		t.Errorf("expected 'No issues found.', got %q", report)
	}
}

func TestFormatReviewReport_SortedBySeverity(t *testing.T) {
	findings := []ReviewFinding{
		{Concern: "style", Severity: "low", File: "main.go", Line: 10, Message: "naming issue"},
		{Concern: "security", Severity: "critical", File: "auth.go", Line: 5, Message: "SQL injection"},
		{Concern: "bugs", Severity: "high", File: "handler.go", Line: 20, Message: "nil deref", Fix: "add nil check"},
	}

	sortBySeverity(findings)
	report := FormatReviewReport(findings)

	if !strings.Contains(report, "=== Review Report ===") {
		t.Error("report should have header")
	}
	if !strings.Contains(report, "3 issue(s) total") {
		t.Error("report should show total count")
	}

	// Verify severity ordering in output
	critIdx := strings.Index(report, "CRITICAL")
	highIdx := strings.Index(report, "HIGH")
	lowIdx := strings.Index(report, "LOW")
	if critIdx >= highIdx || highIdx >= lowIdx {
		t.Error("expected critical before high before low in report")
	}

	if !strings.Contains(report, "Fix: add nil check") {
		t.Error("report should include fix suggestions")
	}
}

func TestDeduplicateFindings(t *testing.T) {
	findings := []ReviewFinding{
		{Concern: "security", Severity: "high", File: "a.go", Line: 1, Message: "issue"},
		{Concern: "bugs", Severity: "high", File: "a.go", Line: 1, Message: "issue"},
		{Concern: "style", Severity: "low", File: "a.go", Line: 2, Message: "other"},
	}
	deduped := deduplicateFindings(findings)
	if len(deduped) != 2 {
		t.Errorf("expected 2 deduplicated findings, got %d", len(deduped))
	}
}

func TestRunReviewPipeline_Empty(t *testing.T) {
	findings, report := RunReviewPipeline(nil, DefaultConcerns())
	if len(findings) != 0 {
		t.Errorf("expected no findings for empty files, got %d", len(findings))
	}
	if !strings.Contains(report, "No files") {
		t.Errorf("expected 'No files' message, got %q", report)
	}
}
