package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestConfigKeysRows_NoRemoveAction(t *testing.T) {
	m := chatModel{}
	for _, row := range m.configKeysRows(nil) {
		if row.kind == "remove" {
			t.Fatalf("remove action should be merged into credential rows, got %+v", row)
		}
	}
}

func TestConfigKeysView_HintWhenCredentialsPresent(t *testing.T) {
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
	view := m.configKeysView()
	if !strings.Contains(view, "enter saved row to remove key") {
		t.Fatalf("expected remove hint, got:\n%s", view)
	}
	if strings.Contains(view, "Remove API key") {
		t.Fatal("separate Remove API key row should not exist")
	}
}

func TestConfigKeysView_NoRemoveHintWithoutCredentials(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	m := chatModel{}
	view := m.configKeysView()
	if !strings.Contains(view, "Add API key") {
		t.Fatalf("expected Add API key row, got:\n%s", view)
	}
	if !strings.Contains(view, "No API keys yet") {
		t.Fatalf("expected empty-state hint, got:\n%s", view)
	}
}

func TestOpenConfigRemoveKeyPanel_OpensKeysTab(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	m := chatModel{}
	next, _ := m.openConfigRemoveKeyPanel()
	if !next.configOpen || next.configTab != configTabKeys {
		t.Fatalf("expected keys tab open, got open=%v tab=%d", next.configOpen, next.configTab)
	}
	if next.configNotice != "No stored API keys" {
		t.Fatalf("notice = %q", next.configNotice)
	}
}
