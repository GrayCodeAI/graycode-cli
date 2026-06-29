package engine

import (
	"context"
	"errors"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// providerChatClient adapts any eyrie types.Provider to ChatClient (continuations + streaming).
type providerChatClient struct {
	p types.ChatProvider
}

// NewProviderChatClient wraps a catalog-backed provider (e.g. DeploymentRouter) for Session use.
func NewProviderChatClient(p types.ChatProvider) ChatClient {
	return &providerChatClient{p: p}
}

// NewUnavailableChatClient preserves Session construction while surfacing
// Eyrie transport setup failures at the first chat call.
func NewUnavailableChatClient(err error) ChatClient {
	if err == nil {
		err = errors.New("hawk: chat transport unavailable")
	}
	return &unavailableChatClient{err: err}
}

func (w *providerChatClient) Chat(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	if w == nil || w.p == nil {
		return nil, errors.New("hawk: chat provider unavailable")
	}
	return w.p.Chat(ctx, messages, opts)
}

func (w *providerChatClient) StreamChatContinue(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions, cfg types.ContinuationConfig) (*types.StreamResult, error) {
	if w == nil || w.p == nil {
		return nil, errors.New("hawk: chat provider unavailable")
	}
	return types.StreamChatWithContinuation(ctx, w.p, messages, opts, cfg)
}

func (w *providerChatClient) SetAPIKey(_, _ string) {
	// Credentials live on concrete adapters inside DeploymentRouter; Session env keys are unused here.
}

type unavailableChatClient struct {
	err error
}

func (c *unavailableChatClient) Chat(context.Context, []types.EyrieMessage, types.ChatOptions) (*types.EyrieResponse, error) {
	return nil, c.err
}

func (c *unavailableChatClient) StreamChatContinue(context.Context, []types.EyrieMessage, types.ChatOptions, types.ContinuationConfig) (*types.StreamResult, error) {
	return nil, c.err
}

func (c *unavailableChatClient) SetAPIKey(_, _ string) {}
