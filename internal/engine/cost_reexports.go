package engine

import (
	"github.com/GrayCodeAI/hawk/internal/engine/cost"
	analytics "github.com/GrayCodeAI/hawk/internal/observability"
)

type (
	Cost           = cost.Cost
	CostOptimizer  = cost.CostOptimizer
	CostTracker    = cost.CostTracker
	RequestCost    = cost.RequestCost
	ModelPrice     = cost.ModelPrice
	Recommendation = cost.Recommendation
)

func NewCostOptimizer() *CostOptimizer                { return cost.NewCostOptimizer() }
func NewCostTracker(sessionID string) *CostTracker    { return cost.NewCostTracker(sessionID) }
func LoadCostHistory() ([]analytics.CostEntry, error) { return cost.LoadCostHistory() }
func FormatCostDisplay(totalUSD float64) string       { return cost.FormatCostDisplay(totalUSD) }
func ModelPricing(modelName string) (inputPricePerM, outputPricePerM float64) {
	return cost.ModelPricing(modelName)
}
func RegisterLivePricing(model string, inputPerM, outputPerM float64) {
	cost.RegisterLivePricing(model, inputPerM, outputPerM)
}
