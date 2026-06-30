// Package routing provides model routing and health checking.
// Model discovery, pricing, and catalog data are delegated to eyrie.
// Hawk does NOT carry a hardcoded model catalog.
package routing

import (
	"context"
	"sort"
	"sync"

	"github.com/GrayCodeAI/eyrie/catalog"
)

// ModelInfo describes a known LLM model (view over eyrie catalog entries).
type ModelInfo struct {
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	ContextSize int     `json:"context_size"`
	InputPrice  float64 `json:"input_price_per_million"`
	OutputPrice float64 `json:"output_price_per_million"`
	Description string  `json:"description,omitempty"`
	Recommended bool    `json:"recommended,omitempty"`
}

var (
	catalogOnce   sync.Once
	cachedCatalog *catalog.CompiledCatalogV1
)

func eyrieCatalogV1() *catalog.CompiledCatalogV1 {
	catalogOnce.Do(func() {
		compiled, err := catalog.LoadCatalogV1(context.Background(), catalog.LoadCatalogV1Options{
			CachePath:    catalog.DefaultCachePath(),
			RequireCache: false,
		})
		if err != nil {
			return
		}
		cachedCatalog = compiled
	})
	return cachedCatalog
}

func fromEyrieV1(model catalog.ModelV1, offering catalog.ModelOfferingV1) ModelInfo {
	inPrice, outPrice := 0.0, 0.0
	if offering.Pricing.RatesPer1M != nil {
		inPrice = offering.Pricing.RatesPer1M["input_tokens"]
		outPrice = offering.Pricing.RatesPer1M["output_tokens"]
	}
	return ModelInfo{
		Name:        model.ID,
		Provider:    model.ProviderID,
		ContextSize: model.ContextWindow,
		InputPrice:  inPrice,
		OutputPrice: outPrice,
		Description: model.Name,
	}
}

func fromEyrieEntry(entry catalog.ModelCatalogEntry, provider string) ModelInfo {
	return ModelInfo{
		Name:        entry.ID,
		Provider:    provider,
		ContextSize: entry.ContextWindow,
		InputPrice:  entry.InputPricePer1M,
		OutputPrice: entry.OutputPricePer1M,
		Description: entry.DisplayName,
	}
}

// Find looks up a model by name via eyrie's JSON catalog.
func Find(name string) (ModelInfo, bool) {
	if compiled := eyrieCatalogV1(); compiled != nil {
		if canonical, ok := compiled.CanonicalModelForAliasOrID(name); ok {
			model := compiled.ModelsByID[canonical]
			offering := firstOffering(compiled, canonical, "")
			return fromEyrieV1(model, offering), true
		}
	}
	return ModelInfo{}, false
}

// ByProvider returns all models for a given provider from eyrie's catalog.
func ByProvider(provider string) []ModelInfo {
	provider = catalog.CanonicalProviderID(provider)
	compiled := eyrieCatalogV1()
	out := []ModelInfo{}
	if compiled != nil {
		entries := catalog.ModelEntriesForProvider(compiled, provider)
		for _, entry := range entries {
			out = append(out, fromEyrieEntry(entry, provider))
		}
	}
	return out
}

// Recommended returns the first catalog model for a provider.
func Recommended(provider string) (ModelInfo, bool) {
	name := DefaultModel(provider)
	if name == "" {
		return ModelInfo{}, false
	}
	info, ok := Find(name)
	if ok {
		info.Recommended = true
	}
	return info, ok
}

// DefaultModel returns the first catalog model for a provider via eyrie JSON.
func DefaultModel(provider string) string {
	return catalog.ProviderDefaultModelV1(eyrieCatalogV1(), provider, "")
}

// AllProviders returns all canonical model owner providers from eyrie's catalog.
func AllProviders() []string {
	out := catalog.AllModelProvidersV1(eyrieCatalogV1())
	sort.Strings(out)
	return out
}

func firstOffering(compiled *catalog.CompiledCatalogV1, canonicalModelID, deploymentID string) catalog.ModelOfferingV1 {
	offerings := compiled.OfferingsByCanonicalModel[canonicalModelID]
	if len(offerings) == 0 {
		return catalog.ModelOfferingV1{}
	}
	if deploymentID != "" {
		for _, offering := range offerings {
			if offering.DeploymentID == deploymentID {
				return offering
			}
		}
	}
	sort.SliceStable(offerings, func(i, j int) bool {
		return offerings[i].DeploymentID < offerings[j].DeploymentID
	})
	return offerings[0]
}

func canonicalProvider(provider string) string {
	return catalog.CanonicalProviderID(provider)
}
