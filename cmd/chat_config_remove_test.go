package cmd

import (
	"strings"
	"testing"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/provider/gateway"
)

func TestConfigGatewayRows_ShowsSavedKey(t *testing.T) {
	graycodeconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		graycodeconfig.InvalidateConfigUICache()
	})
	if err := store.Set(t.Context(), gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890"); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	graycodeconfig.InvalidateConfigUICache()

	rows := chatModel{}.configGatewayRows()
	var found bool
	for _, row := range rows {
		if row.ID == "openrouter" && row.HasKey {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected openrouter row with HasKey, got %+v", rows)
	}
}

func TestConfiguredCredentialProviders_UsedByGatewaysTab(t *testing.T) {
	graycodeconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		graycodeconfig.InvalidateConfigUICache()
	})
	if err := store.Set(t.Context(), gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890"); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	graycodeconfig.InvalidateConfigUICache()

	got := graycodeconfig.ConfiguredCredentialProviders()
	if len(got) != 1 || got[0] != "openrouter" {
		t.Fatalf("configured = %v", got)
	}
}

func TestRemoveCredentialAsync(t *testing.T) {
	graycodeconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		graycodeconfig.InvalidateConfigUICache()
	})
	if err := store.Set(t.Context(), gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890"); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	graycodeconfig.InvalidateConfigUICache()

	cmd := removeCredentialAsync("openrouter")
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg := cmd()
	rem, ok := msg.(configRemoveCredentialMsg)
	if !ok {
		t.Fatalf("msg type %T", msg)
	}
	if rem.err != nil {
		t.Fatal(rem.err)
	}
	if len(rem.removed) != 1 || rem.removed[0] != "OPENROUTER_API_KEY" {
		t.Fatalf("removed = %v", rem.removed)
	}
	if strings.TrimSpace(rem.provider) != "openrouter" {
		t.Fatalf("provider = %q", rem.provider)
	}
}

func TestConfigTabLabels(t *testing.T) {
	if len(configTabLabels) != 2 || configTabLabels[0] != "Gateways" {
		t.Fatalf("tabs = %v", configTabLabels)
	}
}
