package eyrieclient

import (
	"context"

	"github.com/GrayCodeAI/eyrie/catalog"
	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/setup"
)

// CatalogCredentials loads API keys from the OS secret store.
func CatalogCredentials(ctx context.Context) catalog.Credentials {
	return eyriecfg.DiscoveryCredentials(ctx)
}

// DiscoverCatalog refreshes the eyrie remote catalog and live provider model lists.
func DiscoverCatalog(ctx context.Context) (*catalog.RefreshResult, error) {
	return setup.DiscoverModelCatalog(ctx, CatalogCredentials(ctx))
}

// DiscoverCatalogWithKeys refreshes the catalog using explicit env keys (name → value).
func DiscoverCatalogWithKeys(ctx context.Context, apiKeys map[string]string) (*catalog.RefreshResult, error) {
	return setup.DiscoverModelCatalog(ctx, catalog.Credentials{APIKeys: apiKeys})
}

// LoadCatalog loads the compiled catalog from ~/.eyrie/model_catalog.json (no network).
func LoadCatalog(ctx context.Context) (*catalog.CompiledCatalogV1, error) {
	return setup.LoadCompiledCatalog(ctx)
}

// DiscoveryEnvKeys returns env var names needed for catalog discovery (from compiled cache).
func DiscoveryEnvKeys(ctx context.Context) []string {
	compiled, err := LoadCatalog(ctx)
	if err != nil || compiled == nil {
		return nil
	}
	return catalog.DiscoveryEnvKeysFromCatalog(compiled)
}
