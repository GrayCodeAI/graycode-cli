package cmd

import (
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

func TestModelStatusMeta_UsesLiveModelCache(t *testing.T) {
	provider := "xiaomi_mimo_token_plan"
	modelID := "mimo-v2.5-pro"
	modelCacheMu.Lock()
	modelCache[provider] = []configModelOption{{
		ID:               modelID,
		DisplayName:      "MiMo V2.5 Pro",
		ContextWindow:    1_048_576,
		PriceKnown:       true,
		InputPricePer1M:  0.5,
		OutputPricePer1M: 2.0,
	}}
	modelCacheMu.Unlock()
	t.Cleanup(func() {
		InvalidateModelCacheProvider(provider)
	})

	_, ctxLabel := modelStatusMeta(provider, modelID)
	if ctxLabel != "1.0m" {
		t.Fatalf("context label = %q, want 1.0m from live cache", ctxLabel)
	}
}

func TestPlatformContextForNativeModel_MimoV25Pro(t *testing.T) {
	invalidatePlatformContextCache()
	seedPlatformContextCacheForTest(map[string]int{"mimo-v2.5-pro": 1_048_576})
	t.Cleanup(invalidatePlatformContextCache)
	w := platformContextForNativeModel("mimo-v2.5-pro")
	if w < 1_000_000 {
		t.Fatalf("platform context = %d, want >= 1M", w)
	}
}

func TestConnectionStatusParts_OmitsDefault128kPlaceholder(t *testing.T) {
	m := chatModel{session: &engine.Session{}}
	m.session.SetModel("mimo-v2.5-pro")
	m.session.SetProvider("xiaomi_mimo_token_plan")
	_, _, ctxLabel := m.connectionStatusParts()
	if ctxLabel == "128k" {
		t.Fatalf("footer should not show DefaultContextWindow placeholder, got %q", ctxLabel)
	}
}

func TestConnectionStatusParts_MimoShowsPlatformContext(t *testing.T) {
	invalidatePlatformContextCache()
	seedPlatformContextCacheForTest(map[string]int{"mimo-v2.5-pro": 1_048_576})
	t.Cleanup(invalidatePlatformContextCache)
	sess := &engine.Session{}
	sess.SetProvider("xiaomi_mimo_token_plan")
	sess.SetModel("mimo-v2.5-pro")
	applyLiveModelMetadata(sess, "xiaomi_mimo_token_plan", "mimo-v2.5-pro")

	m := chatModel{session: sess}
	_, _, ctxLabel := m.connectionStatusParts()
	if ctxLabel != "1.0m" {
		t.Fatalf("context label = %q, want 1.0m", ctxLabel)
	}
}

func TestApplyLiveModelMetadata_SetsSessionWindow(t *testing.T) {
	provider := "xiaomi_mimo_token_plan"
	modelID := "mimo-v2.5-pro"
	modelCacheMu.Lock()
	modelCache[provider] = []configModelOption{{
		ID:            modelID,
		ContextWindow: 1_048_576,
	}}
	modelCacheMu.Unlock()
	t.Cleanup(func() {
		InvalidateModelCacheProvider(provider)
	})

	sess := engine.NewSession("p", modelID, "", nil)
	applyLiveModelMetadata(sess, provider, modelID)
	if got := sess.ContextWindowSize(); got != 1_048_576 {
		t.Fatalf("ContextWindowSize() = %d, want 1048576", got)
	}
}

func TestIsXiaomiMimoProvider_AcceptsHyphenAndUnderscore(t *testing.T) {
	cases := []string{
		"xiaomi_mimo_token_plan",
		"xiaomi-mimo-token-plan",
		"XIAOMI_MIMO_TOKEN_PLAN",
		"xiaomi_mimo_payg",
		"xiaomi-mimo-payg",
		"xiaomi_mimo",
		"xiaomi-mimo",
	}
	for _, c := range cases {
		if !isXiaomiMimoProvider(c) {
			t.Errorf("isXiaomiMimoProvider(%q) = false, want true", c)
		}
	}
	if isXiaomiMimoProvider("anthropic") || isXiaomiMimoProvider("openai") {
		t.Error("non-xiaomi providers should return false")
	}
}

func TestConnectionStatusParts_MimoShowsPlatformContext_HyphenProvider(t *testing.T) {
	invalidatePlatformContextCache()
	seedPlatformContextCacheForTest(map[string]int{"mimo-v2.5-pro": 1_048_576})
	t.Cleanup(invalidatePlatformContextCache)
	sess := &engine.Session{}
	sess.SetProvider("xiaomi-mimo-token-plan") // hyphenated as normalized at runtime
	sess.SetModel("mimo-v2.5-pro")
	applyLiveModelMetadata(sess, "xiaomi-mimo-token-plan", "mimo-v2.5-pro")

	m := chatModel{session: sess}
	_, _, ctxLabel := m.connectionStatusParts()
	if ctxLabel != "1.0m" {
		t.Fatalf("context label = %q, want 1.0m for hyphen provider", ctxLabel)
	}
}

func TestGatewayDisplayName_XiaomiTokenPlanHyphen(t *testing.T) {
	if got := hawkconfig.GatewayDisplayName("xiaomi-mimo-token-plan"); got != "Xiaomi MiMo — Token Plan" {
		t.Fatalf("GatewayDisplayName(hyphen) = %q, want nice name", got)
	}
	if got := hawkconfig.GatewayDisplayName("xiaomi_mimo_token_plan"); got != "Xiaomi MiMo — Token Plan" {
		t.Fatalf("GatewayDisplayName(underscore) = %q, want nice name", got)
	}
}
