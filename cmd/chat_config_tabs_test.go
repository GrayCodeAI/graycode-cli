package cmd

import (
	"strings"
	"testing"
)

func TestRenderConfigTabBar_DotIndicators(t *testing.T) {
	got := renderConfigTabBar(configTabGateways)
	for _, label := range configTabLabels {
		if !strings.Contains(got, label) {
			t.Fatalf("missing tab label %q in %q", label, got)
		}
	}
	if !strings.Contains(got, configTabDotFilled) {
		t.Fatalf("expected filled dot for active tab, got %q", got)
	}
	if !strings.Contains(got, configTabDotEmpty) {
		t.Fatalf("expected empty dot for inactive tabs, got %q", got)
	}
}
