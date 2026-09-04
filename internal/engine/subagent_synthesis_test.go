package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestSynthesizeSubAgent(t *testing.T) {
	t.Run("tools disabled and prompt appended", func(t *testing.T) {
		mock := &synthesisMockClient{response: "Here is my summary of findings."}
		conversation := []types.GraycodeRouterMessage{
			{Role: "user", Content: "Find all Go files with errors"},
			{Role: "assistant", Content: "I'll search for Go files."},
		}

		result, err := SynthesizeSubAgent(context.Background(), mock, "test-model", conversation)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != mock.response {
			t.Errorf("expected summary text, got: %q", result)
		}
		if mock.calls != 1 {
			t.Fatalf("expected 1 call, got %d", mock.calls)
		}
		if mock.options.Tools != nil {
			t.Errorf("expected Tools to be nil, got: %v", mock.options.Tools)
		}
		if len(mock.messages) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(mock.messages))
		}
		last := mock.messages[len(mock.messages)-1]
		if last.Role != "user" || last.Content != SynthesisPrompt {
			t.Fatalf("last message = %#v, want synthesis prompt", last)
		}
		if mock.messages[0].Content != conversation[0].Content || mock.messages[1].Content != conversation[1].Content {
			t.Fatalf("conversation was not preserved: %#v", mock.messages)
		}
	})

	t.Run("nil client returns error", func(t *testing.T) {
		_, err := SynthesizeSubAgent(context.Background(), nil, "model", []types.GraycodeRouterMessage{{Role: "user", Content: "test"}})
		if err == nil {
			t.Fatal("expected error for nil client")
		}
	})

	t.Run("provider error propagated", func(t *testing.T) {
		mock := &synthesisMockClient{err: errors.New("provider unavailable")}
		_, err := SynthesizeSubAgent(context.Background(), mock, "model", []types.GraycodeRouterMessage{{Role: "user", Content: "test"}})
		if err == nil {
			t.Fatal("expected error from provider")
		}
	})

	t.Run("empty response returns error", func(t *testing.T) {
		_, err := SynthesizeSubAgent(context.Background(), &synthesisMockClient{}, "model", []types.GraycodeRouterMessage{{Role: "user", Content: "test"}})
		if err == nil {
			t.Fatal("expected error for empty response")
		}
	})
}

type synthesisMockClient struct {
	response string
	err      error
	calls    int
	messages []types.GraycodeRouterMessage
	options  types.ChatOptions
}

func (m *synthesisMockClient) Chat(_ context.Context, messages []types.GraycodeRouterMessage, opts types.ChatOptions) (*types.GraycodeRouterResponse, error) {
	m.calls++
	m.messages = append([]types.GraycodeRouterMessage(nil), messages...)
	m.options = opts
	if m.err != nil {
		return nil, m.err
	}
	return &types.GraycodeRouterResponse{Content: m.response, FinishReason: "end_turn"}, nil
}
