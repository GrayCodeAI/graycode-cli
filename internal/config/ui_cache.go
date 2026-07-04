package config

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/credentials"
)

var uiCacheMu sync.RWMutex

var (
	cachedCompiled *catalog.CompiledCatalogV1
	credConfigured map[string]bool
	credHasAny     bool
	credValid      bool
	credSnapMu     sync.Mutex
	credSnapAt     time.Time
	credSnapStore  map[string]string
)

const gatewayCredSnapshotTTL = 5 * time.Second

// InvalidateConfigUICache drops in-memory catalog and credential snapshots (call after refresh/key changes).
func InvalidateConfigUICache() {
	uiCacheMu.Lock()
	cachedCompiled = nil
	credValid = false
	credConfigured = nil
	uiCacheMu.Unlock()
	credSnapMu.Lock()
	credSnapAt = time.Time{}
	credSnapStore = nil
	credSnapMu.Unlock()
	InvalidateCatalogHealthCache()
}

// RefreshConfigCredSnapshot re-reads keychain status for setup gateways (call when opening /config).
func RefreshConfigCredSnapshot(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	PrepareCredentialDiscovery(ctx)
	stored := gatewayCredentialSnapshot(ctx)
	gateways := AllSetupGateways()
	configured := make(map[string]bool, len(gateways))
	hasAny := false
	for _, p := range gateways {
		if gatewayConfiguredFromStored(p, stored) {
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

// gatewayCredentialSnapshot loads only setup-gateway env keys (not the full discovery list).
func gatewayCredentialSnapshot(ctx context.Context) map[string]string {
	credSnapMu.Lock()
	if credSnapStore != nil && time.Since(credSnapAt) < gatewayCredSnapshotTTL {
		out := credSnapStore
		credSnapMu.Unlock()
		return out
	}
	credSnapMu.Unlock()

	out := make(map[string]string)
	for _, gw := range AllSetupGateways() {
		for _, env := range credentialEnvKeysForTarget(gw) {
			if _, ok := out[env]; ok {
				continue
			}
			if secret := credentials.LookupSecret(ctx, env); secret != "" {
				out[env] = secret
			}
		}
	}
	credSnapMu.Lock()
	credSnapStore = out
	credSnapAt = time.Now()
	credSnapMu.Unlock()
	return out
}

func gatewayConfiguredFromStored(providerID string, stored map[string]string) bool {
	for _, env := range credentialEnvKeysForTarget(providerID) {
		if strings.TrimSpace(stored[env]) != "" {
			return true
		}
	}
	return false
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

func CredentialSnapshotReady() bool {
	uiCacheMu.RLock()
	defer uiCacheMu.RUnlock()
	return credValid
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
