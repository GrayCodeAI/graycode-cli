package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog"
)

// CatalogHealth summarizes the on-disk eyrie model catalog for doctor / status output.
type CatalogHealth struct {
	CachePath   string
	Exists      bool
	Modified    time.Time
	SizeBytes   int64
	Models      int
	Deployments int
	Offerings   int
	Stale       bool
	StaleAfter  time.Time
	Source      string
	Error       string
}

// CatalogHealthReport inspects ~/.eyrie/model_catalog.json (or EYRIE_MODEL_CATALOG_PATH).
func CatalogHealthReport(ctx context.Context) CatalogHealth {
	path := catalog.DefaultCachePath()
	h := CatalogHealth{CachePath: path}
	exists, mod, size, err := catalog.CacheInfo(path)
	if err != nil {
		h.Error = err.Error()
		return h
	}
	h.Exists = exists
	h.Modified = mod
	h.SizeBytes = size
	if !exists {
		h.Error = "cache missing — hawk will discover automatically on start"
		return h
	}
	compiled, err := catalog.LoadCatalogV1(ctx, catalog.LoadCatalogV1Options{
		CachePath:    path,
		RequireCache: true,
	})
	if err != nil {
		h.Error = err.Error()
		return h
	}
	h.Models = len(compiled.ModelsByID)
	h.Deployments = len(compiled.DeploymentsByID)
	h.Offerings = len(compiled.OfferingsByID)
	if compiled.Catalog != nil && compiled.Catalog.Provenance != nil {
		h.Source = compiled.Catalog.Provenance.Source
	}
	if compiled.Catalog != nil && !compiled.Catalog.StaleAfter.IsZero() {
		h.StaleAfter = compiled.Catalog.StaleAfter
		h.Stale = time.Now().UTC().After(compiled.Catalog.StaleAfter)
	}
	return h
}

// FormatCatalogHealth returns human-readable catalog status for hawk doctor.
func FormatCatalogHealth(h CatalogHealth) string {
	var b strings.Builder
	b.WriteString("Model catalog (eyrie):\n")
	b.WriteString(fmt.Sprintf("  path: %s\n", h.CachePath))
	if h.Error != "" {
		b.WriteString(fmt.Sprintf("  status: %s\n", h.Error))
		return strings.TrimRight(b.String(), "\n")
	}
	b.WriteString(fmt.Sprintf("  modified: %s (%d bytes)\n", h.Modified.UTC().Format(time.RFC3339), h.SizeBytes))
	if h.Source != "" {
		b.WriteString(fmt.Sprintf("  source: %s\n", h.Source))
	}
	b.WriteString(fmt.Sprintf("  models: %d  deployments: %d  offerings: %d\n", h.Models, h.Deployments, h.Offerings))
	if h.Stale {
		b.WriteString(fmt.Sprintf("  stale: yes (after %s) — hawk refreshes automatically on start\n", h.StaleAfter.UTC().Format(time.RFC3339)))
	} else if !h.StaleAfter.IsZero() {
		b.WriteString(fmt.Sprintf("  stale: no (until %s)\n", h.StaleAfter.UTC().Format(time.RFC3339)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// EnsureCatalogAvailable returns an error when the production catalog cache is missing or empty.
func EnsureCatalogAvailable(ctx context.Context) error {
	h := CatalogHealthReport(ctx)
	if h.Error != "" {
		return fmt.Errorf("%s", h.Error)
	}
	if h.Models == 0 {
		return fmt.Errorf("model catalog has no models — hawk will refresh automatically when API keys are set")
	}
	return nil
}

// CatalogCachePathForDisplay returns the path users should care about.
func CatalogCachePathForDisplay() string {
	if p := strings.TrimSpace(os.Getenv("EYRIE_MODEL_CATALOG_PATH")); p != "" {
		return p
	}
	return catalog.DefaultCachePath()
}
