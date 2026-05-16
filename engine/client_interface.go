package engine

import (
	"context"

	"github.com/GrayCodeAI/eyrie/client"
)

// ChatClient abstracts the LLM client methods used by Session.
// The production implementation is *client.EyrieClient; tests can inject a mock.
type ChatClient interface {
	Chat(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions) (*client.EyrieResponse, error)
	StreamChatContinue(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions, cfg client.ContinuationConfig) (*client.StreamResult, error)
	SetAPIKey(provider, apiKey string)
}

// SetTestClient replaces the session's LLM client. For testing only.
func (s *Session) SetTestClient(c ChatClient) {
	s.client = c
}

// NewMockClientForTest creates a mock ChatClient that returns canned text responses.
func NewMockClientForTest() ChatClient {
	return &exportedMockClient{}
}

type exportedMockClient struct{}

func (m *exportedMockClient) Chat(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions) (*client.EyrieResponse, error) {
	return &client.EyrieResponse{
		Content:      "mock test response",
		FinishReason: "end_turn",
		Usage:        &client.EyrieUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

func (m *exportedMockClient) StreamChatContinue(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions, cfg client.ContinuationConfig) (*client.StreamResult, error) {
	ch := make(chan client.EyrieStreamEvent, 5)
	ch <- client.EyrieStreamEvent{Type: "content", Content: "mock streamed response"}
	ch <- client.EyrieStreamEvent{Type: "done", StopReason: "end_turn", Usage: &client.EyrieUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}}
	close(ch)
	return &client.StreamResult{Events: ch}, nil
}

func (m *exportedMockClient) SetAPIKey(provider, apiKey string) {}
