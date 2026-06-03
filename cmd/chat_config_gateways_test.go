package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

func TestConfigGatewaysView_RequiresKeyForModelCounts(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	m := chatModel{configTab: configTabGateways}
	view := m.configGatewaysView()
	if !strings.Contains(view, "Catalog") {
		t.Fatalf("expected Catalog column header, got:\n%s", view)
	}
	if !strings.Contains(view, "key required") {
		t.Fatalf("expected key-required model cells without credentials, got:\n%s", view)
	}
	if !strings.Contains(view, "Catalog count appears after a saved key") {
		t.Fatalf("expected key requirement hint without credentials, got:\n%s", view)
	}
}

func TestConfigGatewayRefreshTargetIndex_UsesSelectedRow(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
	_ = store.Set(t.Context(), credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()

	sess := &engine.Session{}
	sess.SetProvider("openrouter")
	m := chatModel{
		configTab:           configTabGateways,
		configSel:           1,
		configGatewayFocus:  3,
		session:             sess,
		configModelProvider: "openrouter",
	}
	rows := m.configGatewayRows()
	idx := m.configGatewayRefreshTargetIndex(rows)
	if idx != 1 {
		t.Fatalf("expected selected row index 1, got %d", idx)
	}
}

func TestConfigGatewayRefreshTargetIndex_UsesFocusOnRefreshRow(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
	_ = store.Set(t.Context(), credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()

	rows := []configGatewayRow{
		{ID: "openai", DisplayName: "OpenAI", HasKey: false},
		{ID: "openrouter", DisplayName: "OpenRouter", HasKey: true, Active: true},
	}
	m := chatModel{configTab: configTabGateways, configSel: len(rows), configGatewayFocus: 1}
	idx := m.configGatewayRefreshTargetIndex(rows)
	if idx != 1 || rows[idx].DisplayName != "OpenRouter" {
		t.Fatalf("expected focus row OpenRouter, got %d", idx)
	}
}

func TestFocusConfigActiveGateway_SelectsActiveRow(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
	_ = store.Set(t.Context(), credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()

	sess := &engine.Session{}
	sess.SetProvider("openrouter")
	m := chatModel{
		configTab:           configTabGateways,
		session:             sess,
		configModelProvider: "openrouter",
	}
	next := m.focusConfigActiveGateway()
	rows := next.configGatewayRows()
	active := next.activeGatewayRowIndex(rows)
	if active < 0 {
		t.Fatal("expected active gateway row")
	}
	if next.configSel != active {
		t.Fatalf("configSel = %d, want active row %d", next.configSel, active)
	}
	if next.configScroll > next.configSel || next.configSel >= next.configScroll+configWindowSize {
		t.Fatalf("active row not visible: sel=%d scroll=%d", next.configSel, next.configScroll)
	}
}

func TestHandleConfigGatewaysSelect_NoKeyRedirectsToKeys(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	m := chatModel{configTab: configTabGateways, configSel: 0}
	next, _ := m.handleConfigGatewaysSelect()
	if next.configTab != configTabKeys {
		t.Fatalf("tab = %d, want Keys", next.configTab)
	}
	if next.configSel != 0 {
		t.Fatalf("sel = %d, want Add API key row", next.configSel)
	}
	if !strings.Contains(next.configNotice, "Add an API key") {
		t.Fatalf("notice = %q", next.configNotice)
	}
}
