package engine

import (
	"context"

	"github.com/GrayCodeAI/eyrie/client"
)

// providerChatClient adapts any eyrie client.Provider to ChatClient (continuations + streaming).
type providerChatClient struct {
	p client.Provider
}

// NewProviderChatClient wraps a catalog-backed provider (e.g. DeploymentRouter) for Session use.
func NewProviderChatClient(p client.Provider) ChatClient {
	return &providerChatClient{p: p}
}

func (w *providerChatClient) Chat(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions) (*client.EyrieResponse, error) {
	return w.p.Chat(ctx, messages, opts)
}

func (w *providerChatClient) StreamChatContinue(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions, cfg client.ContinuationConfig) (*client.StreamResult, error) {
	return client.StreamChatWithContinuation(ctx, w.p, messages, opts, cfg)
}

func (w *providerChatClient) SetAPIKey(_, _ string) {
	// Credentials live on concrete adapters inside DeploymentRouter; Session env keys are unused here.
}
