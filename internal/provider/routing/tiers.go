package routing

import (
	"sort"
	"strings"

	eycatalog "github.com/GrayCodeAI/eyrie/catalog"
)

// CostTier is a relative cost band for cascade routing (cheap / mid / expensive).
type CostTier = eycatalog.ModelCostTier

const (
	CostTierCheap     = eycatalog.CostTierCheap
	CostTierMid       = eycatalog.CostTierMid
	CostTierExpensive = eycatalog.CostTierExpensive
)

// CostTierOf resolves a model's cost tier from eyrie catalog policy.
func CostTierOf(modelName string) CostTier {
	return eycatalog.ModelCostTierOf(eyrieCatalogV1(), modelName)
}

// TierModels returns eyrie-preferred model IDs for haiku, sonnet, and opus tiers.
func TierModels(provider string) (haiku, sonnet, opus string) {
	return PreferredModelForTier(provider, eycatalog.TierHaiku, ""),
		PreferredModelForTier(provider, eycatalog.TierSonnet, ""),
		PreferredModelForTier(provider, eycatalog.TierOpus, "")
}

// RolesForProvider builds standard planner/coder/reviewer/commit roles from the catalog.
func RolesForProvider(provider string) ModelRoles {
	haiku, sonnet, opus := TierModels(provider)
	return ModelRoles{
		Planner:  opus,
		Coder:    sonnet,
		Reviewer: sonnet,
		Commit:   haiku,
	}
}

// SuggestTierForTask maps a task type to an eyrie cost tier (not a concrete model ID).
func SuggestTierForTask(taskType string) eycatalog.ModelTier {
	switch taskType {
	case "simple":
		return eycatalog.TierHaiku
	case "generation":
		return eycatalog.TierOpus
	default:
		return eycatalog.TierSonnet
	}
}

// AllCatalogModelNames returns model IDs from the eyrie catalog cache.
func AllCatalogModelNames() []string {
	return catalogModelNames(eyrieCatalogV1())
}

func catalogModelNames(compiled *eycatalog.CompiledCatalogV1) []string {
	if compiled == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for id, model := range compiled.ModelsByID {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
		if model.Name != "" && !seen[model.Name] {
			seen[model.Name] = true
			out = append(out, model.Name)
		}
	}
	if compiled.Catalog == nil {
		sort.Strings(out)
		return out
	}
	for alias, canonical := range compiled.Catalog.Aliases {
		if alias != "" && !seen[alias] {
			seen[alias] = true
			out = append(out, alias)
		}
		if canonical != "" && !seen[canonical] {
			seen[canonical] = true
			out = append(out, canonical)
		}
	}
	sort.Strings(out)
	return out
}

// DefaultHealthTiers builds complexity-based routing tiers from the eyrie catalog.
func DefaultHealthTiers(primaryProvider string) []ModelTier {
	primaryProvider = canonicalProvider(primaryProvider)
	light := tierModelList(primaryProvider, eycatalog.TierHaiku)
	standard := tierModelList(primaryProvider, eycatalog.TierSonnet)
	heavy := tierModelList(primaryProvider, eycatalog.TierOpus)
	return []ModelTier{
		{Name: "light", Models: light, MaxComplexity: 10.0},
		{Name: "standard", Models: standard, MaxComplexity: 30.0},
		{Name: "heavy", Models: heavy, MaxComplexity: 1e9},
	}
}

func tierModelList(primaryProvider string, tier eycatalog.ModelTier) []string {
	seen := map[string]bool{}
	var out []string
	for _, model := range eycatalog.PreferredModelsForTierV1(eyrieCatalogV1(), primaryProvider, tier, 3) {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		out = append(out, model)
	}
	return out
}

// PreferredModelForTier returns the eyrie-preferred model for a provider and tier.
func PreferredModelForTier(provider string, tier eycatalog.ModelTier, fallback string) string {
	return eycatalog.PreferredProviderModelV1(eyrieCatalogV1(), provider, tier, fallback)
}

// MostExpensiveForProvider picks the highest input-priced model for a provider.
func MostExpensiveForProvider(provider, fallback string) string {
	return eycatalog.MostExpensiveModelForProviderV1(eyrieCatalogV1(), provider, fallback)
}
