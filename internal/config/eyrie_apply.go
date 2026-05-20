package config

import (
	"context"
	"fmt"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog"
	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/setup"
)

// ApplyEyrieCredentialsForProvider refreshes live models for one provider after /config saves a key.
func ApplyEyrieCredentialsForProvider(ctx context.Context, providerID string) (*setup.ApplyCredentialsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	PrepareCredentialDiscovery(ctx)
	result, err := setup.ApplyCredentialsForProvider(ctx, providerID, eyriecfg.DiscoveryCredentials(ctx))
	if err != nil {
		return nil, err
	}
	_ = SaveProjectOrGlobalDeploymentRouting(true)
	return result, nil
}

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

// RefreshGatewayCatalog fetches live models for one gateway and updates the cache.
func RefreshGatewayCatalog(ctx context.Context, providerID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	PrepareCredentialDiscovery(ctx)
	result, err := setup.DiscoverProviderCatalog(ctx, providerID, eyriecfg.DiscoveryCredentials(ctx))
	if err != nil {
		return "", err
	}
	n := 0
	if result.Compiled != nil {
		n = len(catalog.ModelEntriesForProvider(result.Compiled, providerID))
	}
	return fmt.Sprintf("Refreshed %s (%d models)", providerID, n), nil
}

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
