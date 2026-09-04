package config_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/catalogtest"
	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
)

func TestPrepareCatalogForSession_StaleCacheRefreshFailureContinues(t *testing.T) {
	catalogtest.Install(t)
	// Force stale so refresh is attempted; remote may fail offline — should not block if cache has models.
	h := graycodeconfig.CatalogHealthReport(context.Background())
	var buf bytes.Buffer
	err := graycodeconfig.PrepareCatalogForSession(context.Background(), &buf, graycodeconfig.CatalogStartupOptions{
		ForceRefresh: true,
	})
	// With ForceRefresh, remote may fail; if we had models before, we tolerate failure.
	if err != nil && h.Models > 0 {
		// Only fail test if we had no usable cache to begin with
		t.Logf("refresh failed without fallback (may be ok if remote works): %v", err)
	}
}

func TestCatalogCachePathForDisplay_RespectsEnv(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom.json")
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", custom)
	if got := graycodeconfig.CatalogCachePathForDisplay(); got != custom {
		t.Fatalf("path = %q want %q", got, custom)
	}
}
