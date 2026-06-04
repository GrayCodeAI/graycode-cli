package routing

import (
	"sync"
	"testing"

	eycatalog "github.com/GrayCodeAI/eyrie/catalog"
)

func TestCostTierOf_CatalogModels(t *testing.T) {
	anthropicHaiku, anthropicSonnet, anthropicOpus := TierModels("anthropic")
	openaiHaiku, openaiSonnet, _ := TierModels("openai")
	geminiHaiku, _, _ := TierModels("gemini")

	tests := []struct {
		model string
		tier  CostTier
	}{
		{anthropicHaiku, CostTierCheap},
		{openaiHaiku, CostTierCheap},
		{geminiHaiku, CostTierCheap},
		{anthropicSonnet, CostTierMid},
		{openaiSonnet, CostTierMid},
		{anthropicOpus, CostTierExpensive},
		{"unknown-model-xyz", CostTierMid},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if tt.model == "" {
				t.Skip("catalog has no model for this tier/provider in test fixture")
			}
			got := CostTierOf(tt.model)
			if got != tt.tier {
				t.Errorf("CostTierOf(%q) = %v, want %v", tt.model, got, tt.tier)
			}
		})
	}
}

func TestPreferredModelForTier(t *testing.T) {
	got := PreferredModelForTier("anthropic", eycatalog.TierHaiku, "")
	if got == "" {
		t.Fatal("expected preferred haiku model for anthropic")
	}
	if CostTierOf(got) != CostTierCheap {
		t.Errorf("preferred haiku model %q should be cheap tier", got)
	}
}

func TestRolesForProvider(t *testing.T) {
	roles := RolesForProvider("anthropic")
	if roles.Planner == "" || roles.Coder == "" || roles.Commit == "" {
		t.Fatal("expected non-empty roles from catalog")
	}
	if CostTierOf(roles.Commit) >= CostTierOf(roles.Planner) {
		t.Errorf("commit tier should be cheaper than planner: %v vs %v", roles.Commit, roles.Planner)
	}
}

func TestCostTierOf_FallsBackWithoutLoadedCatalog(t *testing.T) {
	catalogOnce = sync.Once{}
	catalogOnce.Do(func() {})
	cachedCatalog = nil
	t.Cleanup(func() {
		catalogOnce = sync.Once{}
		cachedCatalog = nil
	})

	if got := CostTierOf("gpt-4o-mini"); got != CostTierCheap {
		t.Fatalf("CostTierOf fallback = %v, want %v", got, CostTierCheap)
	}
}
