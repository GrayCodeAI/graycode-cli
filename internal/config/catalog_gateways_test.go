package config

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/hawk/internal/catalogtest"
)

func TestAllSetupGateways_RegistryOnly(t *testing.T) {
	gws := AllSetupGateways()
	if len(gws) != 11 {
		t.Fatalf("expected 11 setup gateways, got %d: %v", len(gws), gws)
	}
	for _, id := range gws {
		if id == "ai21" || id == "alibaba" {
			t.Fatalf("owner slug %q should not be a gateway", id)
		}
	}
	want := map[string]bool{"gemini": true, "grok": true, "openrouter": true, "kimi": true, "xiaomi": true}
	for id := range want {
		found := false
		for _, gw := range gws {
			if gw == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing setup gateway %q in %v", id, gws)
		}
	}
	if containsString(gws, "google") || containsString(gws, "xai") {
		t.Fatalf("setup gateways should use registry ids, got %v", gws)
	}
	all := AllCatalogProviders()
	if len(all) <= len(gws) {
		t.Logf("catalog providers=%d setup gateways=%d (ok if catalog is bootstrap-only)", len(all), len(gws))
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func TestGatewayDisplayName(t *testing.T) {
	if got := GatewayDisplayName("openrouter"); got != "OpenRouter" {
		t.Fatalf("display name = %q", got)
	}
	if got := GatewayDisplayName("google"); got != "Google Gemini" {
		t.Fatalf("google alias display = %q", got)
	}
	if got := GatewayDisplayName("gemini"); got != "Google Gemini" {
		t.Fatalf("gemini display = %q", got)
	}
}

func TestCachedModelCountForProvider_MatchesEyrieList(t *testing.T) {
	catalogtest.Install(t)
	compiled := CompiledCatalogV1()
	for _, gw := range AllSetupGateways() {
		count := CachedModelCountForProvider(gw)
		entries := catalog.ModelEntriesForProvider(compiled, gw)
		if count != len(entries) {
			t.Fatalf("%s: CachedModelCountForProvider=%d len(entries)=%d", gw, count, len(entries))
		}
	}
}
