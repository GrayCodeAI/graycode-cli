package cmd

import (
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
)

func TestStartConfigXiaomiTokenPlanRegion_WithSavedRegion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	_ = hawkconfig.SetXiaomiTokenPlanRegion("ams")

	m := chatModel{}.startConfigXiaomiTokenPlanRegion()
	if m.configEntry != configEntryXiaomiRegion {
		t.Fatalf("entry = %q", m.configEntry)
	}
	if m.configXiaomiRegionSel != 2 {
		t.Fatalf("sel = %d, want 2 (ams)", m.configXiaomiRegionSel)
	}
	if !strings.Contains(m.configNotice, "ams") {
		t.Fatalf("notice = %q", m.configNotice)
	}
}

func TestHandleConfigGatewaysSelect_TokenPlanNoKeyShowsRegionPicker(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
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
		t.Fatal("missing token plan gateway row")
	}
	m.configSel = idx
	next, _ := m.handleConfigGatewaysSelect()
	if next.configEntry != configEntryXiaomiRegion {
		t.Fatalf("entry = %q, want region picker", next.configEntry)
	}
}
