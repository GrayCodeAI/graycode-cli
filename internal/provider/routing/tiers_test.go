package routing

import "testing"

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
				// FIXME: catalog has no model for this tier/provider in test fixture
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
	got := PreferredModelForTier("unknown-provider-xyz", TierHaiku, "")
	if got != "" {
		t.Fatalf("expected empty haiku model for unknown provider, got %q", got)
	}
}

func TestPreferredModelForTier_WithFallback(t *testing.T) {
	got := PreferredModelForTier("unknown-provider-xyz", TierHaiku, "fallback-model")
	if got != "fallback-model" {
		t.Fatalf("expected fallback model, got %q", got)
	}
}

func TestRolesForProvider_NilCatalog(t *testing.T) {
	roles := RolesForProvider("unknown-provider-xyz")
	if roles.Planner != "" || roles.Coder != "" || roles.Commit != "" {
		t.Fatal("expected empty roles for unknown provider")
	}
}

func TestCostTierOf_FallsBackWithoutCatalogMatch(t *testing.T) {
	if got := CostTierOf("gpt-4o-mini"); got != CostTierCheap {
		t.Fatalf("CostTierOf fallback = %v, want %v", got, CostTierCheap)
	}
}
