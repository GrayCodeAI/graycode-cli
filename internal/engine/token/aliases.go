// Package token is the Stage-1 namespace for token-related types and
// functions in package engine. See ../REFACTOR_PLAN.md.
//
// New code in hawk should import this package instead of reaching into
// engine for token symbols. Implementation will move here in Stage 2.
package token

import (
	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/hawk/engine"
)

// Predictor estimates token usage for upcoming requests based on history.
type Predictor = engine.TokenPredictor

// PredictionRecord is one historical observation backing the predictor.
type PredictionRecord = engine.PredictionRecord

// Prediction is the output of a single estimation pass.
type Prediction = engine.Prediction

// Entry records token consumption for a single request.
type Entry = engine.TokenEntry

// BudgetAlert is raised when token usage crosses a configured threshold.
type BudgetAlert = engine.BudgetAlert

// Reporter aggregates per-session token consumption + budget alerts.
type Reporter = engine.TokenReporter

// NewPredictor returns a fresh empty token predictor.
func NewPredictor() *Predictor {
	return engine.NewTokenPredictor()
}

// NewReporter returns a token reporter with the given session-wide budget.
func NewReporter(sessionBudget int) *Reporter {
	return engine.NewTokenReporter(sessionBudget)
}

// DynamicMaxTokens chooses a sensible max-tokens value for a given context
// size and task type, capped to model limits.
func DynamicMaxTokens(messages []client.EyrieMessage, contextSize int, taskType string) int {
	return engine.DynamicMaxTokens(messages, contextSize, taskType)
}

// ClassifyTaskComplexity returns a coarse category ("simple", "medium",
// "complex") inferred from the task text.
func ClassifyTaskComplexity(task string) string {
	return engine.ClassifyTaskComplexity(task)
}

// FormatPrediction renders a prediction for display.
func FormatPrediction(pred *Prediction, model string) string {
	return engine.FormatPrediction(pred, model)
}

// WarnIfExpensive returns a non-empty warning string when the predicted
// cost exceeds the budget, otherwise empty.
func WarnIfExpensive(pred *Prediction, budgetUSD float64) string {
	return engine.WarnIfExpensive(pred, budgetUSD)
}
