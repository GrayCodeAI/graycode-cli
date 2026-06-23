package engine

import (
	"context"
	"fmt"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/setup"

	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// BuildChatClient returns an LLM client and whether deployment routing is active.
func BuildChatClient(ctx context.Context, useDeploymentRouting bool, legacyProvider string) (ChatClient, string, bool) {
	cfg := eyriecfg.LoadProviderConfig("")
	if useDeploymentRouting {
		p, err := setup.DeploymentProvider(ctx, cfg)
		if err == nil {
			return NewProviderChatClient(types.WrapClientProvider(p)), legacyProvider, true
		}
	}
	c := types.NewClient(&types.ClientConfig{Provider: legacyProvider})
	return c, legacyProvider, false
}

// NewHawkSession constructs a Session using deployment routing when configured.
func NewHawkSession(ctx context.Context, useDeploymentRouting bool, provider, model, systemPrompt string, registry *tool.Registry) *Session {
	chat, label, deploy := BuildChatClient(ctx, useDeploymentRouting, provider)
	return NewSessionWithClient(chat, label, model, systemPrompt, registry, deploy)
}

// RebuildSessionTransport rebuilds the LLM client from current settings and provider.json.
func RebuildSessionTransport(ctx context.Context, s *Session, useDeploymentRouting bool, legacyProvider string) error {
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	chat, label, deploy := BuildChatClient(ctx, useDeploymentRouting, legacyProvider)
	s.ReattachTransport(chat, label, deploy)
	return nil
}
