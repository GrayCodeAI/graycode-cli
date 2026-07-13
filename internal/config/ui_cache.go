package config

import (
	"context"
	"sort"
	"sync"
	"time"
)

var uiCacheMu sync.RWMutex

var (
	credConfigured map[string]bool
	credHasAny     bool
	credValid      bool
	credSnapMu     sync.Mutex
	credSnapAt     time.Time
	credSnapStore  map[string]gatewayCredentialState
)

const gatewayCredSnapshotTTL = 5 * time.Second

type gatewayCredentialState struct {
	credential bool
	deployment bool
}

// InvalidateConfigUICache drops in-memory catalog and credential snapshots (call after refresh/key changes).
func InvalidateConfigUICache() {
	uiCacheMu.Lock()
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
	stored := gatewayCredentialSnapshot(ctx)
	gateways := AllSetupGateways()
	configured := make(map[string]bool, len(gateways))
	hasAny := false
	for _, p := range gateways {
		state := stored[p]
		if state.credential {
			configured[p] = true
		}
		if state.deployment {
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
func gatewayCredentialSnapshot(ctx context.Context) map[string]gatewayCredentialState {
	credSnapMu.Lock()
	if credSnapStore != nil && time.Since(credSnapAt) < gatewayCredSnapshotTTL {
		out := credSnapStore
		credSnapMu.Unlock()
		return out
	}
	credSnapMu.Unlock()

	out := make(map[string]gatewayCredentialState)
	engine, err := newEyrieEngine()
	if err == nil {
		for _, gateway := range engine.Gateways(ctx) {
			out[gateway.ID] = gatewayCredentialState{
				credential: gateway.CredentialConfigured,
				deployment: gateway.DeploymentConfigured,
			}
		}
	}
	credSnapMu.Lock()
	credSnapStore = out
	credSnapAt = time.Now()
	credSnapMu.Unlock()
	return out
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
