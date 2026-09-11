package token

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine/cost"
)

// TokenPredictor estimates how many tokens a task will consume before execution.
// It maintains a history of predictions vs actuals to calibrate future estimates.
type TokenPredictor struct {
	History      []PredictionRecord
	ModelFactors map[string]float64
	mu           sync.RWMutex
}

// PredictionRecord stores one prediction-vs-actual pair for calibration.
type PredictionRecord struct {
	TaskType  string
	Predicted int
	Actual    int
	Model     string
	Timestamp time.Time
	Accuracy  float64
}

// Prediction holds a token usage estimate for a task.
type Prediction struct {
	InputTokens   int
	OutputTokens  int
	TotalTokens   int
	EstimatedCost float64
	Confidence    float64
	Reasoning     string
}

// NewTokenPredictor creates a TokenPredictor with default model factors.
func NewTokenPredictor() *TokenPredictor {
	return &TokenPredictor{
		History: make([]PredictionRecord, 0),
		ModelFactors: map[string]float64{
			"claude-3-opus":     1.4,
			"claude-sonnet-4":   1.0,
			"claude-3-5-sonnet": 1.0,
			"claude-3-5-haiku":  0.7,
			"claude-3-haiku":    0.65,
			"gpt-4o":            1.0,
			"gpt-4o-mini":       0.75,
			"gpt-4-turbo":       1.1,
			"o1":                1.3,
			"o3":                1.2,
			"gemini-2.5-pro":    1.0,
			"deepseek-chat":     0.8,
		},
	}
}

// complexityProfile defines baseline input/output tokens for a complexity tier.
type complexityProfile struct {
	baseInput  int
	baseOutput int
}

var complexityProfiles = map[string]complexityProfile{
	"trivial":   {baseInput: 500, baseOutput: 200},
	"simple":    {baseInput: 2000, baseOutput: 1000},
	"moderate":  {baseInput: 5000, baseOutput: 3000},
	"complex":   {baseInput: 8000, baseOutput: 4000},
	"extensive": {baseInput: 15000, baseOutput: 8000},
}

// Predict estimates token usage for a task given its description, context size, and model.
func (tp *TokenPredictor) Predict(task string, contextSize int, model string) *Prediction {
	tp.mu.RLock()
	factor := tp.modelFactor(model)
	historyCount := tp.countSimilar(task, model)
	tp.mu.RUnlock()

	complexity := ClassifyTaskComplexity(task)
	profile := complexityProfiles[complexity]

	inputTokens := profile.baseInput + contextSize
	outputTokens := int(float64(profile.baseOutput) * factor)
	totalTokens := inputTokens + outputTokens

	cost := tp.EstimateCost(totalTokens, model)

	confidence := tp.calculateConfidence(historyCount, complexity)

	reasoning := fmt.Sprintf(
		"Task classified as %q complexity. Base estimate: %d input + %d output. "+
			"Model factor %.2f applied. Context adds %d tokens.",
		complexity, profile.baseInput, profile.baseOutput, factor, contextSize,
	)
	if historyCount > 0 {
		reasoning += fmt.Sprintf(" Calibrated from %d similar past tasks.", historyCount)
	}

	return &Prediction{
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		TotalTokens:   totalTokens,
		EstimatedCost: cost,
		Confidence:    confidence,
		Reasoning:     reasoning,
	}
}

// ClassifyTaskComplexity categorizes a task description into a complexity tier.
func ClassifyTaskComplexity(task string) string {
	lower := strings.ToLower(task)

	// Extensive: architecture-level changes
	extensiveKeywords := []string{
		"architect", "migration", "redesign", "rewrite entire",
		"full system", "overhaul", "migrate all", "restructure",
	}
	for _, kw := range extensiveKeywords {
		if strings.Contains(lower, kw) {
			return "extensive"
		}
	}

	// Complex: feature implementation, significant refactoring
	complexKeywords := []string{
		"implement feature", "full feature", "new feature",
		"refactor", "implement", "build a", "create a new module",
		"add support for", "integrate",
	}
	for _, kw := range complexKeywords {
		if strings.Contains(lower, kw) {
			return "complex"
		}
	}

	// Moderate: multi-file or debugging tasks
	moderateKeywords := []string{
		"debug", "multi-file", "multiple files", "fix bug",
		"investigate", "across files", "update several",
		"trace", "diagnose",
	}
	for _, kw := range moderateKeywords {
		if strings.Contains(lower, kw) {
			return "moderate"
		}
	}

	// Simple: single file edits, small fixes
	simpleKeywords := []string{
		"edit", "fix typo", "rename", "update", "change",
		"modify", "add field", "single file", "small fix",
		"adjust", "tweak",
	}
	for _, kw := range simpleKeywords {
		if strings.Contains(lower, kw) {
			return "simple"
		}
	}

	// Trivial: questions, one-word answers
	trivialKeywords := []string{
		"what is", "how do", "explain", "question",
		"what does", "why", "where is", "list",
		"show me", "describe",
	}
	for _, kw := range trivialKeywords {
		if strings.Contains(lower, kw) {
			return "trivial"
		}
	}

	// Default to simple if uncertain
	return "simple"
}

// EstimateCost calculates the USD cost for a given token count and model.
// It splits tokens roughly 60/40 input/output for a blended rate.
func (tp *TokenPredictor) EstimateCost(tokens int, model string) float64 {
	inPrice, outPrice := cost.ModelPricing(model)

	// Assume ~60% input, ~40% output when given a total
	inputPortion := float64(tokens) * 0.6
	outputPortion := float64(tokens) * 0.4

	// Prices are per million tokens
	cost := (inputPortion*inPrice + outputPortion*outPrice) / 1_000_000
	return math.Round(cost*1_000_000) / 1_000_000 // round to 6 decimal places
}

// RecordActual records the actual token usage after task completion for calibration.
func (tp *TokenPredictor) RecordActual(taskType string, predicted, actual int, model string) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	var accuracy float64
	if predicted > 0 {
		accuracy = 1.0 - math.Abs(float64(actual-predicted))/float64(predicted)
		if accuracy < 0 {
			accuracy = 0
		}
	}

	record := PredictionRecord{
		TaskType:  taskType,
		Predicted: predicted,
		Actual:    actual,
		Model:     model,
		Timestamp: time.Now(),
		Accuracy:  accuracy,
	}
	tp.History = append(tp.History, record)
}

// Calibrate adjusts ModelFactors based on prediction accuracy history.
// If predictions consistently under-predict for a model, increase its factor.
// If predictions consistently over-predict, decrease its factor.
func (tp *TokenPredictor) Calibrate() {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// Group records by model
	modelRecords := make(map[string][]PredictionRecord)
	for _, r := range tp.History {
		modelRecords[r.Model] = append(modelRecords[r.Model], r)
	}

	for model, records := range modelRecords {
		if len(records) < 3 {
			continue // need at least 3 data points
		}

		// Calculate average ratio: actual / predicted
		var totalRatio float64
		count := 0
		// Use only the last 20 records for recency
		start := 0
		if len(records) > 20 {
			start = len(records) - 20
		}
		for _, r := range records[start:] {
			if r.Predicted > 0 {
				totalRatio += float64(r.Actual) / float64(r.Predicted)
				count++
			}
		}

		if count == 0 {
			continue
		}

		avgRatio := totalRatio / float64(count)

		// Adjust factor: if actual is consistently higher than predicted,
		// increase the factor proportionally (with damping)
		currentFactor, ok := tp.ModelFactors[model]
		if !ok {
			currentFactor = 1.0
		}

		// Apply damped correction: move 30% toward the ideal factor
		adjustment := (avgRatio - 1.0) * 0.3
		newFactor := currentFactor * (1.0 + adjustment)

		// Clamp between 0.3 and 3.0
		if newFactor < 0.3 {
			newFactor = 0.3
		}
		if newFactor > 3.0 {
			newFactor = 3.0
		}

		tp.ModelFactors[model] = newFactor
	}
}

// GetAccuracy returns the mean absolute percentage error (MAPE) for predictions
// of the given model. Lower is better. Returns 0 if no history exists.
func (tp *TokenPredictor) GetAccuracy(model string) float64 {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	var totalError float64
	count := 0

	for _, r := range tp.History {
		if r.Model == model && r.Predicted > 0 {
			absError := math.Abs(float64(r.Actual-r.Predicted)) / float64(r.Actual)
			totalError += absError
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return totalError / float64(count)
}

// FormatPrediction returns a human-readable summary of a token prediction.
func FormatPrediction(pred *Prediction, model string) string {
	var sb strings.Builder
	sb.WriteString("Token Estimate:\n")
	sb.WriteString(fmt.Sprintf("  Input:  ~%s tokens (context + prompt)\n", predictorFormatNumber(pred.InputTokens)))
	sb.WriteString(fmt.Sprintf("  Output: ~%s tokens (response)\n", predictorFormatNumber(pred.OutputTokens)))
	sb.WriteString(fmt.Sprintf("  Total:  ~%s tokens\n", predictorFormatNumber(pred.TotalTokens)))
	sb.WriteString(fmt.Sprintf("  Cost:   ~$%.3f (%s)\n", pred.EstimatedCost, model))
	sb.WriteString(fmt.Sprintf("  Confidence: %.0f%%", pred.Confidence*100))
	if pred.Reasoning != "" {
		// Extract similar task count if present
		if idx := strings.Index(pred.Reasoning, "Calibrated from"); idx >= 0 {
			sb.WriteString(fmt.Sprintf(" (%s)", strings.TrimSuffix(strings.TrimPrefix(pred.Reasoning[idx:], "Calibrated from "), ".")))
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// WarnIfExpensive returns a warning string if the predicted cost exceeds
// 10% of the remaining budget. Returns empty string if within budget.
func WarnIfExpensive(pred *Prediction, budgetUSD float64) string {
	if budgetUSD <= 0 {
		return ""
	}

	threshold := budgetUSD * 0.10
	if pred.EstimatedCost > threshold {
		pct := (pred.EstimatedCost / budgetUSD) * 100
		return fmt.Sprintf(
			"WARNING: Estimated cost $%.3f is %.1f%% of remaining budget ($%.2f). "+
				"Consider simplifying the task or using a cheaper model.",
			pred.EstimatedCost, pct, budgetUSD,
		)
	}

	return ""
}

// modelFactor returns the output scaling factor for a model.
// Must be called with at least a read lock held.
func (tp *TokenPredictor) modelFactor(model string) float64 {
	lower := strings.ToLower(model)
	for key, factor := range tp.ModelFactors {
		if strings.Contains(lower, key) {
			return factor
		}
	}
	return 1.0
}

// countSimilar returns how many historical records match the task type.
// Must be called with at least a read lock held.
func (tp *TokenPredictor) countSimilar(task string, model string) int {
	complexity := ClassifyTaskComplexity(task)
	count := 0
	for _, r := range tp.History {
		if r.TaskType == complexity && r.Model == model {
			count++
		}
	}
	return count
}

// calculateConfidence returns a confidence score [0,1] based on history
// and complexity. More history and simpler tasks = higher confidence.
func (tp *TokenPredictor) calculateConfidence(historyCount int, complexity string) float64 {
	// Base confidence by complexity (simpler tasks are more predictable)
	baseConfidence := map[string]float64{
		"trivial":   0.85,
		"simple":    0.75,
		"moderate":  0.60,
		"complex":   0.45,
		"extensive": 0.30,
	}

	conf := baseConfidence[complexity]
	if conf == 0 {
		conf = 0.50
	}

	// Increase confidence with more history (diminishing returns)
	if historyCount > 0 {
		boost := math.Min(float64(historyCount)*0.03, 0.25)
		conf += boost
	}

	if conf > 0.95 {
		conf = 0.95
	}

	return conf
}

// predictorFormatNumber adds commas to an integer for display.
func predictorFormatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	s := fmt.Sprintf("%d", n)
	result := make([]byte, 0, len(s)+(len(s)-1)/3)

	remainder := len(s) % 3
	if remainder == 0 {
		remainder = 3
	}

	result = append(result, s[:remainder]...)
	for i := remainder; i < len(s); i += 3 {
		result = append(result, ',')
		result = append(result, s[i:i+3]...)
	}

	return string(result)
}
