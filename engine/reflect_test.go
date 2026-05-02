package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client"
)

// mockLLMClient implements LLMClient for testing.
type mockLLMClient struct {
	response string
	err      error
	calls    int
}

func (m *mockLLMClient) Chat(_ context.Context, msgs []client.EyrieMessage, _ client.ChatOptions) (*client.EyrieResponse, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return &client.EyrieResponse{Content: m.response}, nil
}

func TestNewReflector(t *testing.T) {
	mock := &mockLLMClient{}
	r := NewReflector(mock, "test-model")
	if r == nil {
		t.Fatal("expected non-nil reflector")
	}
	if len(r.History()) != 0 {
		t.Errorf("new reflector should have empty history, got %d", len(r.History()))
	}
}

func TestReflect_Success(t *testing.T) {
	mock := &mockLLMClient{
		response: `WHAT_FAILED: The file write was rejected because the path was outside the allowed directory.
WHY_FAILED: The tool constructed an absolute path using the wrong base directory, leading to a permission denial.
WHAT_TO_DO: Use the project root from the session context instead of hardcoding /tmp as the base path.`,
	}

	r := NewReflector(mock, "test-model")
	ref, err := r.Reflect(context.Background(), "fix the config parser", []client.EyrieMessage{
		{Role: "user", Content: "Fix the config parser to handle nested keys."},
		{Role: "assistant", Content: "I will edit config.go."},
	}, "permission denied: /tmp/config.go")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref == nil {
		t.Fatal("expected non-nil reflection")
	}
	if ref.Attempt != 1 {
		t.Errorf("expected attempt=1, got %d", ref.Attempt)
	}
	if ref.TaskGoal != "fix the config parser" {
		t.Errorf("expected goal='fix the config parser', got %q", ref.TaskGoal)
	}
	if !strings.Contains(ref.WhatFailed, "file write was rejected") {
		t.Errorf("WhatFailed should mention rejection, got %q", ref.WhatFailed)
	}
	if !strings.Contains(ref.WhyFailed, "wrong base directory") {
		t.Errorf("WhyFailed should mention wrong base directory, got %q", ref.WhyFailed)
	}
	if !strings.Contains(ref.WhatToDo, "project root") {
		t.Errorf("WhatToDo should mention project root, got %q", ref.WhatToDo)
	}
	if ref.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", mock.calls)
	}
}

func TestReflect_AccumulatesHistory(t *testing.T) {
	mock := &mockLLMClient{
		response: "WHAT_FAILED: first failure\nWHY_FAILED: reason one\nWHAT_TO_DO: fix one",
	}
	r := NewReflector(mock, "test-model")

	_, err := r.Reflect(context.Background(), "goal A", nil, "error A")
	if err != nil {
		t.Fatalf("reflect 1: %v", err)
	}

	mock.response = "WHAT_FAILED: second failure\nWHY_FAILED: reason two\nWHAT_TO_DO: fix two"
	_, err = r.Reflect(context.Background(), "goal B", nil, "error B")
	if err != nil {
		t.Fatalf("reflect 2: %v", err)
	}

	history := r.History()
	if len(history) != 2 {
		t.Fatalf("expected 2 reflections, got %d", len(history))
	}
	if history[0].Attempt != 1 {
		t.Errorf("first reflection attempt should be 1, got %d", history[0].Attempt)
	}
	if history[1].Attempt != 2 {
		t.Errorf("second reflection attempt should be 2, got %d", history[1].Attempt)
	}
}

func TestReflect_NilClient(t *testing.T) {
	r := &Reflector{model: "test"}
	_, err := r.Reflect(context.Background(), "goal", nil, "error")
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if !strings.Contains(err.Error(), "no LLM client") {
		t.Errorf("expected 'no LLM client' error, got %q", err.Error())
	}
}

func TestReflect_LLMError(t *testing.T) {
	mock := &mockLLMClient{err: fmt.Errorf("rate limited")}
	r := NewReflector(mock, "test-model")

	_, err := r.Reflect(context.Background(), "goal", nil, "error")
	if err == nil {
		t.Fatal("expected error from LLM failure")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("expected 'rate limited' in error, got %q", err.Error())
	}
}

func TestReflect_EmptyResponse(t *testing.T) {
	mock := &mockLLMClient{response: ""}
	r := NewReflector(mock, "test-model")

	_, err := r.Reflect(context.Background(), "goal", nil, "error")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
	if !strings.Contains(err.Error(), "empty response") {
		t.Errorf("expected 'empty response' error, got %q", err.Error())
	}
}

func TestReflect_UnstructuredResponse(t *testing.T) {
	mock := &mockLLMClient{
		response: "The approach was fundamentally wrong because it tried to parse JSON as YAML.",
	}
	r := NewReflector(mock, "test-model")

	ref, err := r.Reflect(context.Background(), "parse config", nil, "parse error")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should gracefully fall back to using the whole response.
	if ref.WhatFailed == "" {
		t.Error("WhatFailed should not be empty even for unstructured response")
	}
	if ref.WhyFailed == "" {
		t.Error("WhyFailed should have a fallback value")
	}
}

func TestInjectReflections_Empty(t *testing.T) {
	r := NewReflector(&mockLLMClient{}, "test-model")
	result := r.InjectReflections()
	if result != "" {
		t.Errorf("expected empty injection for no reflections, got %q", result)
	}
}

func TestInjectReflections_WithHistory(t *testing.T) {
	r := NewReflector(&mockLLMClient{
		response: "WHAT_FAILED: test\nWHY_FAILED: test\nWHAT_TO_DO: test",
	}, "test-model")

	r.Reflect(context.Background(), "goal 1", nil, "error 1")
	r.Reflect(context.Background(), "goal 2", nil, "error 2")

	injection := r.InjectReflections()

	if !strings.Contains(injection, "REFLECTIONS FROM PREVIOUS ATTEMPTS") {
		t.Error("injection should contain header")
	}
	if !strings.Contains(injection, "Attempt 1") {
		t.Error("injection should contain attempt 1")
	}
	if !strings.Contains(injection, "Attempt 2") {
		t.Error("injection should contain attempt 2")
	}
	if !strings.Contains(injection, "goal 1") {
		t.Error("injection should contain goal 1")
	}
	if !strings.Contains(injection, "goal 2") {
		t.Error("injection should contain goal 2")
	}
}

func TestReflectorReset(t *testing.T) {
	r := NewReflector(&mockLLMClient{
		response: "WHAT_FAILED: test\nWHY_FAILED: test\nWHAT_TO_DO: test",
	}, "test-model")

	r.Reflect(context.Background(), "goal", nil, "error")
	if len(r.History()) != 1 {
		t.Fatal("expected 1 reflection before reset")
	}

	r.Reset()
	if len(r.History()) != 0 {
		t.Errorf("expected 0 reflections after reset, got %d", len(r.History()))
	}
	if r.InjectReflections() != "" {
		t.Error("injection should be empty after reset")
	}
}

func TestParseReflection_AllFields(t *testing.T) {
	input := `WHAT_FAILED: The function returned nil instead of an error.
WHY_FAILED: Missing nil check on the input parameter.
WHAT_TO_DO: Add a nil guard at the top of the function.`

	ref := parseReflection(input)

	if ref.WhatFailed != "The function returned nil instead of an error." {
		t.Errorf("unexpected WhatFailed: %q", ref.WhatFailed)
	}
	if ref.WhyFailed != "Missing nil check on the input parameter." {
		t.Errorf("unexpected WhyFailed: %q", ref.WhyFailed)
	}
	if ref.WhatToDo != "Add a nil guard at the top of the function." {
		t.Errorf("unexpected WhatToDo: %q", ref.WhatToDo)
	}
}

func TestParseReflection_CaseInsensitive(t *testing.T) {
	input := `what_failed: lowercase test
why_failed: lowercase reason
what_to_do: lowercase fix`

	ref := parseReflection(input)

	if ref.WhatFailed != "lowercase test" {
		t.Errorf("case-insensitive parsing failed for WhatFailed: %q", ref.WhatFailed)
	}
	if ref.WhyFailed != "lowercase reason" {
		t.Errorf("case-insensitive parsing failed for WhyFailed: %q", ref.WhyFailed)
	}
	if ref.WhatToDo != "lowercase fix" {
		t.Errorf("case-insensitive parsing failed for WhatToDo: %q", ref.WhatToDo)
	}
}

func TestBuildReflectionPrompt_ContainsKeyElements(t *testing.T) {
	messages := []client.EyrieMessage{
		{Role: "user", Content: "Fix the parser."},
		{
			Role:    "assistant",
			Content: "I will edit parser.go.",
			ToolUse: []client.ToolCall{
				{Name: "FileWrite", Arguments: map[string]interface{}{"path": "parser.go"}},
			},
		},
		{
			Role: "user",
			ToolResult: &client.ToolResult{
				ToolUseID: "tc1",
				Content:   "Error: syntax error on line 42",
				IsError:   true,
			},
		},
	}

	prompt := buildReflectionPrompt("fix the parser", messages, "syntax error on line 42")

	checks := []string{
		"fix the parser",
		"TASK GOAL",
		"CONVERSATION TRANSCRIPT",
		"FINAL ERROR",
		"syntax error",
		"WHAT_FAILED",
		"WHY_FAILED",
		"WHAT_TO_DO",
		"FileWrite",
		"[tool_result ERROR]",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("prompt should contain %q", check)
		}
	}
}

func TestReflect_ContextCanceled(t *testing.T) {
	mock := &mockLLMClient{err: context.Canceled}
	r := NewReflector(mock, "test-model")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Reflect(ctx, "goal", nil, "error")
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}
