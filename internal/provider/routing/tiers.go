package routing

import (
	"context"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
)

type CostTier int

const (
	CostTierCheap CostTier = iota
	CostTierMid
	CostTierExpensive
)

type PreferenceTier = eyrieengine.ModelClass

const (
	TierHaiku  PreferenceTier = eyrieengine.ModelClassEconomical
	TierSonnet PreferenceTier = eyrieengine.ModelClassBalanced
	TierOpus   PreferenceTier = eyrieengine.ModelClassPremium
)

func CostTierOf(modelName string) CostTier {
	if engine := eyrieModelEngine(); engine != nil {
		switch engine.ModelClassOf(context.Background(), modelName) {
		case eyrieengine.ModelClassEconomical:
			return CostTierCheap
		case eyrieengine.ModelClassPremium:
			return CostTierExpensive
		}
	}
	return CostTierMid
}

func TierModels(provider string) (haiku, sonnet, opus string) {
	return PreferredModelForTier(provider, TierHaiku, ""),
		PreferredModelForTier(provider, TierSonnet, ""),
		PreferredModelForTier(provider, TierOpus, "")
}

func RolesForProvider(provider string) ModelRoles {
	haiku, sonnet, opus := TierModels(provider)
	return ModelRoles{Planner: opus, Coder: sonnet, Reviewer: sonnet, Commit: haiku}
}

func SuggestTierForTask(taskType string) PreferenceTier {
	switch taskType {
	case "simple":
		return TierHaiku
	case "generation":
		return TierOpus
	default:
		return TierSonnet
	}
}

func AllCatalogModelNames() []string {
	if engine := eyrieModelEngine(); engine != nil {
		return engine.ModelNames(context.Background())
	}
	return nil
}

func DefaultHealthTiers(primaryProvider string) []ModelTier {
	primaryProvider = canonicalProvider(primaryProvider)
	return []ModelTier{
		{Name: "light", Models: tierModelList(primaryProvider, TierHaiku), MaxComplexity: 10.0},
		{Name: "standard", Models: tierModelList(primaryProvider, TierSonnet), MaxComplexity: 30.0},
		{Name: "heavy", Models: tierModelList(primaryProvider, TierOpus), MaxComplexity: 1e9},
	}
}

func tierModelList(primaryProvider string, tier PreferenceTier) []string {
	engine := eyrieModelEngine()
	if engine == nil {
		return nil
	}
	models := engine.PreferredModels(context.Background(), primaryProvider, tier, 3)
	return models
}

func PreferredModelForTier(provider string, tier PreferenceTier, fallback string) string {
	if engine := eyrieModelEngine(); engine != nil {
		return engine.PreferredModel(context.Background(), provider, tier, fallback)
	}
	return fallback
}

func MostExpensiveForProvider(provider, fallback string) string {
	return PreferredModelForTier(provider, TierOpus, fallback)
}
