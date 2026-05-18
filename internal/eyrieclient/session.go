package eyrieclient

import (
	"context"

	"github.com/GrayCodeAI/eyrie/client"
	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/setup"

	hawkcfg "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// BuildChatClient returns an LLM client and whether deployment routing is active.
func BuildChatClient(ctx context.Context, settings hawkcfg.Settings, legacyProvider string) (engine.ChatClient, string, bool) {
	cfg := eyriecfg.LoadProviderConfig("")
	if hawkcfg.DeploymentRoutingEnabled(settings) && setup.UseDeploymentRouting(cfg) {
		p, err := setup.DeploymentProvider(ctx, cfg)
		if err == nil {
			return engine.NewProviderChatClient(p), legacyProvider, true
		}
	}
	c := client.Client(&client.EyrieConfig{Provider: legacyProvider})
	return c, legacyProvider, false
}

// NewHawkSession constructs a Session using deployment routing when configured.
func NewHawkSession(ctx context.Context, settings hawkcfg.Settings, provider, model, systemPrompt string, registry *tool.Registry) *engine.Session {
	chat, label, deploy := BuildChatClient(ctx, settings, provider)
	return engine.NewSessionWithClient(chat, label, model, systemPrompt, registry, deploy)
}
