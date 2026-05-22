package engine

import (
	"context"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// providerChatClient adapts any eyrie types.Provider to ChatClient (continuations + streaming).
type providerChatClient struct {
	p types.Provider
}

// NewProviderChatClient wraps a catalog-backed provider (e.g. DeploymentRouter) for Session use.
func NewProviderChatClient(p types.Provider) ChatClient {
	return &providerChatClient{p: p}
}

func (w *providerChatClient) Chat(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	return w.p.Chat(ctx, messages, opts)
}

func (w *providerChatClient) StreamChatContinue(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions, cfg types.ContinuationConfig) (*types.StreamResult, error) {
	return types.StreamChatWithContinuation(ctx, w.p, messages, opts, cfg)
}

func (w *providerChatClient) SetAPIKey(_, _ string) {
	// Credentials live on concrete adapters inside DeploymentRouter; Session env keys are unused here.
}
