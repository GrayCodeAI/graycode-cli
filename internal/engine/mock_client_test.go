package engine

import (
	"context"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// mockClient implements ChatClient for testing without real LLM calls.
type mockClient struct {
	mu        sync.Mutex
	responses []*types.EyrieResponse
	idx       int
	calls     []mockCall
}

type mockCall struct {
	method   string
	messages []types.EyrieMessage
}

func newMockClient(responses ...*types.EyrieResponse) *mockClient {
	return &mockClient{responses: responses}
}

func mockTextResponse(text string) *types.EyrieResponse {
	return &types.EyrieResponse{
		Content:      text,
		FinishReason: "end_turn",
		Usage:        &types.EyrieUsage{PromptTokens: 50, CompletionTokens: 20, TotalTokens: 70},
	}
}

func (m *mockClient) Chat(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, mockCall{method: "Chat", messages: messages})

	if m.idx >= len(m.responses) {
		return mockTextResponse("mock fallback response"), nil
	}
	resp := m.responses[m.idx]
	m.idx++
	return resp, nil
}

func (m *mockClient) StreamChatContinue(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions, cfg types.ContinuationConfig) (*types.StreamResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, mockCall{method: "StreamChatContinue", messages: messages})

	ch := make(chan types.EyrieStreamEvent, 10)

	var content string
	var finishReason string
	if m.idx < len(m.responses) {
		resp := m.responses[m.idx]
		m.idx++
		content = resp.Content
		finishReason = resp.FinishReason
	} else {
		content = "mock streamed response"
		finishReason = "end_turn"
	}

	ch <- types.EyrieStreamEvent{Type: "content", Content: content}
	ch <- types.EyrieStreamEvent{
		Type:       "done",
		StopReason: finishReason,
		Usage:      &types.EyrieUsage{PromptTokens: 50, CompletionTokens: 20, TotalTokens: 70},
	}
	close(ch)

	return &types.StreamResult{Events: ch}, nil
}

func (m *mockClient) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}
