package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/catalogtest"
)

func TestCatalogReady_MissingCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EYRIE_MODEL_CATALOG_PATH", filepath.Join(dir, "missing.json"))
	if CatalogReady(context.Background()) {
		t.Fatal("expected not ready without cache")
	}
}

func TestCatalogReady_WithCache(t *testing.T) {
	catalogtest.Install(t)
	h := CatalogHealthReport(context.Background())
	if h.Error != "" || h.Models == 0 {
		t.Fatalf("unexpected health: %+v", h)
	}
	// Fixture may or may not be stale; CatalogReady requires non-stale.
	if h.Stale && CatalogReady(context.Background()) {
		t.Fatal("expected not ready while stale")
	}
	if !h.Stale && !CatalogReady(context.Background()) {
		t.Fatal("expected ready when cache is fresh")
	}
}

func TestCatalogNeedsAutoRefresh_Stale(t *testing.T) {
	h := CatalogHealth{Models: 10, Stale: true}
	if !catalogNeedsAutoRefresh(h, CatalogStartupOptions{}) {
		t.Fatal("expected auto refresh when stale")
	}
}

func TestCatalogNeedsAutoRefresh_Fresh(t *testing.T) {
	h := CatalogHealth{Models: 10, Stale: false}
	if catalogNeedsAutoRefresh(h, CatalogStartupOptions{}) {
		t.Fatal("expected no refresh when fresh")
	}
}

func TestAutoRefreshCatalogEnabled(t *testing.T) {
	t.Setenv("HAWK_AUTO_REFRESH_CATALOG", "false")
	if autoRefreshCatalogEnabled() {
		t.Fatal("expected disabled")
	}
	t.Setenv("HAWK_AUTO_REFRESH_CATALOG", "")
	if !autoRefreshCatalogEnabled() {
		t.Fatal("expected enabled by default")
	}
}
