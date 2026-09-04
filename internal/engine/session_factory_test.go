package engine

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/provider/gateway"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
)

func TestNewGraycodeSession_UsesResolvedSelectionModel(t *testing.T) {
	selection := gateway.Selection{
		Provider:          "openrouter",
		Model:             "openrouter/auto",
		DeploymentRouting: false,
	}

	sess := NewGraycodeSession(context.Background(), selection, "openrouter", "", "system", nil)
	if got := sess.Provider(); got != "openrouter" {
		t.Fatalf("provider = %q, want openrouter", got)
	}
	if got := sess.Model(); got != "openrouter/auto" {
		t.Fatalf("model = %q, want openrouter/auto", got)
	}
}

func TestBuildChatClientForSettingsComposesCustomGateway(t *testing.T) {
	settings := graycodeconfig.Settings{CustomProviders: []graycodeconfig.CustomProviderConfig{{
		Name: "private-gateway", BaseURL: "https://private.example.test/v1",
		APIKeyEnv: "PRIVATE_GATEWAY_API_KEY", Model: "private/model-v1",
	}}}
	selection := gateway.Selection{Provider: "private-gateway", Model: "private/model-v1"}
	client, label, deployment, err := BuildChatClientForSettings(context.Background(), settings, selection, "")
	if err != nil {
		t.Fatal(err)
	}
	if client == nil || label != "private-gateway" || !deployment {
		t.Fatalf("client=%T label=%q deployment=%v", client, label, deployment)
	}
}

func TestNewGraycodeSession_FallsBackToCallerModelWhenSelectionEmpty(t *testing.T) {
	selection := gateway.Selection{
		Provider:          "openrouter",
		DeploymentRouting: false,
	}

	sess := NewGraycodeSession(context.Background(), selection, "openrouter", "openrouter/fallback", "system", nil)
	if got := sess.Model(); got != "openrouter/fallback" {
		t.Fatalf("model = %q, want openrouter/fallback", got)
	}
}
