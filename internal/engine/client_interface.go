package engine

import (
	"context"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// ChatClient abstracts the LLM client methods used by Session.
// The production implementation is *types.EyrieClient; tests can inject a mock.
type ChatClient interface {
	Chat(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error)
	StreamChatContinue(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions, cfg types.ContinuationConfig) (*types.StreamResult, error)
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

func (m *exportedMockClient) Chat(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	return &types.EyrieResponse{
		Content:      "mock test response",
		FinishReason: "end_turn",
		Usage:        &types.EyrieUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

func (m *exportedMockClient) StreamChatContinue(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions, cfg types.ContinuationConfig) (*types.StreamResult, error) {
	ch := make(chan types.EyrieStreamEvent, 5)
	ch <- types.EyrieStreamEvent{Type: "content", Content: "mock streamed response"}
	ch <- types.EyrieStreamEvent{Type: "done", StopReason: "end_turn", Usage: &types.EyrieUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}}
	close(ch)
	return &types.StreamResult{Events: ch}, nil
}

func (m *exportedMockClient) SetAPIKey(provider, apiKey string) {}
