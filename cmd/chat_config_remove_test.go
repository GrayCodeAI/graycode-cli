package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestConfigHubOptions_OmitsRemoveKeyEntry(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	_ = store.Set(t.Context(), credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")

	m := chatModel{}
	for _, o := range m.configHubOptions() {
		if o.action == "remove-key" {
			t.Fatal("remove-key belongs on /config key remove only, not the hub menu")
		}
	}
}

func TestConfigHubOptions_OmitsRemoveWithoutCredentials(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	m := chatModel{}
	for _, o := range m.configHubOptions() {
		if o.action == "remove-key" {
			t.Fatal("remove-key should not appear when no credentials are stored")
		}
	}
}

func TestConfiguredCredentialProviders_UsedByRemovePicker(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	_ = store.Set(t.Context(), credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")

	m := chatModel{}
	labels := m.configRemoveKeyLabels()
	if len(labels) == 0 {
		t.Fatal("expected at least one removable provider")
	}
	providers := hawkconfig.ConfiguredCredentialProviders()
	if len(labels) != len(providers) {
		t.Fatalf("labels = %v providers = %v", labels, providers)
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
