package config

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestNewEyrieEngineForSettingsIsInvocationScoped(t *testing.T) {
	previousStore := credentials.DefaultStore()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(previousStore) })

	settings := Settings{CustomProviders: []CustomProviderConfig{{
		Name: "private-gateway", BaseURL: "https://private.example.test/v1",
		APIKeyEnv: "PRIVATE_GATEWAY_API_KEY", Model: "private/model-v1",
	}}}
	engine, err := NewEyrieEngineForSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var credentialEnv string
	for _, gateway := range engine.GatewayDefinitions() {
		if gateway.ID == "private_gateway" {
			credentialEnv = gateway.CredentialEnv
		}
	}
	if credentialEnv != "PRIVATE_GATEWAY_API_KEY" {
		got := credentialEnv
		t.Fatalf("custom credential env = %q", got)
	}
	models, err := ListEngineModelsWithSettings(ctx, settings, "private-gateway", false)
	if err != nil || len(models) != 1 || models[0].ID != "private/model-v1" || models[0].GatewayID != "private_gateway" {
		t.Fatalf("custom model surface = %+v, err=%v", models, err)
	}
	if setErr := store.Set(ctx, credentials.AccountForEnv("PRIVATE_GATEWAY_API_KEY"), "private-live-secret-1234567890"); setErr != nil {
		t.Fatal(setErr)
	}
	status, err := engine.CredentialStatus(ctx, "private-gateway")
	if err != nil || !status.Configured {
		t.Fatalf("custom credential surface = %+v, err=%v", status, err)
	}

	if _, collisionErr := NewEyrieEngineForSettings(Settings{CustomProviders: []CustomProviderConfig{{
		Name: "openai", BaseURL: "https://collision.example.test/v1",
	}}}); collisionErr == nil {
		t.Fatal("built-in custom gateway collision was accepted")
	}
	models, err = engine.ListModels(ctx, "private-gateway", false)
	if err != nil || len(models) != 1 {
		t.Fatalf("independent invalid construction changed existing engine: %+v, err=%v", models, err)
	}
	other, err := NewEyrieEngineForSettings(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	for _, gateway := range other.GatewayDefinitions() {
		if gateway.ID == "private_gateway" {
			t.Fatal("custom gateway leaked into another settings-scoped engine")
		}
	}
}
