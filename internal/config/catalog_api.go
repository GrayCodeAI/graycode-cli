package config

import (
	"context"
	"sort"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/runtime"
)

type GatewayStatus = runtime.GatewayStatus

// CompiledCatalogV1 loads the eyrie catalog from cache or bootstrap wiring (no network).
func CompiledCatalogV1() *catalog.CompiledCatalog {
	return compiledCatalogOrBootstrap()
}

func compiledCatalogOrBootstrap() *catalog.CompiledCatalog {
	if compiled, ok := cachedCompiledCatalog(); ok && compiled != nil {
		return compiled
	}
	compiled, err := loadEyrieCatalogV1(context.Background(), false)
	if err == nil && compiled != nil {
		storeCompiledCatalog(compiled)
		return compiled
	}
	bootstrap := catalog.BootstrapCatalog()
	compiled, err = catalog.CompileCatalog(&bootstrap)
	if err != nil {
		return nil
	}
	storeCompiledCatalog(compiled)
	return compiled
}

// AllCatalogProviders returns provider IDs from eyrie (providers + deployments, not hawk constants).
func AllCatalogProviders() []string {
	compiled := compiledCatalogOrBootstrap()
	if compiled == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, id := range catalog.ProviderIDsFromCompiled(compiled) {
		p := runtime.CatalogProviderID(id)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// AllSetupGateways returns gateway IDs where users paste API keys (eyrie registry only).
// Aggregator owner slugs from OpenRouter/CanopyWave catalogs (ai21, alibaba, …) are excluded.
func AllSetupGateways() []string {
	return runtime.SetupGateways()
}

// SetupGatewayCredentialEnv returns the registry env var for a setup gateway (e.g. XIAOMI_MIMO_PAYG_API_KEY).
func SetupGatewayCredentialEnv(providerID string) string {
	return runtime.SetupGatewayCredentialEnv(providerID)
}

// IsSetupGateway reports whether id is a registered setup gateway.
func IsSetupGateway(providerID string) bool {
	return runtime.IsSetupGateway(providerID)
}

func GatewayDisplayName(gatewayID string) string {
	return runtime.GatewayDisplayName(gatewayID)
}

// ActiveGateway returns the user's setup gateway (never an aggregator owner slug like moonshotai).
func ActiveGateway(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	return runtime.ActiveGateway(ctx)
}

func GatewayStatuses(ctx context.Context, activeProvider, activeModel string) []GatewayStatus {
	if ctx == nil {
		ctx = context.Background()
	}
	ensureCredSnapshot(ctx)

	active := activeProvider
	if active == "" && activeModel != "" {
		active = GatewayForModel(activeModel)
	}
	if active == "" {
		active = ActiveGateway(ctx)
	}

	uiCacheMu.RLock()
	configured := credConfigured
	uiCacheMu.RUnlock()

	compiled := compiledCatalogOrBootstrap()
	gateways := runtime.SetupGateways()
	statuses := make([]GatewayStatus, 0, len(gateways))

	for _, providerID := range gateways {
		count := 0
		if compiled != nil {
			count = len(catalog.ModelEntriesForProvider(compiled, providerID))
		}

		hasKey := false
		if configured != nil {
			hasKey = configured[providerID]
		}

		statuses = append(statuses, GatewayStatus{
			ID:                      providerID,
			DisplayName:             runtime.GatewayDisplayName(providerID),
			HasStoredCredential:     hasKey,
			HasConfiguredDeployment: hasKey,
			ModelCount:              count,
			Active:                  providerID == active,
			RegionLabel:             runtime.GatewayRegionLabel(providerID),
			RegionRequired:          runtime.GatewayNeedsRegion(providerID),
		})
	}

	return statuses
}

// GatewayForModel resolves the setup gateway for a model id.
func GatewayForModel(modelID string) string {
	return catalog.GatewayForModel(CompiledCatalogV1(), modelID)
}

// ShouldClearSelectionAfterCredentialRemove reports whether provider/model should reset.
func ShouldClearSelectionAfterCredentialRemove(ctx context.Context, removedProvider string) bool {
	return runtime.ShouldClearSelectionAfterCredentialRemove(ctx, removedProvider)
}

// ClearActiveSelection removes persisted provider/model from provider.json.
func ClearActiveSelection(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return runtime.ClearActiveSelection(ctx)
}

// SyncSelectionWithCredentials clears stale provider/model when keys are missing.
func SyncSelectionWithCredentials(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime.SyncSelectionWithCredentials(ctx)
}

func DefaultModelForProvider(provider string) string {
	return runtime.DefaultModelForProvider(context.Background(), provider)
}

// CachedModelCountForProvider returns model count from the on-disk catalog only (no network).
func CachedModelCountForProvider(provider string) int {
	return runtime.CachedModelCountForProvider(context.Background(), provider)
}

func ModelIDsForProvider(provider string) ([]string, error) {
	entries, err := FetchModelsForProvider(provider)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.ID != "" {
			out = append(out, e.ID)
		}
	}
	return out, nil
}

// CheapestModelForProvider picks the lowest input-priced model from eyrie's catalog.
func CheapestModelForProvider(provider, fallback string) string {
	entries, err := FetchModelsForProvider(provider)
	if err != nil || len(entries) == 0 {
		return fallback
	}
	cheapest := entries[0]
	for _, e := range entries[1:] {
		if e.InputPricePer1M > 0 && (cheapest.InputPricePer1M == 0 || e.InputPricePer1M < cheapest.InputPricePer1M) {
			cheapest = e
		}
	}
	if cheapest.ID != "" {
		return cheapest.ID
	}
	return fallback
}

// ProviderOfModel resolves catalog provider for a canonical model ID or alias.
func ProviderOfModel(modelName string) string {
	compiled := CompiledCatalogV1()
	if compiled == nil {
		return ""
	}
	if canonical, ok := compiled.CanonicalModelForAliasOrID(modelName); ok {
		if model := compiled.ModelsByID[canonical]; model.ID != "" {
			return runtime.CatalogProviderID(model.ProviderID)
		}
	}
	return ""
}

// ExampleModelHints returns short example model aliases for user-facing error messages.
func ExampleModelHints() (anthropic, openai string) {
	compiled := CompiledCatalogV1()
	if compiled == nil {
		return "claude-sonnet-4-6", "gpt-4o"
	}
	if _, ok := compiled.CanonicalModelForAliasOrID("claude-sonnet-4-6"); ok {
		anthropic = "claude-sonnet-4-6"
	}
	if _, ok := compiled.CanonicalModelForAliasOrID("gpt-4o"); ok {
		openai = "gpt-4o"
	}
	if anthropic == "" || openai == "" {
		for _, id := range []string{"anthropic/claude-sonnet-4-6", "openai/gpt-4o"} {
			if _, ok := compiled.ModelsByID[id]; !ok {
				continue
			}
			if strings.HasPrefix(id, "anthropic/") && anthropic == "" {
				anthropic = strings.TrimPrefix(id, "anthropic/")
			}
			if strings.HasPrefix(id, "openai/") && openai == "" {
				openai = strings.TrimPrefix(id, "openai/")
			}
		}
	}
	if anthropic == "" || openai == "" {
		return "claude-sonnet-4-6", "gpt-4o"
	}
	return anthropic, openai
}

// AllCanonicalModelIDs returns sorted canonical model IDs from the eyrie catalog.
func AllCanonicalModelIDs() []string {
	compiled := compiledCatalogOrBootstrap()
	if compiled == nil {
		return nil
	}
	out := make([]string, 0, len(compiled.ModelsByID))
	for id := range compiled.ModelsByID {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// ProviderIDForDeployment returns the catalog provider id for a deployment (e.g. anthropic-direct → anthropic).
func ProviderIDForDeployment(deploymentID string) string {
	compiled := compiledCatalogOrBootstrap()
	if compiled == nil {
		return ""
	}
	dep, ok := compiled.DeploymentsByID[deploymentID]
	if !ok {
		return ""
	}
	return runtime.CatalogProviderID(dep.ProviderID)
}

// PrimaryAPIKeyEnvForDeployment returns the env var name for a deployment's API key.
func PrimaryAPIKeyEnvForDeployment(deploymentID string) string {
	compiled := compiledCatalogOrBootstrap()
	if compiled == nil {
		return ""
	}
	return catalog.PrimaryAPIKeyEnvForDeployment(compiled, deploymentID)
}
