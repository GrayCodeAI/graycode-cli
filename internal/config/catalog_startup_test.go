package config

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/catalogtest"
)

func TestCatalogReady_MissingCache(t *testing.T) {
	InvalidateCatalogHealthCache()
	dir := t.TempDir()
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", filepath.Join(dir, "missing.json"))
	if CatalogReady(context.Background()) {
		t.Fatal("expected not ready without cache")
	}
}

func TestCatalogReady_WithCache(t *testing.T) {
	InvalidateCatalogHealthCache()
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
	t.Setenv("GRAYCODE_AUTO_REFRESH_CATALOG", "false")
	if autoRefreshCatalogEnabled() {
		t.Fatal("expected disabled")
	}
	t.Setenv("GRAYCODE_AUTO_REFRESH_CATALOG", "")
	if !autoRefreshCatalogEnabled() {
		t.Fatal("expected enabled by default")
	}
}

func expectGatewayCountsInLine(t *testing.T, line string, rows []gatewayModelCount) {
	t.Helper()
	for _, row := range rows {
		frag := fmt.Sprintf("%s %d", row.Display, row.Count)
		if !strings.Contains(line, frag) {
			t.Fatalf("line %q missing %q", line, frag)
		}
	}
}

func TestFormatCatalogGatewayStatus(t *testing.T) {
	catalogtest.Install(t)
	rows := catalogGatewayModelCounts()
	h := CatalogHealthReport(context.Background())
	line := formatCatalogGatewayStatus("Catalog: ", rows, h.Models)
	expectGatewayCountsInLine(t, line, rows)
	if !strings.HasPrefix(line, "Catalog: ") {
		t.Fatalf("unexpected prefix in %q", line)
	}
	if strings.Contains(line, "ready (") {
		t.Fatalf("expected per-gateway breakdown, got %q", line)
	}
}

func TestCatalogStatusLine_GatewayBreakdown(t *testing.T) {
	catalogtest.Install(t)
	rows := catalogGatewayModelCounts()
	line := CatalogStatusLine(context.Background())
	if strings.Contains(line, "ready (") && strings.Contains(line, "models)") {
		t.Fatalf("expected gateway breakdown, got %q", line)
	}
	expectGatewayCountsInLine(t, line, rows)
	for _, id := range AllSetupGateways() {
		count := CachedModelCountForProvider(id)
		if count <= 0 {
			continue
		}
		frag := fmt.Sprintf("%s %d", GatewayDisplayName(id), count)
		if !strings.Contains(line, frag) {
			t.Fatalf("line %q missing cached count %q for gateway %q", line, frag, id)
		}
	}
}

func TestFormatCatalogGatewayStatus_FallbackTotal(t *testing.T) {
	catalogtest.Install(t)
	h := CatalogHealthReport(context.Background())
	line := formatCatalogGatewayStatus("Catalog: ", nil, h.Models)
	want := fmt.Sprintf("Catalog: ready (%d models)", h.Models)
	if line != want {
		t.Fatalf("line = %q, want %q", line, want)
	}
}

func TestFormatCatalogGatewayStatus_UpdatingPrefix(t *testing.T) {
	catalogtest.Install(t)
	rows := catalogGatewayModelCounts()
	h := CatalogHealthReport(context.Background())
	line := formatCatalogGatewayStatus("Catalog: updating… ", rows, h.Models)
	wantPrefix := fmt.Sprintf("Catalog: updating… %s %d", rows[0].Display, rows[0].Count)
	if !strings.HasPrefix(line, wantPrefix) {
		t.Fatalf("line = %q, want prefix %q", line, wantPrefix)
	}
	expectGatewayCountsInLine(t, line, rows)
}

func TestCatalogGatewayModelCounts_SortedDescending(t *testing.T) {
	catalogtest.Install(t)
	rows := catalogGatewayModelCounts()
	if rows[0].Count < rows[1].Count {
		t.Fatalf("expected descending sort, got %+v", rows)
	}
	for _, row := range rows {
		if row.Count != CachedModelCountForProvider(gatewayIDForDisplay(row.Display)) {
			t.Fatalf("row count %d does not match cache for %q", row.Count, row.Display)
		}
	}
}

func gatewayIDForDisplay(display string) string {
	for _, id := range AllSetupGateways() {
		if GatewayDisplayName(id) == display {
			return id
		}
	}
	return ""
}
