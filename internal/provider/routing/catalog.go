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
	cachedCatalog *catalog.CompiledCatalog
)

func eyrieCatalog() *catalog.CompiledCatalog {
	catalogOnce.Do(func() {
		compiled, err := catalog.LoadCatalog(context.Background(), catalog.LoadCatalogOptions{
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

func fromEyrie(model catalog.Model, offering catalog.ModelOffering) ModelInfo {
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
	if compiled := eyrieCatalog(); compiled != nil {
		if canonical, ok := compiled.CanonicalModelForAliasOrID(name); ok {
			model := compiled.ModelsByID[canonical]
			offering := firstOffering(compiled, canonical, "")
			return fromEyrie(model, offering), true
		}
	}
	return ModelInfo{}, false
}

// ByProvider returns all models for a given provider from eyrie's catalog.
func ByProvider(provider string) []ModelInfo {
	provider = catalog.CanonicalProviderID(provider)
	compiled := eyrieCatalog()
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
	return catalog.ProviderDefaultModel(eyrieCatalog(), provider, "")
}

// AllProviders returns all canonical model owner providers from eyrie's catalog.
func AllProviders() []string {
	out := catalog.AllModelProviders(eyrieCatalog())
	sort.Strings(out)
	return out
}

func firstOffering(compiled *catalog.CompiledCatalog, canonicalModelID, deploymentID string) catalog.ModelOffering {
	offerings := compiled.OfferingsByCanonicalModel[canonicalModelID]
	if len(offerings) == 0 {
		return catalog.ModelOffering{}
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
