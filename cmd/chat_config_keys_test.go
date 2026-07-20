package cmd

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestConfigGatewaysView_KeyHintsWithCredentials(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
	_ = store.Set(t.Context(), gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()

	m := chatModel{configTab: configTabGateways}
	view := m.configGatewaysView()
	if !strings.Contains(view, "k view key") {
		t.Fatalf("expected key management hint, got:\n%s", view)
	}
}

func TestConfigGatewaysKeyView_OpenWithK(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
	_ = store.Set(t.Context(), gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()

	rows := chatModel{}.configGatewayRows()
	sel := 0
	for i, row := range rows {
		if row.ID == "openrouter" {
			sel = i
			break
		}
	}
	m := chatModel{configTab: configTabGateways, configSel: sel}
	next, _ := m.handleConfigKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if next.configEntry != configEntryKeyView {
		t.Fatalf("expected key view, got entry=%q", next.configEntry)
	}
	if next.configProvider != "openrouter" {
		t.Fatalf("provider = %q", next.configProvider)
	}
}

func TestConfigGatewaysDelete_PendingRemove(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
	_ = store.Set(t.Context(), gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()

	rows := chatModel{}.configGatewayRows()
	sel := 0
	for i, row := range rows {
		if row.ID == "openrouter" {
			sel = i
			break
		}
	}
	m := chatModel{configTab: configTabGateways, configSel: sel}
	next := m.handleConfigGatewaysDelete()
	if next.configKeysPendingRemove != "openrouter" {
		t.Fatalf("pending remove = %q", next.configKeysPendingRemove)
	}
	if next.configKeysRemoveStep != 1 {
		t.Fatalf("remove step = %d, want 1", next.configKeysRemoveStep)
	}
}

func TestConfigGatewaysDelete_DoubleConfirm(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
	_ = store.Set(t.Context(), gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()

	rows := chatModel{}.configGatewayRows()
	sel := 0
	for i, row := range rows {
		if row.ID == "openrouter" {
			sel = i
			break
		}
	}
	m := chatModel{configTab: configTabGateways, configSel: sel}
	m = m.handleConfigGatewaysDelete()
	next, cmd := m.handleConfigGatewaysSelect()
	if cmd != nil {
		t.Fatal("expected no cmd on first confirm enter")
	}
	if next.configKeysRemoveStep != 2 {
		t.Fatalf("remove step = %d, want 2", next.configKeysRemoveStep)
	}
	next, cmd = next.handleConfigGatewaysSelect()
	if cmd == nil {
		t.Fatal("expected remove cmd on final confirm")
	}
	if !next.configSaving {
		t.Fatal("expected saving state after final confirm")
	}
}

func TestOpenConfigRemoveKeyPanel_OpensGatewaysTab(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	m := chatModel{}
	next, _ := m.openConfigRemoveKeyPanel()
	if !next.configOpen || next.configTab != configTabGateways {
		t.Fatalf("expected gateways tab open, got open=%v tab=%d", next.configOpen, next.configTab)
	}
	if next.configNotice != "No stored API keys" {
		t.Fatalf("notice = %q", next.configNotice)
	}
}
