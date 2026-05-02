package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestReviewBeforeWrite_Approved(t *testing.T) {
	mock := &mockLLMClient{
		response: `APPROVED: yes
CONFIDENCE: 0.95
ISSUES: none
SUGGESTIONS: none`,
	}

	result, err := ReviewBeforeWrite(context.Background(), mock, "test-model",
		"add error handling",
		"main.go",
		"func run() { doStuff() }",
		"func run() { if err := doStuff(); err != nil { return err } }",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Approved {
		t.Error("expected Approved=true")
	}
	if result.Confidence != 0.95 {
		t.Errorf("expected confidence=0.95, got %f", result.Confidence)
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected no issues, got %v", result.Issues)
	}
	if len(result.Suggestions) != 0 {
		t.Errorf("expected no suggestions, got %v", result.Suggestions)
	}
}

func TestReviewBeforeWrite_Rejected(t *testing.T) {
	mock := &mockLLMClient{
		response: `APPROVED: no
CONFIDENCE: 0.85
ISSUES: missing nil check on input, return type changed without updating callers
SUGGESTIONS: add nil guard, update call sites`,
	}

	result, err := ReviewBeforeWrite(context.Background(), mock, "test-model",
		"refactor function",
		"handler.go",
		"func handle(r *Request) {}",
		"func handle(r *Request) error { return nil }",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Approved {
		t.Error("expected Approved=false")
	}
	if result.Confidence != 0.85 {
		t.Errorf("expected confidence=0.85, got %f", result.Confidence)
	}
	if len(result.Issues) != 2 {
		t.Errorf("expected 2 issues, got %d: %v", len(result.Issues), result.Issues)
	}
	if len(result.Suggestions) != 2 {
		t.Errorf("expected 2 suggestions, got %d: %v", len(result.Suggestions), result.Suggestions)
	}
}

func TestReviewBeforeWrite_LowConfidenceOverride(t *testing.T) {
	// Even if the model says "approved", low confidence should override.
	mock := &mockLLMClient{
		response: `APPROVED: yes
CONFIDENCE: 0.5
ISSUES: none
SUGGESTIONS: none`,
	}

	result, err := ReviewBeforeWrite(context.Background(), mock, "test-model",
		"complex refactor",
		"engine.go",
		"// old code",
		"// new code",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Approved {
		t.Error("expected Approved=false when confidence < 0.7")
	}
	if len(result.Suggestions) == 0 {
		t.Error("expected suggestion about low confidence")
	}
	found := false
	for _, s := range result.Suggestions {
		if strings.Contains(s, "Low confidence") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Low confidence' suggestion, got %v", result.Suggestions)
	}
}

func TestReviewBeforeWrite_IssuesForceReject(t *testing.T) {
	// Even if the model says "approved", having issues should force rejection.
	mock := &mockLLMClient{
		response: `APPROVED: yes
CONFIDENCE: 0.9
ISSUES: introduces a data race on shared counter
SUGGESTIONS: add mutex`,
	}

	result, err := ReviewBeforeWrite(context.Background(), mock, "test-model",
		"add counter",
		"counter.go",
		"var count int",
		"var count int\nfunc inc() { count++ }",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Approved {
		t.Error("expected Approved=false when issues are present, even if model said yes")
	}
	if len(result.Issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(result.Issues))
	}
}

func TestReviewBeforeWrite_NilClient(t *testing.T) {
	_, err := ReviewBeforeWrite(context.Background(), nil, "test-model",
		"intent", "file.go", "old", "new")
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if !strings.Contains(err.Error(), "no LLM client") {
		t.Errorf("expected 'no LLM client' error, got %q", err.Error())
	}
}

func TestReviewBeforeWrite_LLMError(t *testing.T) {
	mock := &mockLLMClient{err: fmt.Errorf("service unavailable")}
	_, err := ReviewBeforeWrite(context.Background(), mock, "test-model",
		"intent", "file.go", "old", "new")
	if err == nil {
		t.Fatal("expected error from LLM failure")
	}
	if !strings.Contains(err.Error(), "service unavailable") {
		t.Errorf("expected 'service unavailable' in error, got %q", err.Error())
	}
}

func TestReviewBeforeWrite_EmptyResponse(t *testing.T) {
	mock := &mockLLMClient{response: ""}
	_, err := ReviewBeforeWrite(context.Background(), mock, "test-model",
		"intent", "file.go", "old", "new")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
	if !strings.Contains(err.Error(), "empty response") {
		t.Errorf("expected 'empty response' error, got %q", err.Error())
	}
}

func TestParseSelfReview_AllFields(t *testing.T) {
	input := `APPROVED: yes
CONFIDENCE: 0.88
ISSUES: none
SUGGESTIONS: consider adding a comment explaining the algorithm`

	result := parseSelfReview(input)
	if !result.Approved {
		t.Error("expected Approved=true")
	}
	if result.Confidence != 0.88 {
		t.Errorf("expected confidence=0.88, got %f", result.Confidence)
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected no issues, got %v", result.Issues)
	}
	if len(result.Suggestions) != 1 {
		t.Errorf("expected 1 suggestion, got %d: %v", len(result.Suggestions), result.Suggestions)
	}
}

func TestParseSelfReview_MultipleIssues(t *testing.T) {
	input := `APPROVED: no
CONFIDENCE: 0.75
ISSUES: missing error return, unused variable, wrong import path
SUGGESTIONS: fix the three issues above`

	result := parseSelfReview(input)
	if result.Approved {
		t.Error("expected Approved=false")
	}
	if len(result.Issues) != 3 {
		t.Errorf("expected 3 issues, got %d: %v", len(result.Issues), result.Issues)
	}
}

func TestParseSelfReview_DefaultValues(t *testing.T) {
	// Completely unparseable response should yield defaults.
	result := parseSelfReview("This change looks fine to me.")
	if result.Approved {
		t.Error("default should be not approved")
	}
	if result.Confidence != 0.5 {
		t.Errorf("default confidence should be 0.5, got %f", result.Confidence)
	}
}

func TestParseSelfReview_InvalidConfidence(t *testing.T) {
	input := `APPROVED: yes
CONFIDENCE: 2.5
ISSUES: none
SUGGESTIONS: none`

	result := parseSelfReview(input)
	// Invalid confidence (>1.0) should not be accepted; stays at default.
	if result.Confidence != 0.5 {
		t.Errorf("expected default confidence for out-of-range value, got %f", result.Confidence)
	}
}

func TestBuildSelfReviewPrompt_ContainsKeyElements(t *testing.T) {
	prompt := buildSelfReviewPrompt(
		"add logging to handler",
		"server/handler.go",
		"func Handle(w http.ResponseWriter, r *http.Request) {\n\tw.Write([]byte(\"ok\"))\n}",
		"func Handle(w http.ResponseWriter, r *http.Request) {\n\tlog.Printf(\"handling %s\", r.URL)\n\tw.Write([]byte(\"ok\"))\n}",
	)

	checks := []string{
		"add logging to handler",
		"server/handler.go",
		"BEFORE",
		"AFTER",
		"APPROVED",
		"CONFIDENCE",
		"ISSUES",
		"SUGGESTIONS",
		"edge cases",
		"regressions",
		"http.ResponseWriter",
		"log.Printf",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("prompt should contain %q", check)
		}
	}
}

func TestTruncateForReview_ShortContent(t *testing.T) {
	content := "short content"
	result := truncateForReview(content, 1000)
	if result != content {
		t.Errorf("short content should not be truncated, got %q", result)
	}
}

func TestTruncateForReview_LongContent(t *testing.T) {
	content := strings.Repeat("x", 5000)
	result := truncateForReview(content, 100)
	if len(result) >= len(content) {
		t.Error("long content should be truncated")
	}
	if !strings.Contains(result, "truncated") {
		t.Error("truncated content should contain truncation marker")
	}
}

func TestReviewBeforeWrite_ExactThreshold(t *testing.T) {
	// Confidence exactly at the threshold should still be approved.
	mock := &mockLLMClient{
		response: `APPROVED: yes
CONFIDENCE: 0.7
ISSUES: none
SUGGESTIONS: none`,
	}

	result, err := ReviewBeforeWrite(context.Background(), mock, "test-model",
		"intent", "file.go", "old", "new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Approved {
		t.Error("confidence=0.7 should be approved (at threshold)")
	}
}

func TestReviewBeforeWrite_JustBelowThreshold(t *testing.T) {
	mock := &mockLLMClient{
		response: `APPROVED: yes
CONFIDENCE: 0.69
ISSUES: none
SUGGESTIONS: none`,
	}

	result, err := ReviewBeforeWrite(context.Background(), mock, "test-model",
		"intent", "file.go", "old", "new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Approved {
		t.Error("confidence=0.69 should not be approved (below threshold)")
	}
}
