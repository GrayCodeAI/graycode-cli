package cmd

import (
	"context"
	"os"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
)

var (
	refreshCatalogFlag     bool
	skipCatalogRefreshFlag bool
)

func ensureCatalogBeforeAgent(ctx context.Context, strict bool) error {
	opts := graycodeconfig.CatalogStartupOptions{
		ForceRefresh:    refreshCatalogFlag,
		SkipAutoRefresh: skipCatalogRefreshFlag,
		VerboseOutput:   refreshCatalogFlag,
	}
	if strict {
		return graycodeconfig.PrepareCatalogForSession(ctx, os.Stderr, opts)
	}
	graycodeconfig.StartupCatalogPrefetch(ctx)
	return nil
}

func startBackgroundCatalogRefresh(ctx context.Context) {
	if skipCatalogRefreshFlag {
		return
	}
	graycodeconfig.ScheduleBackgroundCatalogRefresh(ctx)
}
