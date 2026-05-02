// Package model provides model routing and health checking.
// Model discovery, pricing, and catalog data are delegated to eyrie.
// Hawk does NOT carry a hardcoded model catalog.
package model

import (
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

// fromEyrie converts an eyrie catalog entry to hawk's ModelInfo.
func fromEyrie(provider string, e catalog.ModelCatalogEntry) ModelInfo {
	desc := e.Description
	if desc == "" {
		desc = e.DisplayName
	}
	return ModelInfo{
		Name:        e.ID,
		Provider:    provider,
		ContextSize: e.ContextWindow,
		InputPrice:  e.InputPricePer1M,
		OutputPrice: e.OutputPricePer1M,
		Description: desc,
	}
}

// eyrieCatalog returns the current eyrie catalog.
func eyrieCatalog() catalog.ModelCatalog {
	return catalog.DefaultModelCatalog()
}

// RegisterDynamic adds a model entry at runtime (custom providers).
func RegisterDynamic(info ModelInfo) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	dynamic = append(dynamic, info)
}

// Find looks up a model by name across eyrie's catalog and dynamic entries.
func Find(name string) (ModelInfo, bool) {
	// Check eyrie catalog first
	cat := eyrieCatalog()
	for provider, models := range cat.Providers {
		for _, m := range models {
			if m.ID == name {
				return fromEyrie(provider, m), true
			}
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
	cat := eyrieCatalog()
	entries := catalog.ModelsForProvider(&cat, provider)
	out := make([]ModelInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, fromEyrie(provider, e))
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

// Recommended returns the recommended model for a provider.
// Delegates to eyrie's GetProviderDefaultModel.
func Recommended(provider string) (ModelInfo, bool) {
	name := catalog.GetProviderDefaultModel(provider, nil)
	if name == "" {
		return ModelInfo{}, false
	}
	info, ok := Find(name)
	if ok {
		info.Recommended = true
	}
	return info, ok
}

// DefaultModel returns the default model for a provider via eyrie.
func DefaultModel(provider string) string {
	name := catalog.GetProviderDefaultModel(provider, nil)
	if name != "" {
		return name
	}
	// Fallback for unknown providers
	return ""
}

// AllProviders returns all provider names from eyrie's catalog.
func AllProviders() []string {
	cat := eyrieCatalog()
	out := make([]string, 0, len(cat.Providers))
	for p := range cat.Providers {
		out = append(out, p)
	}
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	seen := make(map[string]bool, len(out))
	for _, p := range out {
		seen[p] = true
	}
	for _, m := range dynamic {
		if !seen[m.Provider] {
			seen[m.Provider] = true
			out = append(out, m.Provider)
		}
	}
	return out
}
