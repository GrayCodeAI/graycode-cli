// Package cost is the Stage-1 namespace for cost-tracking types and
// functions in package engine. See ../REFACTOR_PLAN.md.
//
// New code in hawk should import this package instead of reaching into
// engine for cost symbols. Implementation will move here in Stage 2.
package cost

import (
	"github.com/GrayCodeAI/hawk/internal/observability"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

// Cost is the canonical cost record (input/output tokens + USD).
type Cost = engine.Cost

// Optimizer recommends cheaper models / shorter prompts when costs trend up.
type Optimizer = engine.CostOptimizer

// Tracker accumulates per-session cost and persists it to analytics.
type Tracker = engine.CostTracker

// RequestCost is the cost of a single LLM request.
type RequestCost = engine.RequestCost

// ModelPrice is a per-million-token price tuple for a single model.
type ModelPrice = engine.ModelPrice

// Recommendation is an Optimizer's suggested change.
type Recommendation = engine.Recommendation

// NewOptimizer returns a fresh cost optimizer.
func NewOptimizer() *Optimizer {
	return engine.NewCostOptimizer()
}

// NewTracker returns a tracker scoped to the given session.
func NewTracker(sessionID string) *Tracker {
	return engine.NewCostTracker(sessionID)
}

// LoadHistory reads persisted cost entries from analytics storage.
func LoadHistory() ([]analytics.CostEntry, error) {
	return engine.LoadCostHistory()
}

// FormatDisplay renders a USD value for terminal display.
func FormatDisplay(totalUSD float64) string {
	return engine.FormatCostDisplay(totalUSD)
}

// ModelPricing returns input + output USD-per-million-token prices for a model.
func ModelPricing(modelName string) (inputPricePerM, outputPricePerM float64) {
	return engine.ModelPricing(modelName)
}
