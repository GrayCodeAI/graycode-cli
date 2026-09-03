// Package routing provides Graycode-owned task routing and health policy. Model
// discovery, pricing, provider ownership, and catalog policy are delegated to
// Eyrie's engine facade through the gateway package (the single Eyrie boundary).
package routing

import (
	"context"
	"sort"

	"github.com/GrayCodeAI/graycode-cli/internal/provider/gateway"
)

// ModelInfo is Graycode's product-facing view of Eyrie model metadata (the gateway
// layer owns the Eyrie conversation that produces it).
type ModelInfo = gateway.ModelInfo

// Find looks up a model by id or alias through Eyrie.
func Find(name string) (ModelInfo, bool) {
	return gateway.ModelInfoLookup(context.Background(), name)
}

// ByProvider returns all models served by a provider/gateway.
func ByProvider(provider string) []ModelInfo {
	infos, err := gateway.ModelsByProvider(context.Background(), provider)
	if err != nil {
		return nil
	}
	return infos
}

// Recommended returns the default catalog model for a provider.
func Recommended(provider string) (ModelInfo, bool) {
	return gateway.RecommendedModel(context.Background(), provider)
}

// DefaultModel returns the catalog default model name for a provider.
func DefaultModel(provider string) string {
	return gateway.DefaultModel(context.Background(), provider)
}

// AllProviders returns the distinct set of providers/gateways in the catalog.
func AllProviders() []string {
	providers, err := gateway.AllProviders(context.Background())
	if err != nil {
		return nil
	}
	sort.Strings(providers)
	return providers
}

// ProviderOfModel resolves which provider owns a model name.
func ProviderOfModel(modelName string) string {
	return gateway.ProviderForModel(context.Background(), modelName)
}

func canonicalProvider(provider string) string {
	return gateway.NormalizeProviderID(provider)
}
