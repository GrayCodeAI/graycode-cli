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
	if !strings.Contains(view, "enter open key · delete remove") {
		t.Fatalf("expected open/remove hint, got:\n%s", view)
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

func TestConfigKeysSelect_OpensKeyView(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
	_ = store.Set(t.Context(), credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()

	m := chatModel{configTab: configTabKeys, configSel: 0}
	next, _ := m.handleConfigKeysSelect()
	if next.configEntry != configEntryKeyView {
		t.Fatalf("expected key view, got entry=%q", next.configEntry)
	}
	if next.configProvider != "openrouter" {
		t.Fatalf("provider = %q", next.configProvider)
	}
}

func TestConfigKeysDelete_PendingRemove(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
	_ = store.Set(t.Context(), credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()

	m := chatModel{configTab: configTabKeys, configSel: 0}
	next := m.handleConfigKeysDelete()
	if next.configKeysPendingRemove != "openrouter" {
		t.Fatalf("pending remove = %q", next.configKeysPendingRemove)
	}
	if next.configKeysRemoveStep != 1 {
		t.Fatalf("remove step = %d, want 1", next.configKeysRemoveStep)
	}
}

func TestConfigKeysDelete_DoubleConfirm(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
	_ = store.Set(t.Context(), credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()

	m := chatModel{configTab: configTabKeys, configSel: 0}
	m = m.handleConfigKeysDelete()
	next, cmd := m.handleConfigKeysSelect()
	if cmd != nil {
		t.Fatal("first enter should not remove yet")
	}
	if next.configKeysRemoveStep != 2 {
		t.Fatalf("remove step = %d, want 2 after first enter", next.configKeysRemoveStep)
	}
	if next.configKeysPendingRemove != "openrouter" {
		t.Fatalf("pending remove = %q", next.configKeysPendingRemove)
	}
	next, cmd = next.handleConfigKeysSelect()
	if cmd == nil {
		t.Fatal("second enter should start remove")
	}
	if next.configSaving != true {
		t.Fatal("expected saving state after final confirm")
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
