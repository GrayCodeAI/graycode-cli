package routing

import (
	"context"

	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
)

type CostTier int

const (
	CostTierCheap CostTier = iota
	CostTierMid
	CostTierExpensive
)

type PreferenceTier = gateway.ModelClass

const (
	TierHaiku  PreferenceTier = gateway.ModelClassEconomical
	TierSonnet PreferenceTier = gateway.ModelClassBalanced
	TierOpus   PreferenceTier = gateway.ModelClassPremium
)

func CostTierOf(modelName string) CostTier {
	switch gateway.ModelClassOf(context.Background(), modelName) {
	case gateway.ModelClassEconomical:
		return CostTierCheap
	case gateway.ModelClassPremium:
		return CostTierExpensive
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
	return gateway.ModelNames(context.Background())
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
	return gateway.PreferredModels(context.Background(), primaryProvider, tier, 3)
}

func PreferredModelForTier(provider string, tier PreferenceTier, fallback string) string {
	return gateway.PreferredModel(context.Background(), provider, tier, fallback)
}

func MostExpensiveForProvider(provider, fallback string) string {
	return PreferredModelForTier(provider, TierOpus, fallback)
}
