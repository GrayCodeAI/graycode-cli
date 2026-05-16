package engine

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// CostOptimizer analyzes usage patterns and suggests ways to reduce API costs.
type CostOptimizer struct {
	History         []RequestCost
	Recommendations []Recommendation
	ModelPricing    map[string]ModelPrice
	mu              sync.RWMutex
}

// RequestCost records the cost details of a single API request.
type RequestCost struct {
	Model        string
	Provider     string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	TaskType     string // "chat", "code", "review", "summarize"
	Duration     time.Duration
	CacheHit     bool
	Timestamp    time.Time
}

// ModelPrice holds the pricing for a model per million tokens.
type ModelPrice struct {
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
}

// Recommendation represents a cost optimization recommendation.
type Recommendation struct {
	Type             string // "model_switch", "caching", "compression", "batching"
	Description      string
	EstimatedSavings float64 // USD per day
	Priority         string  // "high", "medium", "low"
	Action           string  // what to do
}

// NewCostOptimizer creates a CostOptimizer with default pricing for common models.
func NewCostOptimizer() *CostOptimizer {
	return &CostOptimizer{
		History:         make([]RequestCost, 0),
		Recommendations: make([]Recommendation, 0),
		ModelPricing: map[string]ModelPrice{
			"claude-opus": {
				InputPerMillion:      15.0,
				OutputPerMillion:     75.0,
				CacheReadPerMillion:  1.5,
				CacheWritePerMillion: 18.75,
			},
			"claude-sonnet": {
				InputPerMillion:      3.0,
				OutputPerMillion:     15.0,
				CacheReadPerMillion:  0.3,
				CacheWritePerMillion: 3.75,
			},
			"claude-haiku": {
				InputPerMillion:      0.25,
				OutputPerMillion:     1.25,
				CacheReadPerMillion:  0.025,
				CacheWritePerMillion: 0.3125,
			},
			"gpt-4o": {
				InputPerMillion:      2.50,
				OutputPerMillion:     10.0,
				CacheReadPerMillion:  0.25,
				CacheWritePerMillion: 3.125,
			},
			"gpt-4o-mini": {
				InputPerMillion:      0.15,
				OutputPerMillion:     0.60,
				CacheReadPerMillion:  0.015,
				CacheWritePerMillion: 0.1875,
			},
		},
	}
}

// Record adds a RequestCost entry to the history.
func (co *CostOptimizer) Record(cost RequestCost) {
	co.mu.Lock()
	defer co.mu.Unlock()
	co.History = append(co.History, cost)
}

// Analyze scans the history for optimization opportunities and returns recommendations.
func (co *CostOptimizer) Analyze() []Recommendation {
	co.mu.Lock()
	defer co.mu.Unlock()

	recommendations := make([]Recommendation, 0)

	if len(co.History) == 0 {
		co.Recommendations = recommendations
		return recommendations
	}

	// Model downgrade analysis: simple tasks on expensive models
	recommendations = append(recommendations, co.analyzeModelDowngrade()...)

	// Caching analysis: repeated requests without cache hits
	recommendations = append(recommendations, co.analyzeCaching()...)

	// Compression analysis: large input tokens
	recommendations = append(recommendations, co.analyzeCompression()...)

	// Batching analysis: many small sequential calls
	recommendations = append(recommendations, co.analyzeBatching()...)

	// Time-of-day analysis: recommend scheduling non-urgent work
	recommendations = append(recommendations, co.analyzeScheduling()...)

	// Token reduction: output tokens consistently high
	recommendations = append(recommendations, co.analyzeTokenReduction()...)

	co.Recommendations = recommendations
	return recommendations
}

// analyzeModelDowngrade checks if simple tasks use expensive models.
func (co *CostOptimizer) analyzeModelDowngrade() []Recommendation {
	var recs []Recommendation

	simpleTasks := map[string]bool{"chat": true, "summarize": true}
	expensiveModels := map[string]bool{"claude-opus": true, "claude-sonnet": true, "gpt-4o": true}

	var expensiveSimpleCount int
	var expensiveSimpleCost float64

	for _, rc := range co.History {
		model := co.normalizeModel(rc.Model)
		if simpleTasks[rc.TaskType] && expensiveModels[model] {
			expensiveSimpleCount++
			expensiveSimpleCost += rc.CostUSD
		}
	}

	if expensiveSimpleCount > 0 {
		// Estimate savings: assume haiku would cost ~5% of opus, ~8% of sonnet
		estimatedSavings := expensiveSimpleCost * 0.9
		days := co.historyDays()
		if days < 1 {
			days = 1
		}
		dailySavings := estimatedSavings / days

		recs = append(recs, Recommendation{
			Type:             "model_switch",
			Description:      fmt.Sprintf("Switch %d simple tasks (chat/summarize) to a cheaper model like claude-haiku", expensiveSimpleCount),
			EstimatedSavings: dailySavings,
			Priority:         "high",
			Action:           "Route chat and summarize tasks to claude-haiku or gpt-4o-mini instead of expensive models",
		})
	}

	return recs
}

// analyzeCaching checks for repeated requests without cache hits.
func (co *CostOptimizer) analyzeCaching() []Recommendation {
	var recs []Recommendation

	var totalRequests int
	var cacheHits int
	var totalInputCost float64

	for _, rc := range co.History {
		totalRequests++
		if rc.CacheHit {
			cacheHits++
		}
		pricing := co.getPricing(rc.Model)
		totalInputCost += float64(rc.InputTokens) * pricing.InputPerMillion / 1_000_000
	}

	if totalRequests > 5 {
		cacheRate := float64(cacheHits) / float64(totalRequests)
		if cacheRate < 0.3 {
			// Could save ~80% on input costs with caching
			estimatedSavings := totalInputCost * 0.8
			days := co.historyDays()
			if days < 1 {
				days = 1
			}
			dailySavings := estimatedSavings / days

			recs = append(recs, Recommendation{
				Type:             "caching",
				Description:      fmt.Sprintf("Only %.0f%% cache hit rate across %d requests — enable prompt caching", cacheRate*100, totalRequests),
				EstimatedSavings: dailySavings,
				Priority:         "medium",
				Action:           "Enable prompt caching for repeated system prompts and common prefixes",
			})
		}
	}

	return recs
}

// analyzeCompression checks if average input tokens exceed threshold.
func (co *CostOptimizer) analyzeCompression() []Recommendation {
	var recs []Recommendation

	if len(co.History) == 0 {
		return recs
	}

	var totalInput int
	var largeInputCount int
	var largeInputCost float64

	for _, rc := range co.History {
		totalInput += rc.InputTokens
		if rc.InputTokens > 5000 {
			largeInputCount++
			largeInputCost += rc.CostUSD
		}
	}

	avgInput := totalInput / len(co.History)
	if avgInput > 5000 && largeInputCount > 0 {
		// Compression could save ~30% on large inputs
		estimatedSavings := largeInputCost * 0.3
		days := co.historyDays()
		if days < 1 {
			days = 1
		}
		dailySavings := estimatedSavings / days

		recs = append(recs, Recommendation{
			Type:             "compression",
			Description:      fmt.Sprintf("Average input is %d tokens; %d requests exceed 5K tokens", avgInput, largeInputCount),
			EstimatedSavings: dailySavings,
			Priority:         "low",
			Action:           "Compress inputs over 5K tokens using token compression (tok) to reduce costs",
		})
	}

	return recs
}

// analyzeBatching checks for many small sequential calls.
func (co *CostOptimizer) analyzeBatching() []Recommendation {
	var recs []Recommendation

	if len(co.History) < 5 {
		return recs
	}

	// Sort by timestamp
	sorted := make([]RequestCost, len(co.History))
	copy(sorted, co.History)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	// Look for clusters of small requests within 60 seconds
	var batchableCount int
	var batchableCost float64
	for i := 1; i < len(sorted); i++ {
		gap := sorted[i].Timestamp.Sub(sorted[i-1].Timestamp)
		if gap < 60*time.Second && sorted[i].InputTokens < 1000 && sorted[i-1].InputTokens < 1000 {
			batchableCount++
			batchableCost += sorted[i].CostUSD
		}
	}

	if batchableCount >= 5 {
		// Batching overhead reduction ~20%
		estimatedSavings := batchableCost * 0.2
		days := co.historyDays()
		if days < 1 {
			days = 1
		}
		dailySavings := estimatedSavings / days

		recs = append(recs, Recommendation{
			Type:             "batching",
			Description:      fmt.Sprintf("%d small sequential requests could be batched", batchableCount),
			EstimatedSavings: dailySavings,
			Priority:         "medium",
			Action:           "Batch small sequential API calls to reduce overhead and per-request costs",
		})
	}

	return recs
}

// analyzeScheduling recommends off-peak scheduling for non-urgent work.
func (co *CostOptimizer) analyzeScheduling() []Recommendation {
	var recs []Recommendation

	if len(co.History) < 10 {
		return recs
	}

	// Check what fraction of requests happen during business hours (9-17)
	var peakCount int
	var totalCost float64
	for _, rc := range co.History {
		hour := rc.Timestamp.Hour()
		if hour >= 9 && hour < 17 {
			peakCount++
		}
		totalCost += rc.CostUSD
	}

	peakRatio := float64(peakCount) / float64(len(co.History))
	if peakRatio > 0.7 {
		// Batch API is ~50% cheaper for non-urgent work
		nonUrgentFraction := 0.3 // assume 30% of work is non-urgent
		estimatedSavings := totalCost * nonUrgentFraction * 0.5
		days := co.historyDays()
		if days < 1 {
			days = 1
		}
		dailySavings := estimatedSavings / days

		recs = append(recs, Recommendation{
			Type:             "batching",
			Description:      fmt.Sprintf("%.0f%% of requests during peak hours — schedule non-urgent work off-peak", peakRatio*100),
			EstimatedSavings: dailySavings,
			Priority:         "low",
			Action:           "Use batch API for non-urgent tasks to take advantage of 50% cost reduction",
		})
	}

	return recs
}

// analyzeTokenReduction checks if output tokens are consistently high.
func (co *CostOptimizer) analyzeTokenReduction() []Recommendation {
	var recs []Recommendation

	if len(co.History) < 5 {
		return recs
	}

	var highOutputCount int
	var totalOutputCost float64

	for _, rc := range co.History {
		if rc.OutputTokens > 4000 {
			highOutputCount++
			pricing := co.getPricing(rc.Model)
			totalOutputCost += float64(rc.OutputTokens) * pricing.OutputPerMillion / 1_000_000
		}
	}

	highRatio := float64(highOutputCount) / float64(len(co.History))
	if highRatio > 0.5 {
		// Shorter system prompts could reduce output by ~20%
		estimatedSavings := totalOutputCost * 0.2
		days := co.historyDays()
		if days < 1 {
			days = 1
		}
		dailySavings := estimatedSavings / days

		recs = append(recs, Recommendation{
			Type:             "compression",
			Description:      fmt.Sprintf("%.0f%% of responses exceed 4K output tokens — consider shorter prompts", highRatio*100),
			EstimatedSavings: dailySavings,
			Priority:         "medium",
			Action:           "Shorten system prompts and add max_tokens constraints to reduce verbose output",
		})
	}

	return recs
}

// DailyCost returns the sum of costs from the last 24 hours.
func (co *CostOptimizer) DailyCost() float64 {
	co.mu.RLock()
	defer co.mu.RUnlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	var total float64
	for _, rc := range co.History {
		if rc.Timestamp.After(cutoff) {
			total += rc.CostUSD
		}
	}
	return total
}

// WeeklyCost returns the sum of costs from the last 7 days.
func (co *CostOptimizer) WeeklyCost() float64 {
	co.mu.RLock()
	defer co.mu.RUnlock()

	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	var total float64
	for _, rc := range co.History {
		if rc.Timestamp.After(cutoff) {
			total += rc.CostUSD
		}
	}
	return total
}

// CostByModel returns a map of model name to total cost.
func (co *CostOptimizer) CostByModel() map[string]float64 {
	co.mu.RLock()
	defer co.mu.RUnlock()

	result := make(map[string]float64)
	for _, rc := range co.History {
		result[rc.Model] += rc.CostUSD
	}
	return result
}

// CostByTaskType returns a map of task type to total cost.
func (co *CostOptimizer) CostByTaskType() map[string]float64 {
	co.mu.RLock()
	defer co.mu.RUnlock()

	result := make(map[string]float64)
	for _, rc := range co.History {
		result[rc.TaskType] += rc.CostUSD
	}
	return result
}

// ProjectSavings calculates the total estimated daily savings if all recommendations are applied.
func (co *CostOptimizer) ProjectSavings(recommendations []Recommendation) float64 {
	var total float64
	for _, r := range recommendations {
		total += r.EstimatedSavings
	}
	return total
}

// FormatReport generates a formatted cost report string.
func (co *CostOptimizer) FormatReport() string {
	co.mu.RLock()
	defer co.mu.RUnlock()

	var b strings.Builder

	weeklyCost := co.weeklyCostLocked()

	b.WriteString("Cost Report (Last 7 Days):\n")
	b.WriteString(fmt.Sprintf("Total: $%.2f\n", weeklyCost))

	// By Model
	modelCosts := make(map[string]float64)
	for _, rc := range co.History {
		cutoff := time.Now().Add(-7 * 24 * time.Hour)
		if rc.Timestamp.After(cutoff) {
			modelCosts[rc.Model] += rc.CostUSD
		}
	}

	if len(modelCosts) > 0 {
		b.WriteString("\nBy Model:\n")
		sortedModels := sortMapByValue(modelCosts)
		for _, kv := range sortedModels {
			pct := 0.0
			if weeklyCost > 0 {
				pct = kv.Value / weeklyCost * 100
			}
			b.WriteString(fmt.Sprintf("  %s: $%.2f (%.0f%%)\n", kv.Key, kv.Value, pct))
		}
	}

	// By Task
	taskCosts := make(map[string]float64)
	for _, rc := range co.History {
		cutoff := time.Now().Add(-7 * 24 * time.Hour)
		if rc.Timestamp.After(cutoff) {
			taskCosts[rc.TaskType] += rc.CostUSD
		}
	}

	if len(taskCosts) > 0 {
		b.WriteString("\nBy Task:\n")
		sortedTasks := sortMapByValue(taskCosts)
		for _, kv := range sortedTasks {
			pct := 0.0
			if weeklyCost > 0 {
				pct = kv.Value / weeklyCost * 100
			}
			b.WriteString(fmt.Sprintf("  %s: $%.2f (%.0f%%)\n", kv.Key, kv.Value, pct))
		}
	}

	// Recommendations
	if len(co.Recommendations) > 0 {
		b.WriteString("\nRecommendations:\n")
		for _, rec := range co.Recommendations {
			icon := "\U0001f535" // blue circle
			label := "LOW"
			switch rec.Priority {
			case "high":
				icon = "\U0001f7e2" // green circle
				label = "HIGH"
			case "medium":
				icon = "\U0001f7e1" // yellow circle
				label = "MED"
			}
			b.WriteString(fmt.Sprintf("%s %s: %s (saves ~$%.2f/day)\n", icon, label, rec.Action, rec.EstimatedSavings))
		}

		totalSavings := co.projectSavingsLocked(co.Recommendations)
		monthlySavings := totalSavings * 30
		b.WriteString(fmt.Sprintf("\nProjected savings: $%.2f/day ($%.0f/month)\n", totalSavings, monthlySavings))
	}

	return b.String()
}

// WhatIf calculates what the total cost would have been if all requests used the given model.
func (co *CostOptimizer) WhatIf(model string) float64 {
	co.mu.RLock()
	defer co.mu.RUnlock()

	pricing := co.getPricing(model)
	var total float64
	for _, rc := range co.History {
		inputCost := float64(rc.InputTokens) * pricing.InputPerMillion / 1_000_000
		outputCost := float64(rc.OutputTokens) * pricing.OutputPerMillion / 1_000_000
		total += inputCost + outputCost
	}
	return total
}

// Helper methods

func (co *CostOptimizer) normalizeModel(model string) string {
	lower := strings.ToLower(model)
	if strings.Contains(lower, "opus") {
		return "claude-opus"
	}
	if strings.Contains(lower, "sonnet") {
		return "claude-sonnet"
	}
	if strings.Contains(lower, "haiku") {
		return "claude-haiku"
	}
	if strings.Contains(lower, "gpt-4o-mini") {
		return "gpt-4o-mini"
	}
	if strings.Contains(lower, "gpt-4o") {
		return "gpt-4o"
	}
	return model
}

func (co *CostOptimizer) getPricing(model string) ModelPrice {
	normalized := co.normalizeModel(model)
	if p, ok := co.ModelPricing[normalized]; ok {
		return p
	}
	// Default to sonnet pricing
	return ModelPrice{
		InputPerMillion:  3.0,
		OutputPerMillion: 15.0,
	}
}

func (co *CostOptimizer) historyDays() float64 {
	if len(co.History) < 2 {
		return 1
	}
	earliest := co.History[0].Timestamp
	latest := co.History[0].Timestamp
	for _, rc := range co.History {
		if rc.Timestamp.Before(earliest) {
			earliest = rc.Timestamp
		}
		if rc.Timestamp.After(latest) {
			latest = rc.Timestamp
		}
	}
	days := latest.Sub(earliest).Hours() / 24
	if days < 1 {
		return 1
	}
	return days
}

func (co *CostOptimizer) weeklyCostLocked() float64 {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	var total float64
	for _, rc := range co.History {
		if rc.Timestamp.After(cutoff) {
			total += rc.CostUSD
		}
	}
	return total
}

func (co *CostOptimizer) projectSavingsLocked(recommendations []Recommendation) float64 {
	var total float64
	for _, r := range recommendations {
		total += r.EstimatedSavings
	}
	return total
}

// keyValue is used for sorting maps by value.
type keyValue struct {
	Key   string
	Value float64
}

func sortMapByValue(m map[string]float64) []keyValue {
	kvs := make([]keyValue, 0, len(m))
	for k, v := range m {
		kvs = append(kvs, keyValue{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool {
		return kvs[i].Value > kvs[j].Value // descending
	})
	return kvs
}
