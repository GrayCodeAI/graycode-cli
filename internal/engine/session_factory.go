package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/provider/gateway"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
)

// BuildChatClient returns an LLM client and whether deployment routing is active.
// It is the single composition root: it builds one gateway.Gateway and adapts it
// to the graycode ChatClient port via the gateway's anti-corruption adapter.
func BuildChatClient(ctx context.Context, selection gateway.Selection, legacyProvider string) (ChatClient, string, bool, error) {
	modelRuntime, err := gateway.New(ctx, nil)
	if err != nil {
		return nil, requestedProvider(selection, legacyProvider), false, fmt.Errorf("graycode-router transport: %w", err)
	}
	return buildChatClientWithRuntime(ctx, modelRuntime, selection, legacyProvider)
}

func buildChatClientWithRuntime(ctx context.Context, modelRuntime *gateway.Gateway, selection gateway.Selection, legacyProvider string) (ChatClient, string, bool, error) {
	_ = ctx // request contexts are applied per generation by the facade adapter
	provider := strings.TrimSpace(selection.Provider)
	if provider == "" {
		provider = legacyProvider
	}
	resolvedSelection := selection
	if strings.TrimSpace(resolvedSelection.Provider) == "" {
		resolvedSelection.Provider = provider
	}
	if modelRuntime == nil {
		return nil, provider, false, errors.New("graycode-router transport: runtime is nil")
	}
	label := strings.TrimSpace(resolvedSelection.Provider)
	if label == "" {
		label = provider
	}
	return modelRuntime.ChatClient(), label, true, nil
}

// BuildChatClientForSettings composes the model runtime from one effective
// settings snapshot. It is the command-facing path for --settings isolation.
func BuildChatClientForSettings(ctx context.Context, settings graycodeconfig.Settings, selection gateway.Selection, legacyProvider string) (ChatClient, string, bool, error) {
	modelRuntime, err := gateway.New(ctx, gatewayCustomGateways(settings.CustomProviders))
	if err != nil {
		return nil, requestedProvider(selection, legacyProvider), false, fmt.Errorf("graycode-router transport: %w", err)
	}
	return buildChatClientWithRuntime(ctx, modelRuntime, selection, legacyProvider)
}

// convertCustomProviders maps config.CustomProviderConfig → gateway.CustomProviderConfig
// at the composition root (the gateway package cannot import config).
// Delegates the final step to gateway.BuildCustomGateways so a new
// CustomProviderConfig field only needs wiring once.
func convertCustomProviders(in []graycodeconfig.CustomProviderConfig) []gateway.CustomProviderConfig {
	out := make([]gateway.CustomProviderConfig, 0, len(in))
	for _, p := range in {
		if p.Name == "" && p.BaseURL == "" {
			continue
		}
		out = append(out, gateway.CustomProviderConfig{
			Name: p.Name, BaseURL: p.BaseURL, APIKeyEnv: p.APIKeyEnv, Model: p.Model,
		})
	}
	return out
}

// gatewayCustomGateways converts config providers to gateway specs, reusing
// the shared conversion loop.
func gatewayCustomGateways(in []graycodeconfig.CustomProviderConfig) []gateway.CustomProviderConfig {
	return convertCustomProviders(in)
}

func requestedProvider(selection gateway.Selection, legacyProvider string) string {
	if provider := strings.TrimSpace(selection.Provider); provider != "" {
		return provider
	}
	return strings.TrimSpace(legacyProvider)
}

// NewGraycodeSession constructs a Session using an engine-resolved selection.
func NewGraycodeSession(ctx context.Context, selection gateway.Selection, provider, model, systemPrompt string, registry *tool.Registry) *Session {
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

// NewGraycodeSessionForSettings constructs a session with an invocation-scoped
// GraycodeRouter engine, including custom gateways from an explicit settings file.
func NewGraycodeSessionForSettings(ctx context.Context, settings graycodeconfig.Settings, selection gateway.Selection, provider, model, systemPrompt string, registry *tool.Registry) *Session {
	chat, label, deploy, err := BuildChatClientForSettings(ctx, settings, selection, provider)
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
func RebuildSessionTransport(ctx context.Context, s *Session, selection gateway.Selection, legacyProvider string) error {
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

func RebuildSessionTransportForSettings(ctx context.Context, settings graycodeconfig.Settings, s *Session, selection gateway.Selection, legacyProvider string) error {
	if s == nil {
		return errors.New("session is nil")
	}
	chat, label, deploy, err := BuildChatClientForSettings(ctx, settings, selection, legacyProvider)
	if err != nil {
		return err
	}
	s.ReattachTransport(chat, label, deploy)
	return nil
}
