package cmd

import (
	"context"
	"os"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

var (
	refreshCatalogFlag     bool
	skipCatalogRefreshFlag bool
)

func ensureCatalogBeforeAgent(ctx context.Context, strict bool) error {
	opts := hawkconfig.CatalogStartupOptions{
		ForceRefresh:    refreshCatalogFlag,
		SkipAutoRefresh: skipCatalogRefreshFlag,
		VerboseOutput:   refreshCatalogFlag,
	}
	if strict {
		return hawkconfig.PrepareCatalogForSession(ctx, os.Stderr, opts)
	}
	hawkconfig.StartupCatalogPrefetch(ctx)
	return nil
}

func startBackgroundCatalogRefresh(ctx context.Context) {
	if skipCatalogRefreshFlag {
		return
	}
	hawkconfig.ScheduleBackgroundCatalogRefresh(ctx)
}
