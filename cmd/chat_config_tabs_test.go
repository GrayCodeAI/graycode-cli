package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

func TestRenderConfigTabBar_DotIndicators(t *testing.T) {
	got := renderConfigTabBar(configTabGateways)
	for _, label := range configTabLabels {
		if !strings.Contains(got, label) {
			t.Fatalf("missing tab label %q in %q", label, got)
		}
	}
	if strings.Contains(got, "Keys") {
		t.Fatalf("Keys tab should be merged into Gateways, got %q", got)
	}
	if !strings.Contains(got, icons.CircleFilled()) {
		t.Fatalf("expected filled dot for active tab, got %q", got)
	}
	if !strings.Contains(got, icons.CircleOutline()) {
		t.Fatalf("expected empty dot for inactive tabs, got %q", got)
	}
}

func TestOpenConfigPanel_FirstRunOpensGateways(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	m := chatModel{}
	next, _ := m.openConfigPanel()
	if !next.configOpen || next.configTab != configTabGateways {
		t.Fatalf("expected Gateways tab on first open, got open=%v tab=%d", next.configOpen, next.configTab)
	}
	if !strings.Contains(next.configNotice, "Select a gateway") {
		t.Fatalf("notice = %q", next.configNotice)
	}
}
