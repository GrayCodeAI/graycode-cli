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
		t.Fatalf("catalog missing %s tier model for %s", tier, defaultPackProvider)
	}
	return m
}
