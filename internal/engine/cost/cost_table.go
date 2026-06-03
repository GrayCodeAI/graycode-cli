package cost

import (
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/provider/routing"
)

// Tier-based default pricing when catalog data is unavailable.
var tierDefaults = map[routing.CostTier][2]float64{
	routing.CostTierCheap:     {0.15, 0.60},
	routing.CostTierMid:       {3.0, 15.0},
	routing.CostTierExpensive: {15.0, 75.0},
}

var livePricing sync.Map // model id -> [2]float64{input per 1M, output per 1M}

// RegisterLivePricing records per-million token rates from a live catalog fetch.
// Overrides compiled-catalog defaults for footer cost and pre-request estimates.
func RegisterLivePricing(model string, inputPerM, outputPerM float64) {
	model = strings.TrimSpace(model)
	if model == "" || (inputPerM <= 0 && outputPerM <= 0) {
		return
	}
	livePricing.Store(model, [2]float64{inputPerM, outputPerM})
}

func ModelPricing(modelName string) (inputPricePerM, outputPricePerM float64) {
	modelName = strings.TrimSpace(modelName)
	if v, ok := livePricing.Load(modelName); ok {
		if p, ok := v.([2]float64); ok && (p[0] > 0 || p[1] > 0) {
			return p[0], p[1]
		}
	}
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
