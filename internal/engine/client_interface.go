package engine

import (
	"context"

	"github.com/GrayCodeAI/hawk-core-contracts/llm"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// ChatClient abstracts the LLM client methods used by Session.
// The production implementation is the Eyrie engine adapter; tests can inject a mock.
type ChatClient interface {
	Chat(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error)
	StreamChatContinue(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions, cfg types.ContinuationConfig) (*types.StreamResult, error)
}

// resilienceManagingChatClient is an optional capability implemented by
// facade clients that already own provider retries, rate limiting,
// continuation, and protocol normalization. Keeping it separate from
// ChatClient preserves compatibility with injected and legacy clients.
type resilienceManagingChatClient interface {
	ManagesResilience() bool
}

func clientManagesResilience(client ChatClient) bool {
	manager, ok := client.(resilienceManagingChatClient)
	return ok && manager.ManagesResilience()
}

// nativeCompactionCapable is an optional capability implemented by facade
// clients bound to an engine that supports provider-native compaction — so
// Session never needs to unwrap the raw engine.
type nativeCompactionCapable interface {
	NativeCompaction(ctx context.Context, provider, model string) bool
	CompactNative(ctx context.Context, req llm.NativeCompactionRequest) (string, error)
}

func clientNativeCompaction(client ChatClient, ctx context.Context, provider, model string) bool {
	if c, ok := client.(nativeCompactionCapable); ok {
		return c.NativeCompaction(ctx, provider, model)
	}
	return false
}

// SetTestClient replaces the session's LLM client. For testing only.
// Also reattaches the ChatService so the agent loop's `s.ChatLLM().Stream`
// call site sees the mock (Phase 7 migration).
func (s *Session) SetTestClient(c ChatClient) {
	if s.llm != nil {
		s.llm.Reattach(c, s.llm.Provider())
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
