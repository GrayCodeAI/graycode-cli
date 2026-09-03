package engine

import (
	"github.com/GrayCodeAI/graycode-cli/internal/engine/token"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

type (
	TokenPredictor   = token.TokenPredictor
	PredictionRecord = token.PredictionRecord
	Prediction       = token.Prediction
	TokenEntry       = token.TokenEntry
	BudgetAlert      = token.BudgetAlert
	TokenReporter    = token.TokenReporter
)

func NewTokenPredictor() *TokenPredictor                { return token.NewTokenPredictor() }
func NewTokenReporter(sessionBudget int) *TokenReporter { return token.NewTokenReporter(sessionBudget) }

func DynamicMaxTokens(messages []types.EyrieMessage, contextSize int, taskType string) int {
	return token.DynamicMaxTokens(messages, contextSize, taskType)
}
func ClassifyTaskComplexity(task string) string { return token.ClassifyTaskComplexity(task) }
func FormatPrediction(pred *Prediction, model string) string {
	return token.FormatPrediction(pred, model)
}

func WarnIfExpensive(pred *Prediction, budgetUSD float64) string {
	return token.WarnIfExpensive(pred, budgetUSD)
}
func CountTokens(text string) int     { return token.CountTokens(text) }
func CountTokensFast(text string) int { return token.CountTokensFast(text) }
func CompressForContext(text string, budget int) (string, int) {
	return token.CompressForContext(text, budget)
}
func EstimateTokens(msgs []types.EyrieMessage) int   { return token.EstimateTokens(msgs) }
func EstimateMessageTokens(m types.EyrieMessage) int { return token.EstimateMessageTokens(m) }
