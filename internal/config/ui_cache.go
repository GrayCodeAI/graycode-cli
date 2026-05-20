package config

import (
	"context"
	"sort"
	"sync"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/credentials"
)

var uiCacheMu sync.RWMutex

var (
	cachedCompiled *catalog.CompiledCatalogV1
	credConfigured map[string]bool
	credHasAny     bool
	credValid      bool
)

// InvalidateConfigUICache drops in-memory catalog and credential snapshots (call after refresh/key changes).
func InvalidateConfigUICache() {
	uiCacheMu.Lock()
	cachedCompiled = nil
	credValid = false
	credConfigured = nil
	uiCacheMu.Unlock()
}

// RefreshConfigCredSnapshot re-reads keychain status for setup gateways (call when opening /config).
func RefreshConfigCredSnapshot(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	PrepareCredentialDiscovery(ctx)
	compiled := compiledCatalogOrBootstrap()
	gateways := AllSetupGateways()
	configured := make(map[string]bool, len(gateways))
	hasAny := false
	for _, p := range gateways {
		if credentialSetForGateway(ctx, compiled, p) {
			configured[p] = true
			hasAny = true
		}
	}
	uiCacheMu.Lock()
	credConfigured = configured
	credHasAny = hasAny
	credValid = true
	uiCacheMu.Unlock()
}

func ensureCredSnapshot(ctx context.Context) {
	uiCacheMu.RLock()
	valid := credValid
	uiCacheMu.RUnlock()
	if valid {
		return
	}
	RefreshConfigCredSnapshot(ctx)
}

func credentialSetForGateway(ctx context.Context, compiled *catalog.CompiledCatalogV1, provider string) bool {
	if compiled == nil {
		return false
	}
	provider = catalogProviderID(provider)
	envs := catalog.APIKeyEnvsForProvider(compiled, provider)
	if len(envs) == 0 {
		return false
	}
	for _, env := range envs {
		if credentials.HasSecret(ctx, env) {
			return true
		}
	}
	return false
}

// ConfiguredCredentialProviders returns setup gateways with a stored API key (cached for TUI).
func configuredCredentialProvidersCached(ctx context.Context) []string {
	ensureCredSnapshot(ctx)
	uiCacheMu.RLock()
	defer uiCacheMu.RUnlock()
	var out []string
	for p, set := range credConfigured {
		if set {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// HasConfiguredDeploymentCached is a fast cached check for the /config TUI only.
func HasConfiguredDeploymentCached(ctx context.Context) bool {
	return hasConfiguredDeploymentCached(ctx)
}

func hasConfiguredDeploymentCached(ctx context.Context) bool {
	ensureCredSnapshot(ctx)
	uiCacheMu.RLock()
	defer uiCacheMu.RUnlock()
	return credHasAny
}

func storeCompiledCatalog(compiled *catalog.CompiledCatalogV1) {
	uiCacheMu.Lock()
	cachedCompiled = compiled
	uiCacheMu.Unlock()
}

func cachedCompiledCatalog() (*catalog.CompiledCatalogV1, bool) {
	uiCacheMu.RLock()
	defer uiCacheMu.RUnlock()
	if cachedCompiled == nil {
		return nil, false
	}
	return cachedCompiled, true
}
