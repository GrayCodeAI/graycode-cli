package engine

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/hawk/internal/provider/routing"
)

const testProvider = "anthropic"

func testTierModels(t *testing.T, provider string) (haiku, sonnet, opus string) {
	t.Helper()
	haiku = routing.PreferredModelForTier(provider, catalog.TierHaiku, "")
	sonnet = routing.PreferredModelForTier(provider, catalog.TierSonnet, "")
	opus = routing.PreferredModelForTier(provider, catalog.TierOpus, "")
	if haiku == "" || sonnet == "" || opus == "" {
		t.Fatalf("eyrie catalog missing tier models for provider %q", provider)
	}
	return haiku, sonnet, opus
}
