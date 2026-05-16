package cmd

import "testing"

func TestBuildReviewPrompt(t *testing.T) {
	t.Parallel()
	concern := ReviewConcern{
		Name:   "security",
		Prompt: "Check for security vulnerabilities",
	}
	result := buildReviewPrompt([]string{"main.go", "auth.go"}, concern)
	if result == "" {
		t.Error("should produce non-empty prompt")
	}
}

func TestReviewForConcern(t *testing.T) {
	t.Parallel()
	concern := ReviewConcern{
		Name:   "bugs",
		Prompt: "Find bugs",
	}
	findings := reviewForConcern([]string{"handler.go"}, concern)
	_ = findings
}

func TestFormatReviewReport_Empty(t *testing.T) {
	t.Parallel()
	report := FormatReviewReport(nil)
	_ = report
}

func TestFormatReviewReport_WithFindings(t *testing.T) {
	t.Parallel()
	findings := []ReviewFinding{
		{File: "main.go", Line: 10, Severity: "high", Message: "potential nil deref"},
		{File: "config.go", Line: 5, Severity: "low", Message: "unused variable"},
	}
	report := FormatReviewReport(findings)
	if report == "" {
		t.Error("should produce report")
	}
}
