package cmd

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
			tea "charm.land/bubbletea/v2"
	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

func chatModelForConfigPasteTest() chatModel {
	ti := textinput.New()
	ti.Width = 40
	ti, _ = ti.Update(tea.WindowSizeMsg{Width: 40, Height: 1})
	return chatModel{configInput: ti, input: textarea.New()}
}

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
	if !strings.Contains(view, "Select a gateway") {
		t.Fatalf("expected gateway-first setup hint without credentials, got:\n%s", view)
	}
}

func TestConfigGatewaysView_ShowsSaveOrProbeNotice(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	m := chatModel{
		configTab:    configTabGateways,
		configSel:    0,
		configNotice: "Key saved in " + credentialsStoreLabel() + " — provider rejected this key: credential probe failed: invalid API key (HTTP 401)",
		width:        120,
		height:       40,
	}
	view := m.configTabShellView(m.configGatewaysView())
	if !strings.Contains(view, "Key saved") || !strings.Contains(view, "rejected") {
		t.Fatalf("expected visible save/probe notice, got:\n%s", view)
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

func TestHandleConfigGatewaysSelect_TokenPlanNoKeyShowsRegion(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	m := chatModel{configTab: configTabGateways}
	rows := m.configGatewayRows()
	idx := -1
	for i, row := range rows {
		if row.ID == hawkconfig.ProviderXiaomiTokenPlan {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("expected xiaomi_mimo_token_plan gateway row")
	}
	m.configSel = idx
	next, _ := m.handleConfigGatewaysSelect()
	if next.configEntry != configEntryXiaomiRegion {
		t.Fatalf("entry = %q, want xiaomi region picker", next.configEntry)
	}
	if next.configProvider != hawkconfig.ProviderXiaomiTokenPlan {
		t.Fatalf("provider = %q", next.configProvider)
	}
}

func TestHandleConfigGatewaysSelect_NoKeyStartsPaste(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	m := chatModelForConfigPasteTest()
	m.configTab = configTabGateways
	gwRows := m.configGatewayRows()
	if len(gwRows) == 0 {
		t.Fatal("expected gateway rows")
	}
	sel := 0
	for i, row := range gwRows {
		if row.ID == hawkconfig.ProviderXiaomiTokenPlan {
			continue
		}
		sel = i
		break
	}
	m.configSel = sel
	next, cmd := m.handleConfigGatewaysSelect()
	if next.configTab != configTabGateways {
		t.Fatalf("tab = %d, want Gateways", next.configTab)
	}
	if next.configEntry != configEntryAPIKeyPaste {
		t.Fatalf("entry = %q, want API key paste", next.configEntry)
	}
	if next.configProvider != gwRows[sel].ID {
		t.Fatalf("provider = %q, want %s", next.configProvider, gwRows[sel].ID)
	}
	if !strings.Contains(next.configNotice, "Paste API key") {
		t.Fatalf("notice = %q", next.configNotice)
	}
	_ = cmd
}
