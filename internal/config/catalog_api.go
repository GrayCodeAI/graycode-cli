package config

import (
	"context"
	"sort"
	"strings"

	llm "github.com/GrayCodeAI/eagle/llm"
	gw "github.com/GrayCodeAI/hawk/internal/provider/gateway"
)

type GatewayStatus struct {
	ID                      string
	DisplayName             string
	HasStoredCredential     bool
	HasConfiguredDeployment bool
	ModelCount              int
	Active                  bool
	RegionLabel             string
	RegionRequired          bool
	RegionOptions           []llm.GatewayRegionOption
	DNSHost                 string
}

// IsCatalogCacheRequired reports whether an Eyrie operation failed because
// the local model catalog has not been created yet.
func IsCatalogCacheRequired(err error) bool {
	return gw.IsCatalogCacheRequired(err)
}

// RegisteredProviderCount returns Eyrie's canonical first-class provider count.
func RegisteredProviderCount() int {
	return gw.RegisteredProviderCount()
}

func AllCatalogProviders() []string {
	engine, err := newEyrieEngine()
	if err != nil {
		return nil
	}
	providers, _ := engine.ModelProviders(context.Background())
	seen := map[string]bool{}
	for _, provider := range providers {
		if provider = gw.NormalizeProviderID(provider); provider != "" {
			seen[provider] = true
		}
	}
	for _, gateway := range engine.GatewayDefinitions() {
		seen[gateway.ID] = true
	}
	providers = providers[:0]
	for provider := range seen {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return providers
}

func AllSetupGateways() []string {
	engine, err := newEyrieEngine()
	if err != nil {
		return nil
	}
	gateways := engine.GatewayDefinitions()
	out := make([]string, 0, len(gateways))
	for _, gateway := range gateways {
		out = append(out, gateway.ID)
	}
	return out
}

func SetupGatewayCredentialEnv(providerID string) string {
	if gateway, ok := engineGateway(providerID); ok {
		return gateway.CredentialEnv
	}
	return ""
}

func IsSetupGateway(providerID string) bool {
	_, ok := engineGateway(providerID)
	return ok
}

func GatewayDisplayName(providerID string) string {
	if gateway, ok := engineGateway(providerID); ok {
		return gateway.DisplayName
	}
	return providerID
}

func DefaultThinkingDisabled(providerID string) bool {
	return gw.DefaultThinkingDisabled(providerID)
}

func ThinkingToggleSupported(providerID string) bool {
	return gw.ThinkingToggleSupported(providerID)
}

func GatewayDNSHost(providerID string) string {
	if gateway, ok := engineGateway(providerID); ok {
		return gateway.DNSHost
	}
	return ""
}

func GatewayRegionOptions(providerID string) []llm.GatewayRegionOption {
	if gateway, ok := engineGateway(providerID); ok {
		return gateway.RegionOptions
	}
	return nil
}

func HasRegionOptions(providerID string) bool {
	return len(GatewayRegionOptions(providerID)) > 0
}

func GatewaySupportsLiveDiscovery(providerID string) bool {
	if gateway, ok := engineGateway(providerID); ok {
		return gateway.SupportsLiveDiscovery
	}
	return false
}

func ActiveGateway(ctx context.Context) string {
	engine, err := newEyrieEngine()
	if err != nil {
		return ""
	}
	selection := engine.ActiveSelection(ctx)
	providerID := gw.NormalizeProviderID(selection.Provider)
	for _, gateway := range engine.GatewayDefinitions() {
		if gw.NormalizeProviderID(gateway.ID) == providerID {
			return gateway.ID
		}
	}
	return engine.GatewayForModel(ctx, selection.Model)
}

func GatewayStatuses(ctx context.Context, activeProvider, activeModel string) []GatewayStatus {
	engine, err := newEyrieEngine()
	if err != nil {
		return nil
	}
	gateways := engine.Gateways(ctx)
	out := make([]GatewayStatus, 0, len(gateways))
	for _, gateway := range gateways {
		active := gateway.Active
		if activeProvider != "" {
			active = gw.NormalizeProviderID(activeProvider) == gw.NormalizeProviderID(gateway.ID)
		} else if activeModel != "" {
			active = engine.GatewayForModel(ctx, activeModel) == gateway.ID
		}
		out = append(out, GatewayStatus{
			ID: gateway.ID, DisplayName: gateway.DisplayName,
			HasStoredCredential:     gateway.CredentialConfigured,
			HasConfiguredDeployment: gateway.DeploymentConfigured,
			ModelCount:              gateway.ModelCount, Active: active,
			RegionLabel: gateway.RegionLabel, RegionRequired: gateway.RegionRequired,
			RegionOptions: gateway.RegionOptions, DNSHost: gateway.DNSHost,
		})
	}
	return out
}

func GatewayForModel(modelID string) string {
	engine, err := newEyrieEngine()
	if err != nil {
		return ""
	}
	return engine.GatewayForModel(context.Background(), modelID)
}

func ShouldClearSelectionAfterCredentialRemove(ctx context.Context, removedProvider string) bool {
	engine, err := newEyrieEngine()
	if err != nil {
		return true
	}
	selection := engine.ActiveSelection(ctx)
	removedProvider = gw.NormalizeProviderID(removedProvider)
	return !engine.EffectiveSelection(ctx, gw.SelectionOptions{}).HasConfiguredDeployment ||
		gw.NormalizeProviderID(selection.Provider) == removedProvider ||
		gw.NormalizeProviderID(engine.GatewayForModel(ctx, selection.Model)) == removedProvider
}

func ClearActiveSelection(ctx context.Context) error {
	engine, err := newEyrieEngine()
	if err != nil {
		return err
	}
	return engine.ClearSelection(ctx)
}

func SyncSelectionWithCredentials(ctx context.Context) {
	engine, err := newEyrieEngine()
	if err != nil {
		return
	}
	active := engine.ActiveSelection(ctx)
	ready := map[string]bool{}
	hasAny := false
	for _, gateway := range engine.Gateways(ctx) {
		providerID := gw.NormalizeProviderID(gateway.ID)
		ready[providerID] = gateway.DeploymentConfigured
		hasAny = hasAny || gateway.DeploymentConfigured
	}
	activeGateway := gw.NormalizeProviderID(active.Provider)
	if activeGateway == "" && active.Model != "" {
		activeGateway = gw.NormalizeProviderID(engine.GatewayForModel(ctx, active.Model))
	}
	if !hasAny || (activeGateway != "" && !ready[activeGateway]) {
		_ = engine.ClearSelection(ctx)
	}
}

func DefaultModelForProvider(provider string) string {
	engine, err := newEyrieEngine()
	if err != nil {
		return ""
	}
	return engine.DefaultModel(context.Background(), provider, "")
}

func DefaultModelForProviderWithSettings(settings Settings, provider string) string {
	engine, err := NewEyrieEngineForSettings(settings)
	if err != nil {
		return ""
	}
	return engine.DefaultModel(context.Background(), provider, "")
}

func CachedModelCountForProvider(provider string) int {
	models, _ := ListEngineModels(context.Background(), provider, false)
	return len(models)
}

func ModelIDsForProvider(provider string) ([]string, error) {
	models, err := ListEngineModels(context.Background(), provider, false)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(models))
	for _, model := range models {
		if model.ID != "" {
			out = append(out, model.ID)
		}
	}
	return out, nil
}

func CheapestModelForProvider(provider, fallback string) string {
	engine, err := newEyrieEngine()
	if err != nil {
		return fallback
	}
	return engine.PreferredModel(context.Background(), provider, gw.ModelClassEconomical, fallback)
}

func ProviderOfModel(modelName string) string {
	engine, err := newEyrieEngine()
	if err != nil {
		return ""
	}
	return gw.NormalizeProviderID(engine.ProviderForModel(context.Background(), modelName))
}

func ProviderOfModelWithSettings(settings Settings, modelName string) string {
	engine, err := NewEyrieEngineForSettings(settings)
	if err != nil {
		return ""
	}
	return gw.NormalizeProviderID(engine.ProviderForModel(context.Background(), modelName))
}

func ExampleModelHints() (string, string) {
	engine, err := newEyrieEngine()
	if err != nil {
		return "an Anthropic model", "an OpenAI model"
	}
	return engine.DefaultModel(context.Background(), "anthropic", "an Anthropic model"),
		engine.DefaultModel(context.Background(), "openai", "an OpenAI model")
}

func AllCanonicalModelIDs() []string {
	engine, err := newEyrieEngine()
	if err != nil {
		return nil
	}
	snapshot, err := engine.Catalog(context.Background())
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(snapshot.Models))
	for _, model := range snapshot.Models {
		out = append(out, model.ID)
	}
	sort.Strings(out)
	return out
}

func ProviderIDForDeployment(deploymentID string) string {
	engine, err := newEyrieEngine()
	if err != nil {
		return ""
	}
	for _, gateway := range engine.GatewayDefinitions() {
		if gateway.DeploymentID == strings.TrimSpace(deploymentID) {
			return gateway.ID
		}
	}
	return ""
}

func PrimaryAPIKeyEnvForDeployment(deploymentID string) string {
	engine, err := newEyrieEngine()
	if err != nil {
		return ""
	}
	for _, gateway := range engine.GatewayDefinitions() {
		if gateway.DeploymentID == strings.TrimSpace(deploymentID) {
			return gateway.CredentialEnv
		}
	}
	return ""
}

func engineGateway(providerID string) (gw.GatewayDefs, bool) {
	engine, err := newEyrieEngine()
	if err != nil {
		return gw.GatewayDefs{}, false
	}
	providerID = gw.NormalizeProviderID(providerID)
	for _, gateway := range engine.GatewayDefinitions() {
		if gw.NormalizeProviderID(gateway.ID) == providerID {
			return gateway, true
		}
	}
	return gw.GatewayDefs{}, false
}
