package config

import (
	"testing"

	"github.com/GrayCodeAI/hawk/internal/provider/routing"

	eycatalog "github.com/GrayCodeAI/eyrie/catalog"
)

func testPackModel(t *testing.T, tier eycatalog.ModelTier) string {
	t.Helper()
	m := routing.PreferredModelForTier(defaultPackProvider, tier, "")
	if m == "" {
		// Without a live catalog, no models are available (fully dynamic)
		t.Skipf("no %s tier model for %s without live catalog", tier, defaultPackProvider)
	}
	return m
}
