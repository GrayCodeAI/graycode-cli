// Package routing provides Hawk-owned task routing and health policy. Model
// discovery, pricing, provider ownership, and catalog policy are delegated to
// Eyrie's host-neutral engine facade.
package routing

import (
	"context"
	"sort"
	"sync"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
)

// ModelInfo is Hawk's product-facing view of Eyrie model metadata.
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
	modelEngineOnce sync.Once
	modelEngine     *eyrieengine.Engine
)

func eyrieModelEngine() *eyrieengine.Engine {
	modelEngineOnce.Do(func() {
		modelEngine, _ = eyrieengine.New(eyrieengine.Options{})
	})
	return modelEngine
}

func fromEngineModel(model eyrieengine.Model) ModelInfo {
	return ModelInfo{
		Name: model.ID, Provider: model.ProviderID,
		ContextSize: model.ContextWindow,
		InputPrice:  model.InputPricePer1M, OutputPrice: model.OutputPricePer1M,
		Description: model.Description,
	}
}

// Find looks up a model by id or alias through Eyrie.
func Find(name string) (ModelInfo, bool) {
	engine := eyrieModelEngine()
	if engine == nil {
		return ModelInfo{}, false
	}
	model, ok, err := engine.ModelInfo(context.Background(), name)
	if err != nil || !ok {
		return ModelInfo{}, false
	}
	return fromEngineModel(model), true
}

// ByProvider returns all models served by a provider/gateway.
func ByProvider(provider string) []ModelInfo {
	engine := eyrieModelEngine()
	if engine == nil {
		return nil
	}
	models, err := engine.ListModels(context.Background(), provider, false)
	if err != nil {
		return nil
	}
	out := make([]ModelInfo, 0, len(models))
	for _, model := range models {
		out = append(out, fromEngineModel(model))
	}
	return out
}

// Recommended returns the default catalog model for a provider.
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

func DefaultModel(provider string) string {
	if engine := eyrieModelEngine(); engine != nil {
		return engine.DefaultModel(context.Background(), provider, "")
	}
	return ""
}

func AllProviders() []string {
	engine := eyrieModelEngine()
	if engine == nil {
		return nil
	}
	providers, err := engine.ModelProviders(context.Background())
	if err != nil {
		return nil
	}
	sort.Strings(providers)
	return providers
}

func canonicalProvider(provider string) string {
	return eyrieengine.NormalizeProviderID(provider)
}
