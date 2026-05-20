package config

import (
	"context"
	"fmt"
	"time"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/setup"
)

// ApplyEyrieCredentials discovers the catalog and writes provider.json (routing only on disk).
func ApplyEyrieCredentials(ctx context.Context) (*setup.ApplyCredentialsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	PrepareCredentialDiscovery(ctx)
	result, err := setup.ApplyCredentials(ctx, eyriecfg.DiscoveryCredentials(ctx))
	if err != nil {
		return nil, err
	}
	_ = SaveProjectOrGlobalDeploymentRouting(true)
	return result, nil
}

// FormatApplyCredentialsSummary is a short status line for the TUI after /config saves keys.
func FormatApplyCredentialsSummary(result *setup.ApplyCredentialsResult) string {
	if result == nil || result.Catalog == nil || result.Catalog.Compiled == nil {
		return "Eyrie credentials applied"
	}
	nModels := len(result.Catalog.Compiled.ModelsByID)
	nDeps := 0
	if result.ProviderConfig != nil {
		nDeps = len(result.ProviderConfig.Deployments)
	}
	return fmt.Sprintf("Eyrie: %d models, %d deployments configured, routing updated → %s",
		nModels, nDeps, result.ProviderConfigPath)
}
