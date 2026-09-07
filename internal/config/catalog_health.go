package config

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/theme"
)

var (
	catalogHealthMu    sync.Mutex
	catalogHealthCache CatalogHealth
	catalogHealthAt    time.Time
)

const catalogHealthCacheTTL = 15 * time.Second

// CatalogHealth summarizes the on-disk graycode-router model catalog for doctor / status output.
type CatalogHealth struct {
	CachePath   string    `json:"cache_path"`
	Exists      bool      `json:"exists"`
	Modified    time.Time `json:"modified,omitempty"`
	SizeBytes   int64     `json:"size_bytes,omitempty"`
	Models      int       `json:"models,omitempty"`
	Deployments int       `json:"deployments,omitempty"`
	Offerings   int       `json:"offerings,omitempty"`
	Stale       bool      `json:"stale,omitempty"`
	StaleAfter  time.Time `json:"stale_after,omitempty"`
	Source      string    `json:"source,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// CatalogHealthReport inspects ~/.graycode-router/model_catalog.json (or GRAYCODE_ROUTER_MODEL_CATALOG_PATH).
func CatalogHealthReport(ctx context.Context) CatalogHealth {
	path := CatalogCachePathForDisplay()
	catalogHealthMu.Lock()
	if !catalogHealthAt.IsZero() && time.Since(catalogHealthAt) < catalogHealthCacheTTL && catalogHealthCache.CachePath == path {
		h := catalogHealthCache
		catalogHealthMu.Unlock()
		return h
	}
	catalogHealthMu.Unlock()

	h := catalogHealthReportUncached(ctx)

	catalogHealthMu.Lock()
	catalogHealthCache = h
	catalogHealthAt = time.Now()
	catalogHealthMu.Unlock()
	return h
}

// InvalidateCatalogHealthCache drops cached catalog health (after refresh).
func InvalidateCatalogHealthCache() {
	catalogHealthMu.Lock()
	catalogHealthAt = time.Time{}
	catalogHealthMu.Unlock()
}

func catalogHealthReportUncached(ctx context.Context) CatalogHealth {
	engine, err := newGraycodeRouterEngine()
	if err != nil {
		return CatalogHealth{Error: err.Error()}
	}
	status := engine.CatalogHealth(ctx)
	h := CatalogHealth{
		CachePath: status.Path, Exists: status.Exists, Modified: status.ModifiedAt,
		SizeBytes: status.Size, Models: status.Models, Deployments: status.Deployments,
		Offerings: status.Offerings, Stale: status.Stale, StaleAfter: status.StaleAfter,
		Source: status.Source, Error: status.Error,
	}
	if !h.Exists && h.Error == "" {
		h.Error = "cache missing — graycode will discover automatically on start"
	}
	return h
}

// FormatCatalogHealth returns human-readable catalog status for graycode doctor.
func FormatCatalogHealth(h CatalogHealth) string {
	var b strings.Builder
	b.WriteString(theme.Tint("Model catalog (graycode-router):", theme.ReportInfo) + "\n")
	b.WriteString("  " + theme.Tint("path:", theme.ReportMuted) + " " + theme.Tint(h.CachePath, theme.ReportInfo) + "\n")
	if h.Error != "" {
		b.WriteString("  " + theme.Tint("status:", theme.ReportMuted) + " " + theme.Tint(h.Error, theme.ReportError) + "\n")
		return strings.TrimRight(b.String(), "\n")
	}
	b.WriteString("  " + theme.Tint("modified:", theme.ReportMuted) + " " + theme.Tint(h.Modified.UTC().Format(time.RFC3339), theme.ReportInfo) + fmt.Sprintf(" (%d bytes)", h.SizeBytes) + "\n")
	if h.Source != "" {
		b.WriteString("  " + theme.Tint("source:", theme.ReportMuted) + " " + theme.Tint(h.Source, theme.ReportInfo) + "\n")
	}
	b.WriteString("  " + theme.Tint("models:", theme.ReportMuted) + " " + theme.Tint(fmt.Sprintf("%d", h.Models), theme.ReportInfo) + "  " + theme.Tint("deployments:", theme.ReportMuted) + " " + theme.Tint(fmt.Sprintf("%d", h.Deployments), theme.ReportInfo) + "  " + theme.Tint("offerings:", theme.ReportMuted) + " " + theme.Tint(fmt.Sprintf("%d", h.Offerings), theme.ReportInfo) + "\n")
	if h.Stale {
		b.WriteString("  " + theme.Tint("stale:", theme.ReportMuted) + " " + theme.Tint("yes", theme.ReportWarn) + fmt.Sprintf(" (after %s) — graycode refreshes automatically on start\n", h.StaleAfter.UTC().Format(time.RFC3339)))
	} else if !h.StaleAfter.IsZero() {
		b.WriteString("  " + theme.Tint("stale:", theme.ReportMuted) + " " + theme.Tint("no", theme.ReportSuccess) + fmt.Sprintf(" (until %s)\n", h.StaleAfter.UTC().Format(time.RFC3339)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// CatalogEmptyHint returns actionable guidance when the catalog has no models.
func CatalogEmptyHint(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	if !HasConfiguredDeploymentCached(ctx) {
		return "run /config to paste an API key or set up Ollama (local, no key)"
	}
	return "check network access, then graycode preflight or /config — graycode refreshes the catalog automatically"
}

// EnsureCatalogAvailable returns an error when the production catalog cache is missing or empty.
func EnsureCatalogAvailable(ctx context.Context) error {
	h := CatalogHealthReport(ctx)
	if h.Error != "" {
		if !h.Exists {
			return fmt.Errorf("model catalog cache missing — %s", CatalogEmptyHint(ctx))
		}
		return fmt.Errorf("%s — %s", h.Error, CatalogEmptyHint(ctx))
	}
	if h.Models == 0 {
		return fmt.Errorf("model catalog has no models — %s", CatalogEmptyHint(ctx))
	}
	return nil
}

// CatalogCachePathForDisplay returns the path users should care about.
func CatalogCachePathForDisplay() string {
	engine, err := newGraycodeRouterEngine()
	if err == nil {
		return engine.StatePaths().Catalog
	}
	return ""
}
