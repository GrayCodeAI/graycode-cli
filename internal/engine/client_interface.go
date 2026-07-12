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
}

// SetTestClient replaces the session's LLM client. For testing only.
// Also reattaches the ChatService so the agent loop's `s.ChatLLM().Stream`
// call site sees the mock (Phase 7 migration).
func (s *Session) SetTestClient(c ChatClient) {
	s.client = c
	if s.llm != nil {
		s.llm.Reattach(c, s.provider)
	}
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
