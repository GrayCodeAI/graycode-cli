package prompt

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// mockLLMClient implements LLMClient for testing
type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) Chat(ctx context.Context, msgs []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &types.EyrieResponse{Content: m.response}, nil
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{0.0, "0.0"},
		{1.5, "1.50"},
		{3.14, "3.14"},
		{123.45, "123.45"},
	}
	for _, tt := range tests {
		result := formatFloat(tt.input)
		if result != tt.expected {
			t.Errorf("formatFloat(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestItoa2(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{99, "99"},
		{100, "100"},
		{-1, "-1"},
		{-10, "-10"},
	}
	for _, tt := range tests {
		result := itoa2(tt.input)
		if result != tt.expected {
			t.Errorf("itoa2(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestOptimizePrompt(t *testing.T) {
	po := NewPromptOptimizer()
	po.Parameters["test_param"] = &PromptParameter{
		Name:  "test_param",
		Value: "original value",
	}

	llm := &mockLLMClient{response: "optimized value"}

	result, err := OptimizePrompt(context.Background(), llm, "test-model", po, "test_param", "needs improvement")
	if err != nil {
		t.Fatalf("OptimizePrompt error: %v", err)
	}
	if result != "optimized value" {
		t.Errorf("OptimizePrompt result = %q, want %q", result, "optimized value")
	}
}

func TestOptimizePrompt_ParameterNotFound(t *testing.T) {
	po := NewPromptOptimizer()
	llm := &mockLLMClient{response: "optimized"}

	_, err := OptimizePrompt(context.Background(), llm, "test-model", po, "nonexistent", "feedback")
	if err == nil {
		t.Error("expected error for non-existent parameter")
	}
}

func TestOptimizePrompt_LLMError(t *testing.T) {
	po := NewPromptOptimizer()
	po.Parameters["test_param"] = &PromptParameter{
		Name:  "test_param",
		Value: "original value",
	}

	llm := &mockLLMClient{err: context.DeadlineExceeded}

	_, err := OptimizePrompt(context.Background(), llm, "test-model", po, "test_param", "feedback")
	if err == nil {
		t.Error("expected error from LLM")
	}
}
