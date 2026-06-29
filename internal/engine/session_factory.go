package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/eyrie/runtime"

	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
)

func transportBoolPtr(v bool) *bool { return &v }

// BuildChatClient returns an LLM client and whether deployment routing is active.
func BuildChatClient(ctx context.Context, selection runtime.SelectionState, legacyProvider string) (ChatClient, string, bool, error) {
	provider := strings.TrimSpace(selection.Provider)
	if provider == "" {
		provider = legacyProvider
	}
	transport, err := runtime.ResolveChatTransport(ctx, runtime.ChatTransportOpts{
		Selection: runtime.SelectionOpts{
			ProviderOverride: provider,
			ModelOverride:    selection.Model,
			// Preserve the engine-resolved routing decision. Transport
			// construction itself remains in Eyrie.
			DeploymentRoutingOverride: transportBoolPtr(selection.DeploymentRouting),
		},
	})
	if err == nil && transport.Provider != nil {
		label := strings.TrimSpace(transport.Selection.Provider)
		if label == "" {
			label = provider
		}
		return NewProviderChatClient(types.WrapClientProvider(transport.Provider)), label, transport.Selection.DeploymentRouting, nil
	}
	if err != nil {
		return nil, provider, false, fmt.Errorf("eyrie transport: %w", err)
	}
	return nil, provider, false, fmt.Errorf("eyrie transport: no provider resolved for %q", provider)
}

// NewHawkSession constructs a Session using an engine-resolved selection.
func NewHawkSession(ctx context.Context, selection runtime.SelectionState, provider, model, systemPrompt string, registry *tool.Registry) *Session {
	chat, label, deploy, err := BuildChatClient(ctx, selection, provider)
	if err != nil {
		chat = NewUnavailableChatClient(err)
	}
	resolvedModel := strings.TrimSpace(selection.Model)
	if resolvedModel == "" {
		resolvedModel = model
	}
	return NewSessionWithClient(chat, label, resolvedModel, systemPrompt, registry, deploy)
}

// RebuildSessionTransport rebuilds the LLM client from the engine-resolved selection.
func RebuildSessionTransport(ctx context.Context, s *Session, selection runtime.SelectionState, legacyProvider string) error {
	if s == nil {
		return errors.New("session is nil")
	}
	chat, label, deploy, err := BuildChatClient(ctx, selection, legacyProvider)
	if err != nil {
		return err
	}
	s.ReattachTransport(chat, label, deploy)
	return nil
}
