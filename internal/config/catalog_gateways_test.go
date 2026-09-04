package config

import (
	"context"
	"fmt"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/graycode-cli/internal/catalogtest"
)

func TestIsCatalogCacheRequired(t *testing.T) {
	err := fmt.Errorf("apply credentials: %w", catalog.ErrCatalogCacheRequired)
	if !IsCatalogCacheRequired(err) {
		t.Fatal("wrapped catalog-cache error was not recognized")
	}
	if IsCatalogCacheRequired(fmt.Errorf("authentication failed")) {
		t.Fatal("unrelated error was classified as a catalog-cache error")
	}
}

func TestAllSetupGateways_RegistryOnly(t *testing.T) {
	gws := AllSetupGateways()
	if got, want := len(gws), RegisteredProviderCount(); got != want {
		t.Fatalf("setup gateways = %d, registered providers = %d: %v", got, want, gws)
	}
	for _, id := range gws {
		if id == "ai21" || id == "alibaba" {
			t.Fatalf("owner slug %q should not be a gateway", id)
		}
	}
	// Required gateways that exist in the published eyrie v0.1.3.
	want := map[string]bool{"azure": true, "bedrock": true, "gemini": true, "grok": true, "openrouter": true, "kimi": true, "vertex": true, "xiaomi_mimo_payg": true, "xiaomi_mimo_token_plan": true, "deepseek": true, "minimax_token_plan": true, "minimax_payg": true, "zai_payg": true, "zai_coding": true, "agnes": true}
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
	// Newly-added gateways (groq, poolside) present in the local Eyrie checkout
	// but not yet in a published eyrie release — log if present, don't
	// require them so the test passes with both GOWORK=on and GOWORK=off.
	for _, extra := range []string{"groq", "poolside"} {
		if containsString(gws, extra) {
			t.Logf("extra gateway %q present (local eyrie build)", extra)
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

func TestSetupGatewayID_PreservesUnderscores(t *testing.T) {
	if got := ActiveProviderID("xiaomi_mimo_payg"); got != "xiaomi_mimo_payg" {
		t.Fatalf("xiaomi_mimo_payg = %q", got)
	}
	if got := ActiveProviderID("xiaomi_mimo"); got != "xiaomi_mimo_payg" {
		t.Fatalf("legacy xiaomi_mimo = %q", got)
	}
	if got := ActiveProviderID("zai_payg"); got != "zai_payg" {
		t.Fatalf("zai_payg = %q", got)
	}
	if got := ActiveProviderID("z-ai-payg"); got != "zai_payg" {
		t.Fatalf("z-ai-payg = %q", got)
	}
	if got := ActiveProviderID("zai_coding"); got != "zai_coding" {
		t.Fatalf("zai_coding = %q", got)
	}
	if got := ActiveProviderID("z-ai-coding"); got != "zai_coding" {
		t.Fatalf("z-ai-coding = %q", got)
	}
}

func TestCredentialInferenceForProvider_XiaomiPayg(t *testing.T) {
	inf, err := CredentialInferenceForProvider("xiaomi_mimo_payg")
	if err != nil {
		t.Fatal(err)
	}
	if inf.ProviderID != "xiaomi_mimo_payg" || inf.EnvVar != "XIAOMI_MIMO_PAYG_API_KEY" {
		t.Fatalf("unexpected inference: %+v", inf)
	}
}

func TestHasStoredCredentialForProvider_XiaomiPayg(t *testing.T) {
	InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		InvalidateConfigUICache()
	})
	_ = store.Set(context.Background(), credentials.AccountForEnv("XIAOMI_MIMO_PAYG_API_KEY"), "test-xiaomi-key-12345678")
	if env := SetupGatewayCredentialEnv("xiaomi_mimo_payg"); env != "XIAOMI_MIMO_PAYG_API_KEY" {
		t.Fatalf("env = %q", env)
	}
	if !HasStoredCredentialForProvider(context.Background(), "xiaomi_mimo_payg") {
		t.Fatal("expected stored credential for xiaomi_mimo_payg")
	}
	RefreshConfigCredSnapshot(context.Background())
	found := false
	for _, p := range configuredCredentialProvidersCached(context.Background()) {
		if p == "xiaomi_mimo_payg" {
			found = true
		}
	}
	if !found {
		t.Fatal("cred cache should include xiaomi_mimo_payg")
	}
}

func TestGatewayDisplayName(t *testing.T) {
	if got := GatewayDisplayName("openrouter"); got != "OpenRouter" {
		t.Fatalf("display name = %q", got)
	}
	if got := GatewayDisplayName("google"); got != "Gemini API" {
		t.Fatalf("google alias display = %q", got)
	}
	if got := GatewayDisplayName("gemini"); got != "Gemini API" {
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
