package cost

import "github.com/GrayCodeAI/hawk/internal/provider/routing"

// Tier-based default pricing when catalog data is unavailable.
var tierDefaults = map[routing.CostTier][2]float64{
	routing.CostTierCheap:     {0.15, 0.60},
	routing.CostTierMid:       {3.0, 15.0},
	routing.CostTierExpensive: {15.0, 75.0},
}

func ModelPricing(modelName string) (inputPricePerM, outputPricePerM float64) {
	info, ok := routing.Find(modelName)
	if ok && (info.InputPrice > 0 || info.OutputPrice > 0) {
		return info.InputPrice, info.OutputPrice
	}
	// Fall back to tier-based defaults so routing decisions still produce
	// meaningful cost estimates even when the catalog lacks pricing data.
	tier := routing.CostTierOf(modelName)
	if d, ok := tierDefaults[tier]; ok {
		return d[0], d[1]
	}
	return 3.0, 15.0
}
