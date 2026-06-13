package routing

import (
	"sort"
	"strings"

	eycatalog "github.com/GrayCodeAI/eyrie/catalog"
)

// CostTier is a relative cost band for cascade routing (cheap / mid / expensive).
type CostTier int

const (
	CostTierCheap CostTier = iota
	CostTierMid
	CostTierExpensive
)

// CostTierOf resolves a model's cost tier from eyrie catalog data (family and
// within-provider pricing). Unknown models default to mid-tier.
func CostTierOf(modelName string) CostTier {
	if tier, ok := tierFromCatalogFamily(modelName); ok {
		return mapEyrieTier(tier)
	}
	if tier, ok := tierFromCatalogPricing(modelName); ok {
		return tier
	}
	return tierFromName(modelName)
}

// tierFromName infers cost tier from well-known model name patterns.
// This is a fallback when the eyrie catalog is unavailable or incomplete.
func tierFromName(modelName string) CostTier {
	lower := strings.ToLower(strings.TrimSpace(modelName))
	for _, pat := range cheapPatterns {
		if strings.Contains(lower, pat) {
			return CostTierCheap
		}
	}
	for _, pat := range expensivePatterns {
		if strings.Contains(lower, pat) {
			return CostTierExpensive
		}
	}
	return CostTierMid
}

var (
	cheapPatterns     = []string{"haiku", "mini", "flash", "lite", "nano", "micro", "small", "tiny"}
	expensivePatterns = []string{"opus", "pro", "max", "ultra", "heavy", "large", "o1", "o3"}
)

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
	if primaryProvider == "" {
		primaryProvider = "anthropic"
	}
	light := tierModelList(primaryProvider, eycatalog.TierHaiku, "openai", "gemini")
	standard := tierModelList(primaryProvider, eycatalog.TierSonnet, "openai", "gemini")
	heavy := tierModelList(primaryProvider, eycatalog.TierOpus, "openai", "gemini")
	return []ModelTier{
		{Name: "light", Models: light, MaxComplexity: 10.0},
		{Name: "standard", Models: standard, MaxComplexity: 30.0},
		{Name: "heavy", Models: heavy, MaxComplexity: 1e9},
	}
}

func tierModelList(primaryProvider string, tier eycatalog.ModelTier, extraProviders ...string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(m string) {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	add(PreferredModelForTier(primaryProvider, tier, ""))
	for _, p := range extraProviders {
		add(PreferredModelForTier(p, tier, ""))
	}
	return out
}

// PreferredModelForTier returns the eyrie-preferred model for a provider and tier.
func PreferredModelForTier(provider string, tier eycatalog.ModelTier, fallback string) string {
	provider = canonicalProvider(provider)
	if provider == "" {
		return fallback
	}
	if m := eycatalog.GetPreferredProviderModel(provider, tier, nil); m != "" {
		return m
	}
	return fallback
}

// MostExpensiveForProvider picks the highest input-priced model for a provider.
func MostExpensiveForProvider(provider, fallback string) string {
	models := ByProvider(canonicalProvider(provider))
	if len(models) == 0 {
		return fallback
	}
	best := models[0]
	for _, m := range models[1:] {
		if m.InputPrice > best.InputPrice {
			best = m
		}
	}
	if best.Name != "" {
		return best.Name
	}
	return fallback
}

func mapEyrieTier(tier eycatalog.ModelTier) CostTier {
	switch tier {
	case eycatalog.TierHaiku:
		return CostTierCheap
	case eycatalog.TierOpus:
		return CostTierExpensive
	default:
		return CostTierMid
	}
}

func tierFromCatalogFamily(modelName string) (eycatalog.ModelTier, bool) {
	compiled := eyrieCatalogV1()
	if compiled == nil {
		return "", false
	}
	canonical := modelName
	if c, ok := compiled.CanonicalModelForAliasOrID(modelName); ok {
		canonical = c
	}
	model := compiled.ModelsByID[canonical]
	if model.ID == "" {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(model.Family)) {
	case "haiku", "cheap", "lite", "flash", "mini":
		return eycatalog.TierHaiku, true
	case "opus", "pro", "max", "heavy", "ultra":
		return eycatalog.TierOpus, true
	case "sonnet", "standard", "balanced", "medium":
		return eycatalog.TierSonnet, true
	}
	return "", false
}

func tierFromCatalogPricing(modelName string) (CostTier, bool) {
	info, ok := Find(modelName)
	if !ok || info.InputPrice <= 0 {
		return 0, false
	}
	models := ByProvider(canonicalProvider(info.Provider))
	if len(models) < 2 {
		return 0, false
	}

	prices := make([]float64, 0, len(models))
	seen := map[float64]bool{}
	for _, m := range models {
		if m.InputPrice <= 0 || seen[m.InputPrice] {
			continue
		}
		seen[m.InputPrice] = true
		prices = append(prices, m.InputPrice)
	}
	if len(prices) < 2 {
		return 0, false
	}
	sort.Float64s(prices)

	price := info.InputPrice
	switch {
	case price <= prices[0]:
		return CostTierCheap, true
	case price >= prices[len(prices)-1]:
		return CostTierExpensive, true
	default:
		return CostTierMid, true
	}
}
