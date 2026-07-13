package engine

import (
	"context"
	"testing"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestNewHawkSession_UsesResolvedSelectionModel(t *testing.T) {
	selection := eyrieengine.Selection{
		Provider:          "openrouter",
		Model:             "openrouter/auto",
		DeploymentRouting: false,
	}

	sess := NewHawkSession(context.Background(), selection, "openrouter", "", "system", nil)
	if got := sess.Provider(); got != "openrouter" {
		t.Fatalf("provider = %q, want openrouter", got)
	}
	if got := sess.Model(); got != "openrouter/auto" {
		t.Fatalf("model = %q, want openrouter/auto", got)
	}
}

func TestBuildChatClientForSettingsComposesCustomGateway(t *testing.T) {
	settings := hawkconfig.Settings{CustomProviders: []hawkconfig.CustomProviderConfig{{
		Name: "private-gateway", BaseURL: "https://private.example.test/v1",
		APIKeyEnv: "PRIVATE_GATEWAY_API_KEY", Model: "private/model-v1",
	}}}
	selection := eyrieengine.Selection{Provider: "private-gateway", Model: "private/model-v1"}
	client, label, deployment, err := BuildChatClientForSettings(context.Background(), settings, selection, "")
	if err != nil {
		t.Fatal(err)
	}
	if client == nil || label != "private-gateway" || !deployment {
		t.Fatalf("client=%T label=%q deployment=%v", client, label, deployment)
	}
}

func TestNewHawkSession_FallsBackToCallerModelWhenSelectionEmpty(t *testing.T) {
	selection := eyrieengine.Selection{
		Provider:          "openrouter",
		DeploymentRouting: false,
	}

	sess := NewHawkSession(context.Background(), selection, "openrouter", "openrouter/fallback", "system", nil)
	if got := sess.Model(); got != "openrouter/fallback" {
		t.Fatalf("model = %q, want openrouter/fallback", got)
	}
}
