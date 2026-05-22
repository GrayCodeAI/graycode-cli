package engine

import (
	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/hawk/internal/engine/token"
)

type TokenPredictor = token.TokenPredictor
type PredictionRecord = token.PredictionRecord
type Prediction = token.Prediction
type TokenEntry = token.TokenEntry
type BudgetAlert = token.BudgetAlert
type TokenReporter = token.TokenReporter

func NewTokenPredictor() *TokenPredictor                { return token.NewTokenPredictor() }
func NewTokenReporter(sessionBudget int) *TokenReporter { return token.NewTokenReporter(sessionBudget) }
func DynamicMaxTokens(messages []client.EyrieMessage, contextSize int, taskType string) int {
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
func EstimateTokens(msgs []client.EyrieMessage) int   { return token.EstimateTokens(msgs) }
func EstimateMessageTokens(m client.EyrieMessage) int { return token.EstimateMessageTokens(m) }
