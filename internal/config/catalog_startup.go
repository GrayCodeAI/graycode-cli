package config

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/hawk/internal/env"
)

type gatewayModelCount struct {
	Display string
	Count   int
}

// catalogGatewayModelCounts returns cached model counts per setup gateway (non-zero only).
func catalogGatewayModelCounts() []gatewayModelCount {
	var out []gatewayModelCount
	for _, id := range AllSetupGateways() {
		n := CachedModelCountForProvider(id)
		if n <= 0 {
			continue
		}
		out = append(out, gatewayModelCount{
			Display: GatewayDisplayName(id),
			Count:   n,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Display < out[j].Display
	})
	return out
}

func formatCatalogGatewayStatus(prefix string, rows []gatewayModelCount, total int) string {
	if len(rows) == 0 {
		if total > 0 {
			return fmt.Sprintf("%sready (%d models)", prefix, total)
		}
		return prefix + "empty"
	}
	parts := make([]string, len(rows))
	for i, row := range rows {
		parts[i] = fmt.Sprintf("%s %d", row.Display, row.Count)
	}
	return prefix + strings.Join(parts, " · ")
}

// CatalogStatusLine returns a short one-line status for the TUI welcome banner.
func CatalogStatusLine(ctx context.Context) string {
	h := CatalogHealthReport(ctx)
	if h.Error != "" {
		if !h.Exists {
			return "Catalog: missing — " + CatalogEmptyHint(ctx)
		}
		return "Catalog: unavailable — " + CatalogEmptyHint(ctx)
	}
	if h.Models == 0 {
		return "Catalog: empty — " + CatalogEmptyHint(ctx)
	}
	rows := catalogGatewayModelCounts()
	if h.Stale {
		return formatCatalogGatewayStatus("Catalog: updating… ", rows, h.Models)
	}
	return formatCatalogGatewayStatus("Catalog: ", rows, h.Models)
}

// CatalogReady reports whether the eyrie catalog cache exists and has models.
func CatalogReady(ctx context.Context) bool {
	h := CatalogHealthReport(ctx)
	return h.Error == "" && h.Models > 0 && !h.Stale
}

// CatalogStartupOptions controls automatic catalog refresh at hawk startup.
type CatalogStartupOptions struct {
	ForceRefresh    bool
	SkipAutoRefresh bool
	VerboseOutput   bool // full DiscoverReport; default is one line
}

// PrepareCatalogForSession ensures a usable, fresh catalog before chat/print.
// By default hawk auto-discovers when the cache is missing, empty, or stale.
func PrepareCatalogForSession(ctx context.Context, out io.Writer, opts CatalogStartupOptions) error {
	h := CatalogHealthReport(ctx)
	if !catalogNeedsAutoRefresh(h, opts) {
		return nil
	}
	hadUsableCache := h.Error == "" && h.Models > 0
	if hadUsableCache && !opts.ForceRefresh && !catalogRefreshAlways() {
		// A stale-but-usable cache is good enough to start with: refresh in
		// the background instead of blocking startup on the network (print
		// mode would otherwise stall up to 90s before the first token).
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			_ = AutoRefreshCatalog(bgCtx, nil, false)
		}()
		return nil
	}
	if err := AutoRefreshCatalog(ctx, out, opts.VerboseOutput); err != nil {
		if hadUsableCache {
			if out != nil {
				_, _ = fmt.Fprintf(out, "Catalog refresh skipped (using %d cached models): %v\n", h.Models, err)
			}
			return nil
		}
		return fmt.Errorf("automatic catalog refresh failed: %w\n\n%s\nCache path: %s", err, catalogRefreshFailureHint(ctx), CatalogCachePathForDisplay())
	}
	h = CatalogHealthReport(ctx)
	if h.Error != "" || h.Models == 0 {
		if hadUsableCache {
			return nil
		}
		msg := "model catalog unavailable after refresh"
		if h.Error != "" {
			msg = h.Error
		}
		return fmt.Errorf("%s\n\n%s\nCache path: %s", msg, catalogRefreshFailureHint(ctx), CatalogCachePathForDisplay())
	}
	return nil
}

func catalogNeedsAutoRefresh(h CatalogHealth, opts CatalogStartupOptions) bool {
	if opts.SkipAutoRefresh && !opts.ForceRefresh {
		return false
	}
	if opts.ForceRefresh {
		return true
	}
	if !autoRefreshCatalogEnabled() {
		return false
	}
	if catalogRefreshAlways() {
		return true
	}
	if h.Error != "" || h.Models == 0 {
		return true
	}
	return h.Stale
}

// AutoRefreshCatalog runs eyrie discover (remote + live APIs when keys are set).
func AutoRefreshCatalog(ctx context.Context, out io.Writer, verbose bool) error {
	if out != nil {
		if verbose {
			_, _ = fmt.Fprintln(out, "Discovering model catalog (published catalog + live provider APIs)...")
		} else {
			_, _ = fmt.Fprintln(out, "Updating model catalog automatically…")
		}
	}
	refreshCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	result, err := refreshModelCatalog(refreshCtx, false)
	if err != nil {
		return err
	}
	compiled, loadErr := loadEyrieCatalogV1(refreshCtx, false)
	if loadErr == nil && compiled != nil {
		storeCompiledCatalog(compiled)
	}
	InvalidateConfigUICache()
	if out != nil {
		if verbose {
			_, _ = fmt.Fprintln(out, strings.TrimSpace(formatCatalogSnapshot(result)))
		} else if compiled != nil {
			_, _ = fmt.Fprintf(
				out, "Catalog ready: %d models, %d deployments → %s\n",
				len(compiled.ModelsByID),
				len(compiled.DeploymentsByID),
				result.CachePath,
			)
		}
		_, _ = fmt.Fprintln(out)
	}
	return nil
}

// TryAutoRefreshCatalog refreshes once when the cache cannot be read (e.g. mid-session).
func TryAutoRefreshCatalog(ctx context.Context) error {
	if !autoRefreshCatalogEnabled() {
		return fmt.Errorf("automatic catalog refresh is disabled (HAWK_AUTO_REFRESH_CATALOG=0)")
	}
	return AutoRefreshCatalog(ctx, nil, false)
}

// RefreshCatalogAfterCredentials runs eyrie discover after /config saves API keys.
func RefreshCatalogAfterCredentials(ctx context.Context, out io.Writer) error {
	if !autoRefreshCatalogEnabled() {
		return nil
	}
	return AutoRefreshCatalog(ctx, out, false)
}

// StartupCatalogPrefetch refreshes the catalog in the background when the cache needs it.
func StartupCatalogPrefetch(ctx context.Context) {
	if !autoRefreshCatalogEnabled() {
		return
	}
	h := CatalogHealthReport(ctx)
	if !catalogNeedsAutoRefresh(h, CatalogStartupOptions{}) {
		return
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		_ = AutoRefreshCatalog(bgCtx, nil, false)
	}()
}

// DiscoverCatalogAfterSetup runs during optional hawk setup after API keys are saved.
func DiscoverCatalogAfterSetup(ctx context.Context, out io.Writer) {
	if out == nil {
		out = os.Stdout
	}
	h := CatalogHealthReport(ctx)
	if !catalogNeedsAutoRefresh(h, CatalogStartupOptions{}) {
		return
	}
	_ = AutoRefreshCatalog(ctx, out, false)
}

func catalogRefreshFailureHint(ctx context.Context) string {
	if !HasConfiguredDeployment(ctx) {
		return "No API keys in " + credentials.PlatformSecretStoreName() + ". Run /config to paste a key or set up Ollama."
	}
	return "Check network access and stored keys (" + credentials.PlatformSecretStoreName() + "). Run hawk preflight or /config."
}

func autoRefreshCatalogEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(env.Getenv("HAWK_AUTO_REFRESH_CATALOG"))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func catalogRefreshAlways() bool {
	switch strings.ToLower(strings.TrimSpace(env.Getenv("HAWK_CATALOG_REFRESH_ALWAYS"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// ScheduleBackgroundCatalogRefresh silently refreshes the catalog when it is already stale,
// or after StaleAfter passes during a long interactive session.
func ScheduleBackgroundCatalogRefresh(ctx context.Context) {
	if !autoRefreshCatalogEnabled() {
		return
	}
	h := CatalogHealthReport(ctx)
	if h.Error != "" || h.Models == 0 {
		return
	}
	refresh := func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		_ = AutoRefreshCatalog(bgCtx, nil, false)
	}
	if h.Stale {
		go refresh()
		return
	}
	if h.StaleAfter.IsZero() {
		return
	}
	delay := time.Until(h.StaleAfter.UTC())
	if delay <= 0 {
		return
	}
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			refresh()
		}
	}()
}
