package eyrieclient

import (
	"context"

	"github.com/GrayCodeAI/eyrie/catalog"
	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/setup"
)

// CatalogCredentials collects API keys from the environment using eyrie's provider profiles.
// Hawk does not maintain its own list of env var names.
func CatalogCredentials() catalog.Credentials {
	return eyriecfg.DiscoveryCredentialsFromOS()
}

// DiscoverCatalog refreshes the eyrie remote catalog and live provider model lists using env API keys.
func DiscoverCatalog(ctx context.Context) (*catalog.RefreshResult, error) {
	return setup.DiscoverModelCatalog(ctx, CatalogCredentials())
}

// DiscoverCatalogWithKeys refreshes the catalog using explicit env keys (name → value).
func DiscoverCatalogWithKeys(ctx context.Context, apiKeys map[string]string) (*catalog.RefreshResult, error) {
	return setup.DiscoverModelCatalog(ctx, catalog.Credentials{APIKeys: apiKeys})
}

// LoadCatalog loads the compiled catalog from ~/.eyrie/model_catalog.json (no network).
func LoadCatalog(ctx context.Context) (*catalog.CompiledCatalogV1, error) {
	return setup.LoadCompiledCatalog(ctx)
}
