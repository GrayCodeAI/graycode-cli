package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/provider/gateway"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// BuildChatProvider adapts Graycode's engine-backed session client to the smaller
// provider contract used by host integrations such as Kestrel. Model resolution,
// credentials, routing, and transport remain owned by GraycodeRouter's engine facade.
func BuildChatProvider(ctx context.Context, selection gateway.Selection, legacyProvider string) (types.ChatProvider, string, error) {
	client, provider, _, err := BuildChatClient(ctx, selection, legacyProvider)
	if err != nil {
		return nil, provider, err
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil, "", fmt.Errorf("graycode-router transport: provider unavailable")
	}
	return &engineChatProvider{
		client:   client,
		provider: provider,
		model:    strings.TrimSpace(selection.Model),
	}, provider, nil
}

// engineChatProvider keeps compatibility-only provider behavior at Graycode's
// integration edge while all generation goes through the engine ChatClient.
type engineChatProvider struct {
	client   ChatClient
	provider string
	model    string
}

var _ types.ChatProvider = (*engineChatProvider)(nil)

func (p *engineChatProvider) Chat(ctx context.Context, messages []types.GraycodeRouterMessage, opts types.ChatOptions) (*types.GraycodeRouterResponse, error) {
	if strings.TrimSpace(opts.Provider) == "" {
		opts.Provider = p.provider
	}
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = p.model
	}
	return p.client.Chat(ctx, messages, opts)
}

func (p *engineChatProvider) StreamChat(ctx context.Context, messages []types.GraycodeRouterMessage, opts types.ChatOptions) (*types.StreamResult, error) {
	if strings.TrimSpace(opts.Provider) == "" {
		opts.Provider = p.provider
	}
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = p.model
	}
	return p.client.StreamChatContinue(ctx, messages, opts, types.ContinuationConfig{})
}

func (p *engineChatProvider) Ping(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func (p *engineChatProvider) Name() string { return p.provider }
