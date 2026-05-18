// Package model provides model routing and health checking.
// Model discovery, pricing, and catalog data are delegated to eyrie.
// Hawk does NOT carry a hardcoded model catalog.
package routing

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/GrayCodeAI/eyrie/catalog"
)

// ModelInfo describes a known LLM model (hawk's internal representation).
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
	catalogMu sync.RWMutex
	dynamic   []ModelInfo // runtime-registered models (custom providers)
)

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

func eyrieCatalogV1() *catalog.CompiledCatalogV1 {
	home, _ := os.UserHomeDir()
	compiled, err := catalog.LoadCatalogV1(context.Background(), catalog.LoadCatalogV1Options{
		CachePath: filepath.Join(home, ".eyrie", "model_catalog.json"),
	})
	if err != nil {
		return nil
	}
	return compiled
}

// RegisterDynamic adds a model entry at runtime (custom providers).
func RegisterDynamic(info ModelInfo) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	dynamic = append(dynamic, info)
}

// Find looks up a model by name across eyrie's catalog and dynamic entries.
func Find(name string) (ModelInfo, bool) {
	if compiled := eyrieCatalogV1(); compiled != nil {
		if canonical, ok := compiled.CanonicalModelForAliasOrID(name); ok {
			model := compiled.ModelsByID[canonical]
			offering := firstOffering(compiled, canonical, "")
			return fromEyrieV1(model, offering), true
		}
	}
	// Check dynamic entries
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	for _, m := range dynamic {
		if m.Name == name {
			return m, true
		}
	}
	return ModelInfo{}, false
}

// ByProvider returns all models for a given provider from eyrie's catalog.
func ByProvider(provider string) []ModelInfo {
	provider = canonicalProvider(provider)
	compiled := eyrieCatalogV1()
	out := []ModelInfo{}
	if compiled != nil {
		modelIDs := make([]string, 0, len(compiled.ModelsByID))
		for id, model := range compiled.ModelsByID {
			if canonicalProvider(model.ProviderID) == provider {
				modelIDs = append(modelIDs, id)
			}
		}
		sort.Strings(modelIDs)
		for _, id := range modelIDs {
			out = append(out, fromEyrieV1(compiled.ModelsByID[id], firstOffering(compiled, id, "")))
		}
	}
	// Append dynamic entries for this provider
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	for _, m := range dynamic {
		if m.Provider == provider {
			out = append(out, m)
		}
	}
	return out
}

// Recommended returns the first JSON-catalog model for a provider.
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

// DefaultModel returns the first catalog model for a provider via Eyrie's JSON catalog.
func DefaultModel(provider string) string {
	provider = canonicalProvider(provider)
	if compiled := eyrieCatalogV1(); compiled != nil {
		for _, model := range ByProvider(provider) {
			return model.Name
		}
	}
	return ""
}

// AllProviders returns all canonical model owner providers from eyrie's catalog.
func AllProviders() []string {
	seen := map[string]bool{}
	var out []string
	if compiled := eyrieCatalogV1(); compiled != nil {
		for _, model := range compiled.ModelsByID {
			provider := canonicalProvider(model.ProviderID)
			if provider != "" && !seen[provider] {
				seen[provider] = true
				out = append(out, provider)
			}
		}
	}
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	for _, m := range dynamic {
		if !seen[m.Provider] {
			seen[m.Provider] = true
			out = append(out, m.Provider)
		}
	}
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
	switch provider {
	case "gemini":
		return "google"
	case "grok":
		return "xai"
	case "zai":
		return "z-ai"
	default:
		return provider
	}
}
