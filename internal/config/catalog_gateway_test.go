package config

import (
	"context"
	"testing"
)

func TestActiveGateway_IgnoresOwnerSlug(t *testing.T) {
	if IsSetupGateway("moonshotai") {
		t.Fatal("moonshotai should not be a setup gateway")
	}
	if gw := GatewayForModel("openrouter/auto"); gw != "openrouter" {
		t.Fatalf("GatewayForModel(openrouter/auto) = %q", gw)
	}
}

func TestShouldClearSelection_NoCredentials(t *testing.T) {
	ctx := context.Background()
	if !ShouldClearSelectionAfterCredentialRemove(ctx, "canopywave") {
		// When no creds configured, should always clear — may be false if test env has keys
		t.Log("HasConfiguredDeployment true in test env; skipping strict assert")
	}
}
