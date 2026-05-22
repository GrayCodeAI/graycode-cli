package cmd

import (
	"context"
	"testing"
)

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
	mockChat := func(_ context.Context, _ string) (string, error) {
		return `[{"file":"handler.go","line":15,"severity":"high","message":"nil deref","fix":"add nil check"}]`, nil
	}
	findings := reviewForConcern(context.Background(), []string{"handler.go"}, concern, mockChat)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != "high" {
		t.Errorf("expected high severity, got %q", findings[0].Severity)
	}
}

func TestParseReviewFindings_EmptyArray(t *testing.T) {
	t.Parallel()
	findings := parseReviewFindings("[]", "security")
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestParseReviewFindings_NoIssues(t *testing.T) {
	t.Parallel()
	findings := parseReviewFindings("No issues found.", "bugs")
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for 'No issues found', got %d", len(findings))
	}
}

func TestParseReviewFindings_Fallback(t *testing.T) {
	t.Parallel()
	findings := parseReviewFindings("The code has some issues with error handling", "style")
	if len(findings) != 1 {
		t.Fatalf("expected 1 fallback finding, got %d", len(findings))
	}
	if findings[0].Concern != "style" {
		t.Errorf("expected concern 'style', got %q", findings[0].Concern)
	}
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
