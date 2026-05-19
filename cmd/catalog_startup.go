package cmd

import (
	"context"
	"os"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/onboarding"
)

var (
	refreshCatalogFlag    bool
	skipCatalogRefreshFlag bool
)

func ensureFirstRunSetup() error {
	if !onboarding.NeedsSetup() {
		return nil
	}
	onboarding.Welcome(version)
	return onboarding.RunSetup()
}

func ensureCatalogBeforeAgent(ctx context.Context, strict bool) error {
	_ = hawkconfig.MigrateProviderConfig()
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
