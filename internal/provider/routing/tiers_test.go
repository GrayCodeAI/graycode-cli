package routing

import (
	"sync"
	"testing"

	eycatalog "github.com/GrayCodeAI/eyrie/catalog"
)

func TestCostTierOf_CatalogModels(t *testing.T) {
	tests := []struct {
		model string
		tier  CostTier
	}{
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

func TestPreferredModelForTier_NilCatalog(t *testing.T) {
	// Without a catalog, PreferredModelForTier returns empty
	got := PreferredModelForTier("anthropic", eycatalog.TierHaiku, "")
	if got != "" {
		t.Fatalf("expected empty haiku model without catalog, got %q", got)
	}
}

func TestPreferredModelForTier_WithFallback(t *testing.T) {
	// With a fallback, it returns the fallback
	got := PreferredModelForTier("anthropic", eycatalog.TierHaiku, "fallback-model")
	if got != "fallback-model" {
		t.Fatalf("expected fallback model, got %q", got)
	}
}

func TestRolesForProvider_NilCatalog(t *testing.T) {
	// Without a catalog, roles are empty
	roles := RolesForProvider("anthropic")
	if roles.Planner != "" || roles.Coder != "" || roles.Commit != "" {
		t.Fatal("expected empty roles without catalog")
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
