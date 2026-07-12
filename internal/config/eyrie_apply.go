package config

import (
	"context"
	"time"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/runtime"
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

// RefreshGatewayCatalog fetches live models for one gateway and updates the cache.
func RefreshGatewayCatalog(ctx context.Context, providerID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return runtime.RefreshGatewayCatalog(ctx, providerID)
}

func FormatApplyCredentialsSummary(result *setup.ApplyCredentialsResult) string {
	return runtime.FormatApplyCredentialsSummary(result)
}
