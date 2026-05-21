package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestConfigKeysRows_IncludesActions(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	m := chatModel{}
	rows := m.configKeysRows(hawkconfig.ConfiguredCredentialProviders())
	if len(rows) < 2 {
		t.Fatalf("expected add + ollama actions, got %d rows", len(rows))
	}
	if rows[len(rows)-2].kind != configKeysActionAdd {
		t.Fatalf("expected Add API key row, got %q", rows[len(rows)-2].kind)
	}
}

func TestConfiguredCredentialProviders_UsedByKeysTab(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	_ = store.Set(t.Context(), credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()

	m := chatModel{}
	rows := m.configKeysRows(hawkconfig.ConfiguredCredentialProviders())
	found := false
	for _, r := range rows {
		if r.kind == configKeysRowCredential && r.provider == "openrouter" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected openrouter credential row, got %+v", rows)
	}
	providers := hawkconfig.ConfiguredCredentialProviders()
	if len(providers) == 0 {
		t.Fatal("expected configured providers")
	}
}

func TestRemoveCredentialAsyncMessage(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	_ = store.Set(t.Context(), credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")

	msg := removeCredentialAsync("openrouter")()
	rem, ok := msg.(configRemoveCredentialMsg)
	if !ok {
		t.Fatalf("unexpected msg type %T", msg)
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
	if len(configTabLabels) != 3 || configTabLabels[1] != "Gateways" {
		t.Fatalf("tabs = %v", configTabLabels)
	}
}
