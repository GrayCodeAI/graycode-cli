package engine

import (
	"testing"

	"github.com/GrayCodeAI/hawk/internal/provider/routing"
)

const testProvider = "anthropic"

func testTierModels(t *testing.T, provider string) (haiku, sonnet, opus string) {
	t.Helper()
	haiku = routing.PreferredModelForTier(provider, routing.TierHaiku, "")
	sonnet = routing.PreferredModelForTier(provider, routing.TierSonnet, "")
	opus = routing.PreferredModelForTier(provider, routing.TierOpus, "")
	if haiku == "" || sonnet == "" || opus == "" {
		t.Skipf("no tier models for %q without live catalog (fully dynamic)", provider)
	}
	return haiku, sonnet, opus
}
