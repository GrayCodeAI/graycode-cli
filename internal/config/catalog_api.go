package config

import (
	"context"
	"sort"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
	"github.com/GrayCodeAI/eyrie/runtime"
)

// CompiledCatalogV1 loads the eyrie catalog from cache or bootstrap wiring (no network).
func CompiledCatalogV1() *catalog.CompiledCatalogV1 {
	return compiledCatalogOrBootstrap()
}

func compiledCatalogOrBootstrap() *catalog.CompiledCatalogV1 {
	if compiled, ok := cachedCompiledCatalog(); ok && compiled != nil {
		return compiled
	}
	compiled, err := loadEyrieCatalogV1(context.Background(), false)
	if err == nil && compiled != nil {
		storeCompiledCatalog(compiled)
		return compiled
	}
	bootstrap := catalog.BootstrapCatalogV1()
	compiled, err = catalog.CompileCatalogV1(&bootstrap)
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
		p := catalogProviderID(id)
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
	specs := registry.CredentialRegistry()
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.ProviderID
	}
	return out
}

// setupGatewayRegistryID maps catalog/engine aliases to credential registry gateway ids.
// Most registry IDs use underscores for multi-word plans (e.g. xiaomi_mimo_token_plan).
// Z.AI uses underscore naming for uniformity with Xiaomi/MiniMax plan splits: zai_payg and zai_coding (no legacy aliases).
func setupGatewayRegistryID(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch p {
	case "google":
		return "gemini"
	case "xai":
		return "grok"
	case "zai_payg":
		return "zai_payg"
	case "zai_coding":
		return "zai_coding"
	case "xiaomi_mimo", "xiaomi-mimo":
		return "xiaomi_mimo_payg"
	case "xiaomi_mimo_token_plan", "xiaomi-mimo-token-plan":
		return "xiaomi_mimo_token_plan"
	case "xiaomi_mimo_payg", "xiaomi-mimo-payg":
		return "xiaomi_mimo_payg"
	default:
		return p
	}
}

// SetupGatewayCredentialEnv returns the registry env var for a setup gateway (e.g. XIAOMI_MIMO_PAYG_API_KEY).
func SetupGatewayCredentialEnv(providerID string) string {
	spec, ok := registry.DefaultRegistry.Get(setupGatewayRegistryID(providerID))
	if !ok || !spec.RequiresKey {
		return ""
	}
	return strings.TrimSpace(spec.CredentialEnv)
}

// IsSetupGateway reports whether id is a registered setup gateway.
func IsSetupGateway(providerID string) bool {
	return catalog.IsSetupGateway(setupGatewayRegistryID(providerID))
}

func GatewayDisplayName(gatewayID string) string {
	gatewayID = setupGatewayRegistryID(gatewayID)
	if name := registry.DisplayName(gatewayID); name != gatewayID {
		return name
	}
	return gatewayID
}

// ActiveGateway returns the user's setup gateway (never an aggregator owner slug like moonshotai).
func ActiveGateway(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	if p := catalogProviderID(ActiveProvider(ctx)); catalog.IsSetupGateway(p) {
		return setupGatewayRegistryID(p)
	}
	if m := strings.TrimSpace(ActiveModel(ctx)); m != "" {
		if gw := GatewayForModel(m); gw != "" {
			return setupGatewayRegistryID(gw)
		}
	}
	return ""
}

// GatewayForModel resolves the setup gateway for a model id.
func GatewayForModel(modelID string) string {
	return catalog.GatewayForModel(CompiledCatalogV1(), modelID)
}

// ShouldClearSelectionAfterCredentialRemove reports whether provider/model should reset.
func ShouldClearSelectionAfterCredentialRemove(ctx context.Context, removedProvider string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	removedProvider = catalogProviderID(removedProvider)
	if !HasConfiguredDeployment(ctx) {
		return true
	}
	if gw := ActiveGateway(ctx); gw == removedProvider {
		return true
	}
	if m := strings.TrimSpace(ActiveModel(ctx)); m != "" && GatewayForModel(m) == removedProvider {
		return true
	}
	return false
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
	if !HasConfiguredDeployment(ctx) {
		if HasSelectedModel() || strings.TrimSpace(ActiveProvider(ctx)) != "" {
			_ = ClearActiveSelection(ctx)
		}
		return
	}
	gw := ActiveGateway(ctx)
	if gw == "" {
		return
	}
	if !credentialConfiguredForGateway(ctx, gw) {
		_ = ClearActiveSelection(ctx)
	}
}

func credentialConfiguredForGateway(ctx context.Context, gateway string) bool {
	ensureCredSnapshot(ctx)
	uiCacheMu.RLock()
	defer uiCacheMu.RUnlock()
	if !credValid {
		return false
	}
	gateway = setupGatewayRegistryID(gateway)
	return credConfigured[gateway]
}

func DefaultModelForProvider(provider string) string {
	compiled := CompiledCatalogV1()
	if compiled != nil {
		if id := catalog.FirstModelForProvider(compiled, provider); id != "" {
			return id
		}
	}
	// All providers are fully dynamic — try live API if credentials are available.
	if APIKeyForProvider(provider) != "" {
		models, err := runtime.ListModels(context.Background(), runtime.ListModelsOpts{
			ProviderID: provider,
			Source:     runtime.ListSourceAuto,
		})
		if err == nil && len(models) > 0 {
			return models[0].ID
		}
	}
	return ""
}

// CachedModelCountForProvider returns model count from the on-disk catalog only (no network).
func CachedModelCountForProvider(provider string) int {
	provider = setupGatewayRegistryID(provider)
	if provider == "" {
		return 0
	}
	compiled := CompiledCatalogV1()
	if compiled == nil {
		return 0
	}
	return len(catalog.ModelEntriesForProvider(compiled, provider))
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
			return catalogProviderID(model.ProviderID)
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
	return catalogProviderID(dep.ProviderID)
}

// PrimaryAPIKeyEnvForDeployment returns the env var name for a deployment's API key.
func PrimaryAPIKeyEnvForDeployment(deploymentID string) string {
	compiled := compiledCatalogOrBootstrap()
	if compiled == nil {
		return ""
	}
	return catalog.PrimaryAPIKeyEnvForDeployment(compiled, deploymentID)
}

// ConfigProviderList returns provider names for the /config UI from catalog + custom providers.
func ConfigProviderList(custom []CustomProviderConfig) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range AllCatalogProviders() {
		engine := NormalizeProviderForEngine(p)
		if engine == "" || seen[engine] {
			continue
		}
		seen[engine] = true
		out = append(out, engine)
	}
	for _, cp := range custom {
		name := strings.TrimSpace(cp.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
