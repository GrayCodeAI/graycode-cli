package engine

import (
	"context"
	"sync"

	"github.com/GrayCodeAI/eyrie/client"
)

// mockClient implements ChatClient for testing without real LLM calls.
type mockClient struct {
	mu        sync.Mutex
	responses []*client.EyrieResponse
	idx       int
	calls     []mockCall
}

type mockCall struct {
	method   string
	messages []client.EyrieMessage
}

func newMockClient(responses ...*client.EyrieResponse) *mockClient {
	return &mockClient{responses: responses}
}

func mockTextResponse(text string) *client.EyrieResponse {
	return &client.EyrieResponse{
		Content:      text,
		FinishReason: "end_turn",
		Usage:        &client.EyrieUsage{PromptTokens: 50, CompletionTokens: 20, TotalTokens: 70},
	}
}

func mockToolResponse(toolName string, args map[string]interface{}) *client.EyrieResponse {
	return &client.EyrieResponse{
		FinishReason: "tool_use",
		ToolCalls: []client.ToolCall{{
			ID:        "call_mock_1",
			Name:      toolName,
			Arguments: args,
		}},
		Usage: &client.EyrieUsage{PromptTokens: 50, CompletionTokens: 30, TotalTokens: 80},
	}
}

func (m *mockClient) Chat(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions) (*client.EyrieResponse, error) {
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

func (m *mockClient) StreamChatContinue(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions, cfg client.ContinuationConfig) (*client.StreamResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, mockCall{method: "StreamChatContinue", messages: messages})

	ch := make(chan client.EyrieStreamEvent, 10)

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

	ch <- client.EyrieStreamEvent{Type: "content", Content: content}
	ch <- client.EyrieStreamEvent{
		Type:       "done",
		StopReason: finishReason,
		Usage:      &client.EyrieUsage{PromptTokens: 50, CompletionTokens: 20, TotalTokens: 70},
	}
	close(ch)

	return &client.StreamResult{Events: ch}, nil
}

func (m *mockClient) SetAPIKey(provider, apiKey string) {}

func (m *mockClient) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockClient) lastCall() mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return mockCall{}
	}
	return m.calls[len(m.calls)-1]
}
