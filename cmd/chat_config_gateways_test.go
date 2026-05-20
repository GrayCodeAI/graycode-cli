package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestConfigGatewaysView_CatalogHeaderWithoutKeys(t *testing.T) {
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
	if !strings.Contains(view, "add key in Keys tab") {
		t.Fatalf("expected keys hint without credentials, got:\n%s", view)
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
