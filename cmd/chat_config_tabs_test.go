package cmd

import (
	"strings"
	"testing"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/provider/gateway"
	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
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
	graycodeconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		graycodeconfig.InvalidateConfigUICache()
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

func TestOpenConfigAtTab_ModelsWithoutCredentialsFallsBackToGateways(t *testing.T) {
	graycodeconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		graycodeconfig.InvalidateConfigUICache()
	})

	m := chatModel{}
	next, _ := m.openConfigAtTab(configTabModels)
	if !next.configOpen || next.configTab != configTabGateways {
		t.Fatalf("expected Gateways fallback, got open=%v tab=%d", next.configOpen, next.configTab)
	}
	if !strings.Contains(next.configNotice, "Select a gateway") {
		t.Fatalf("notice = %q", next.configNotice)
	}
}
