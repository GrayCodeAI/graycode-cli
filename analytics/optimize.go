package analytics

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// CostEntry represents a single LLM API call with cost metadata.
type CostEntry struct {
	SessionID    string        `json:"session_id"`
	Model        string        `json:"model"`
	TaskType     string        `json:"task_type"`     // "simple", "generation", "debug", "refactor", "review", "chat"
	InputTokens  int           `json:"input_tokens"`
	OutputTokens int           `json:"output_tokens"`
	CostUSD      float64       `json:"cost_usd"`
	Duration     time.Duration `json:"duration"`
	Kept         bool          `json:"kept"` // was the output used/kept?
	Timestamp    time.Time     `json:"timestamp"`
}

// OptimizationReport contains spend analysis and recommendations.
type OptimizationReport struct {
	TotalSpend      float64                `json:"total_spend"`
	WastedSpend     float64                `json:"wasted_spend"`     // spend on simple tasks with expensive models
	AbandonedSpend  float64                `json:"abandoned_spend"`  // spend on outputs that were discarded
	ProductiveSpend float64                `json:"productive_spend"` // spend on outputs that were kept
	YieldRate       float64                `json:"yield_rate"`       // productive / total
	Recommendations []Recommendation       `json:"recommendations"`
	ByModel         map[string]*ModelSpend `json:"by_model"`
	ByTaskType      map[string]*TaskSpend  `json:"by_task_type"`
}

// ModelSpend aggregates cost data for a specific model.
type ModelSpend struct {
	Model     string  `json:"model"`
	TotalCost float64 `json:"total_cost"`
	Calls     int     `json:"calls"`
	AvgTokens int     `json:"avg_tokens"`
}

// TaskSpend aggregates cost data for a specific task type.
type TaskSpend struct {
	TaskType       string  `json:"task_type"`
	TotalCost      float64 `json:"total_cost"`
	Calls          int     `json:"calls"`
	SuggestedModel string  `json:"suggested_model"` // cheaper model that would suffice
}

// Recommendation represents a cost optimization suggestion.
type Recommendation struct {
	Type        string  `json:"type"`        // "downgrade", "batch", "cache"
	Description string  `json:"description"`
	Savings     float64 `json:"savings"` // estimated USD savings
}

// Model pricing tiers (approximate cost per million input tokens).
var modelPricing = map[string]float64{
	// Anthropic
	"claude-opus-4-20250514":   15.0,
	"claude-sonnet-4-20250514": 3.0,
	"claude-haiku-3-20250307":  0.25,
	"opus":                     15.0,
	"sonnet":                   3.0,
	"haiku":                    0.25,
	// OpenAI
	"gpt-4":        30.0,
	"gpt-4-turbo":  10.0,
	"gpt-4o":       2.5,
	"gpt-4o-mini":  0.15,
	"gpt-3.5-turbo": 0.5,
	// Fallback tiers
	"expensive": 15.0,
	"mid":       3.0,
	"cheap":     0.25,
}

// expensiveModelThreshold is the per-million-token cost above which a model
// is considered "expensive" for simple tasks.
const expensiveModelThreshold = 5.0

// isExpensiveModel returns true if the model's per-million-token input cost
// exceeds the threshold for simple tasks.
func isExpensiveModel(model string) bool {
	normalized := normalizeModelName(model)
	if price, ok := modelPricing[normalized]; ok {
		return price >= expensiveModelThreshold
	}
	// Unknown models: check if the name contains an expensive tier keyword.
	lower := strings.ToLower(model)
	if strings.Contains(lower, "opus") {
		return true
	}
	// "gpt-4" is expensive, but "gpt-4o" and "gpt-4o-mini" are not.
	if strings.Contains(lower, "gpt-4") && !strings.Contains(lower, "gpt-4o") {
		return true
	}
	return false
}

// normalizeModelName maps common model name variations to canonical keys.
// Uses ordered matching to ensure longer/more-specific names match first
// (e.g., "gpt-4o-mini" before "gpt-4o" before "gpt-4").
func normalizeModelName(model string) string {
	lower := strings.ToLower(model)

	// Ordered list: longer/more-specific patterns first.
	patterns := []struct {
		substring string
		canonical string
	}{
		{"gpt-4o-mini", "gpt-4o-mini"},
		{"gpt-4o", "gpt-4o"},
		{"gpt-4-turbo", "gpt-4-turbo"},
		{"gpt-4", "gpt-4"},
		{"gpt-3.5", "gpt-3.5-turbo"},
		{"opus", "opus"},
		{"sonnet", "sonnet"},
		{"haiku", "haiku"},
	}
	for _, p := range patterns {
		if strings.Contains(lower, p.substring) {
			return p.canonical
		}
	}
	return model
}

// Analyze examines cost entries and produces optimization recommendations.
func Analyze(entries []CostEntry) *OptimizationReport {
	report := &OptimizationReport{
		ByModel:    make(map[string]*ModelSpend),
		ByTaskType: make(map[string]*TaskSpend),
	}

	if len(entries) == 0 {
		return report
	}

	for _, e := range entries {
		report.TotalSpend += e.CostUSD

		if e.Kept {
			report.ProductiveSpend += e.CostUSD
		} else {
			report.AbandonedSpend += e.CostUSD
		}

		// Simple tasks on expensive models count as wasted spend.
		if e.TaskType == "simple" && isExpensiveModel(e.Model) {
			report.WastedSpend += e.CostUSD
		}

		// Aggregate by model.
		ms, ok := report.ByModel[e.Model]
		if !ok {
			ms = &ModelSpend{Model: e.Model}
			report.ByModel[e.Model] = ms
		}
		ms.TotalCost += e.CostUSD
		ms.Calls++
		totalTokens := e.InputTokens + e.OutputTokens
		// Running average: update incrementally.
		ms.AvgTokens = ((ms.AvgTokens * (ms.Calls - 1)) + totalTokens) / ms.Calls

		// Aggregate by task type.
		ts, ok := report.ByTaskType[e.TaskType]
		if !ok {
			ts = &TaskSpend{TaskType: e.TaskType}
			report.ByTaskType[e.TaskType] = ts
		}
		ts.TotalCost += e.CostUSD
		ts.Calls++
	}

	// Compute yield rate.
	if report.TotalSpend > 0 {
		report.YieldRate = report.ProductiveSpend / report.TotalSpend
	}

	// Fill in suggested models for each task type.
	for _, ts := range report.ByTaskType {
		ts.SuggestedModel = SuggestModel(ts.TaskType, "")
	}

	// Generate recommendations.
	report.Recommendations = generateOptimizationRecs(entries, report)

	return report
}

// generateOptimizationRecs produces actionable recommendations sorted by savings.
func generateOptimizationRecs(entries []CostEntry, report *OptimizationReport) []Recommendation {
	var recs []Recommendation

	// 1. Downgrade recommendations: simple tasks on expensive models.
	simpleOnExpensive := 0
	var simpleExpensiveCost float64
	for _, e := range entries {
		if e.TaskType == "simple" && isExpensiveModel(e.Model) {
			simpleOnExpensive++
			simpleExpensiveCost += e.CostUSD
		}
	}
	if simpleOnExpensive > 0 {
		// Estimate savings: switching from expensive to cheap model saves ~90% on those calls.
		savings := simpleExpensiveCost * 0.90
		recs = append(recs, Recommendation{
			Type: "downgrade",
			Description: fmt.Sprintf(
				"%d simple tasks were sent to expensive models (total $%.4f). "+
					"Route simple questions to a cheaper model (haiku/gpt-4o-mini) to save ~$%.4f.",
				simpleOnExpensive, simpleExpensiveCost, savings),
			Savings: savings,
		})
	}

	// 2. Downgrade recommendations per task type that could use cheaper models.
	for _, e := range entries {
		if e.TaskType == "chat" && isExpensiveModel(e.Model) {
			// Gather chat-on-expensive stats.
			var chatExpensiveCost float64
			chatCount := 0
			for _, ce := range entries {
				if ce.TaskType == "chat" && isExpensiveModel(ce.Model) {
					chatExpensiveCost += ce.CostUSD
					chatCount++
				}
			}
			if chatCount > 0 {
				savings := chatExpensiveCost * 0.80
				recs = append(recs, Recommendation{
					Type: "downgrade",
					Description: fmt.Sprintf(
						"%d chat/conversational tasks used expensive models ($%.4f). "+
							"Consider using sonnet/gpt-4o for general conversation to save ~$%.4f.",
						chatCount, chatExpensiveCost, savings),
					Savings: savings,
				})
			}
			break // Only add this recommendation once.
		}
	}

	// 3. Cache recommendation: repeated similar calls.
	if report.TotalSpend > 1.0 {
		// If there are many calls from few task types, caching could help.
		avgCallsPerType := 0
		if len(report.ByTaskType) > 0 {
			total := 0
			for _, ts := range report.ByTaskType {
				total += ts.Calls
			}
			avgCallsPerType = total / len(report.ByTaskType)
		}
		if avgCallsPerType > 10 {
			savings := report.TotalSpend * 0.15
			recs = append(recs, Recommendation{
				Type: "cache",
				Description: fmt.Sprintf(
					"High call volume (avg %d calls/task-type). "+
						"Enable prompt caching to reduce redundant token costs by ~$%.4f.",
					avgCallsPerType, savings),
				Savings: savings,
			})
		}
	}

	// 4. Abandoned output warning.
	if report.AbandonedSpend > 0 && report.TotalSpend > 0 {
		abandonRate := report.AbandonedSpend / report.TotalSpend * 100
		if abandonRate > 20 {
			recs = append(recs, Recommendation{
				Type: "batch",
				Description: fmt.Sprintf(
					"%.0f%% of spend ($%.4f) was on abandoned/discarded outputs. "+
						"Consider iterating with cheaper models first, then upgrading for final output.",
					abandonRate, report.AbandonedSpend),
				Savings: report.AbandonedSpend * 0.50,
			})
		}
	}

	// Sort by savings descending.
	sort.Slice(recs, func(i, j int) bool {
		return recs[i].Savings > recs[j].Savings
	})

	return recs
}

// ClassifyTask determines the task type from a prompt/response pair.
// Returns one of: "simple", "generation", "debug", "refactor", "review", "chat".
func ClassifyTask(prompt string, response string) string {
	lowerPrompt := strings.ToLower(prompt)
	lowerResponse := strings.ToLower(response)

	hasCodeBlock := strings.Contains(response, "```") ||
		strings.Contains(response, "func ") ||
		strings.Contains(response, "def ") ||
		strings.Contains(response, "class ") ||
		strings.Contains(response, "import ")

	// Check prompt-based intent signals first. These take priority over
	// response-length heuristics because a short response to a debug or
	// review prompt is still a debug/review task.

	// Debugging: error-related prompts.
	if containsAny(lowerPrompt, "fix", "bug", "error", "debug", "broken", "failing", "crash", "panic", "stack trace") {
		return "debug"
	}

	// Refactoring: restructuring-related prompts.
	if containsAny(lowerPrompt, "refactor", "rename", "reorganize", "simplify", "clean up", "restructure", "extract") {
		return "refactor"
	}

	// Review: code review prompts.
	if containsAny(lowerPrompt, "review", "check this", "look over", "feedback", "critique", "audit") {
		return "review"
	}

	// Chat: conversational, explanation, or question prompts.
	if containsAny(lowerPrompt, "explain", "what is", "how does", "why", "tell me", "describe") {
		return "chat"
	}

	// Simple tasks: short responses without code, yes/no answers, lookups.
	// Only classify as simple when no strong prompt-based signal was found.
	responseTokenEstimate := len(strings.Fields(response))
	if responseTokenEstimate < 50 && !hasCodeBlock {
		return "simple"
	}

	// Code generation: prompts that result in code.
	if hasCodeBlock || containsAny(lowerPrompt, "implement", "create", "write", "build", "add", "generate") {
		return "generation"
	}

	// Remaining conversational indicators in the response.
	if containsAny(lowerResponse, "here's", "the answer", "in summary") {
		return "chat"
	}

	// Default to chat for anything unclassified.
	return "chat"
}

// SuggestModel recommends the cheapest adequate model for a task type.
// If currentModel is empty, it returns the recommended model for the task type.
func SuggestModel(taskType string, currentModel string) string {
	switch taskType {
	case "simple":
		return "haiku"
	case "chat":
		return "sonnet"
	case "review":
		return "sonnet"
	case "debug":
		return "sonnet"
	case "refactor":
		return "sonnet"
	case "generation":
		// Complex generation benefits from top-tier models.
		return "opus"
	default:
		return "sonnet"
	}
}

// FormatOptimizationReport produces a human-readable optimization report.
func FormatOptimizationReport(r *OptimizationReport) string {
	var b strings.Builder

	b.WriteString("=== Cost Optimization Report ===\n\n")
	b.WriteString(fmt.Sprintf("  Total spend:      $%.4f\n", r.TotalSpend))
	b.WriteString(fmt.Sprintf("  Productive spend: $%.4f\n", r.ProductiveSpend))
	b.WriteString(fmt.Sprintf("  Abandoned spend:  $%.4f\n", r.AbandonedSpend))
	b.WriteString(fmt.Sprintf("  Wasted spend:     $%.4f\n", r.WastedSpend))
	b.WriteString(fmt.Sprintf("  Yield rate:       %.1f%%\n", r.YieldRate*100))

	// By model.
	b.WriteString("\n--- Spend by Model ---\n")
	var models []*ModelSpend
	for _, ms := range r.ByModel {
		models = append(models, ms)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].TotalCost > models[j].TotalCost })
	for _, ms := range models {
		b.WriteString(fmt.Sprintf("  %-30s %5d calls  avg %5d tokens  $%.4f\n",
			ms.Model, ms.Calls, ms.AvgTokens, ms.TotalCost))
	}

	// By task type.
	b.WriteString("\n--- Spend by Task Type ---\n")
	var tasks []*TaskSpend
	for _, ts := range r.ByTaskType {
		tasks = append(tasks, ts)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].TotalCost > tasks[j].TotalCost })
	for _, ts := range tasks {
		suggested := ""
		if ts.SuggestedModel != "" {
			suggested = fmt.Sprintf(" (suggested: %s)", ts.SuggestedModel)
		}
		b.WriteString(fmt.Sprintf("  %-15s %5d calls  $%.4f%s\n",
			ts.TaskType, ts.Calls, ts.TotalCost, suggested))
	}

	// Recommendations.
	if len(r.Recommendations) > 0 {
		b.WriteString("\n--- Recommendations ---\n")
		for i, rec := range r.Recommendations {
			b.WriteString(fmt.Sprintf("  %d. [%s] %s (est. savings: $%.4f)\n",
				i+1, rec.Type, rec.Description, rec.Savings))
		}
	}

	return b.String()
}
