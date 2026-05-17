package engine

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/eyrie/client"
)

func TestSynthesizeSubAgent(t *testing.T) {
	t.Run("tools disabled and prompt appended", func(t *testing.T) {
		mock := client.NewMockProvider(client.MockModeFixed)
		mock.Response = "Here is my summary of findings."

		mockClient := &mockLLMForSynthesis{provider: mock}

		conversation := []client.EyrieMessage{
			{Role: "user", Content: "Find all Go files with errors"},
			{Role: "assistant", Content: "I'll search for Go files."},
		}

		result, err := SynthesizeSubAgent(context.Background(), mockClient, "test-model", conversation)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result != "Here is my summary of findings." {
			t.Errorf("expected summary text, got: %q", result)
		}

		// Verify the mock was called exactly once.
		if mock.CallCount() != 1 {
			t.Fatalf("expected 1 call, got %d", mock.CallCount())
		}

		call := mock.LastCall()

		// Verify tools are nil (disabled).
		if call.Options.Tools != nil {
			t.Errorf("expected Tools to be nil, got: %v", call.Options.Tools)
		}

		// Verify synthesis prompt was appended as last user message.
		lastMsg := call.Messages[len(call.Messages)-1]
		if lastMsg.Role != "user" {
			t.Errorf("expected last message role 'user', got: %q", lastMsg.Role)
		}
		if lastMsg.Content != SynthesisPrompt {
			t.Errorf("expected SynthesisPrompt in last message, got: %q", lastMsg.Content)
		}

		// Verify conversation messages are preserved before the synthesis prompt.
		if len(call.Messages) != 3 {
			t.Fatalf("expected 3 messages (2 conversation + 1 prompt), got %d", len(call.Messages))
		}
		if call.Messages[0].Content != "Find all Go files with errors" {
			t.Errorf("first message not preserved: %q", call.Messages[0].Content)
		}
		if call.Messages[1].Content != "I'll search for Go files." {
			t.Errorf("second message not preserved: %q", call.Messages[1].Content)
		}
	})

	t.Run("nil client returns error", func(t *testing.T) {
		conversation := []client.EyrieMessage{
			{Role: "user", Content: "test"},
		}
		_, err := SynthesizeSubAgent(context.Background(), nil, "model", conversation)
		if err == nil {
			t.Fatal("expected error for nil client")
		}
	})

	t.Run("provider error propagated", func(t *testing.T) {
		mock := client.NewMockProvider(client.MockModeError)
		mockClient := &mockLLMForSynthesis{provider: mock}

		conversation := []client.EyrieMessage{
			{Role: "user", Content: "test"},
		}

		_, err := SynthesizeSubAgent(context.Background(), mockClient, "model", conversation)
		if err == nil {
			t.Fatal("expected error from provider")
		}
	})

	t.Run("empty response returns error", func(t *testing.T) {
		mock := client.NewMockProvider(client.MockModeFixed)
		mock.Response = ""
		mockClient := &mockLLMForSynthesis{provider: mock}

		conversation := []client.EyrieMessage{
			{Role: "user", Content: "test"},
		}

		_, err := SynthesizeSubAgent(context.Background(), mockClient, "model", conversation)
		if err == nil {
			t.Fatal("expected error for empty response")
		}
	})
}

// mockLLMForSynthesis wraps a MockProvider to satisfy the LLMClient interface.
type mockLLMForSynthesis struct {
	provider *client.MockProvider
}

func (m *mockLLMForSynthesis) Chat(ctx context.Context, msgs []client.EyrieMessage, opts client.ChatOptions) (*client.EyrieResponse, error) {
	return m.provider.Chat(ctx, msgs, opts)
}
