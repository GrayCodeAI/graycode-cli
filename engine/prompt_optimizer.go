package engine

import (
	"encoding/json"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// PromptOptimizer implements a DSPy-inspired prompt optimization system.
// It automatically improves system prompts based on success/failure data
// by curating few-shot examples and running A/B tests on prompt variants.
type PromptOptimizer struct {
	Examples    []DSPyExample          `json:"examples"`
	MaxExamples int                    `json:"max_examples"`
	Metrics     map[string]float64     `json:"metrics"`
	mu          sync.RWMutex
}

// DSPyExample represents a curated example from a successful session,
// used for few-shot prompt injection.
type DSPyExample struct {
	Task       string    `json:"task"`
	Approach   string    `json:"approach"`
	Outcome    string    `json:"outcome"`
	ToolsUsed  []string  `json:"tools_used"`
	TokensUsed int       `json:"tokens_used"`
	Score      float64   `json:"score"`
	Timestamp  time.Time `json:"timestamp"`
}

// DSPyVariant represents a prompt template variant being tested.
type DSPyVariant struct {
	ID          string  `json:"id"`
	Template    string  `json:"template"`
	SuccessRate float64 `json:"success_rate"`
	UsageCount  int     `json:"usage_count"`
	AvgTokens   float64 `json:"avg_tokens"`
}

// ABTest manages an A/B test between two prompt variants using
// Thompson sampling for efficient exploration/exploitation.
type ABTest struct {
	VariantA      DSPyVariant `json:"variant_a"`
	VariantB      DSPyVariant `json:"variant_b"`
	SuccessesA    int         `json:"successes_a"`
	FailuresA     int         `json:"failures_a"`
	SuccessesB    int         `json:"successes_b"`
	FailuresB     int         `json:"failures_b"`
	mu            sync.Mutex
}

// NewPromptOptimizer creates a new optimizer with default settings.
func NewPromptOptimizer() *PromptOptimizer {
	return &PromptOptimizer{
		Examples:    make([]DSPyExample, 0),
		MaxExamples: 5,
		Metrics:     make(map[string]float64),
	}
}

// RecordOutcome records a task outcome for learning. If the outcome is
// successful and high-quality, it adds the example to the few-shot pool.
// It maintains diversity by not keeping too many similar examples.
func (po *PromptOptimizer) RecordOutcome(task, approach, outcome string, toolsUsed []string, tokens int) {
	po.mu.Lock()
	defer po.mu.Unlock()

	// Update metrics
	key := normalizeTask(task)
	if outcome == "success" {
		po.Metrics[key] = po.Metrics[key]*0.9 + 0.1
	} else {
		po.Metrics[key] = po.Metrics[key] * 0.9
	}

	// Only add successful outcomes as examples
	if outcome != "success" {
		return
	}

	// Check diversity: don't add if too similar to existing examples
	for _, ex := range po.Examples {
		if optimizerJaccardSimilarity(task, ex.Task) > 0.7 {
			// Too similar, skip (but update score of existing if this is better)
			if tokens < ex.TokensUsed {
				ex.Score = math.Min(1.0, ex.Score+0.1)
			}
			return
		}
	}

	// Compute quality score based on token efficiency
	score := 0.7 // base score for success
	if tokens < 1000 {
		score += 0.2
	} else if tokens < 5000 {
		score += 0.1
	}
	if len(toolsUsed) > 0 && len(toolsUsed) <= 3 {
		score += 0.1 // bonus for focused tool usage
	}
	if score > 1.0 {
		score = 1.0
	}

	ex := DSPyExample{
		Task:       task,
		Approach:   approach,
		Outcome:    outcome,
		ToolsUsed:  toolsUsed,
		TokensUsed: tokens,
		Score:      score,
		Timestamp:  time.Now(),
	}

	po.Examples = append(po.Examples, ex)

	// Keep the pool within bounds (MaxExamples * 3 is the soft limit)
	if len(po.Examples) > po.MaxExamples*3 {
		po.pruneLowest()
	}
}

// SelectExamples picks the most relevant examples for a given task using
// keyword overlap scoring. Prioritizes recency, relevance, and diversity.
func (po *PromptOptimizer) SelectExamples(task string, n int) []DSPyExample {
	po.mu.RLock()
	defer po.mu.RUnlock()

	if len(po.Examples) == 0 {
		return nil
	}

	if n > len(po.Examples) {
		n = len(po.Examples)
	}
	if n > po.MaxExamples {
		n = po.MaxExamples
	}

	type scored struct {
		index int
		score float64
	}

	var candidates []scored
	for i, ex := range po.Examples {
		s := po.ScoreExample(ex, task)
		candidates = append(candidates, scored{index: i, score: s})
	}

	// Sort by score descending
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Select with diversity enforcement
	var selected []DSPyExample
	for _, c := range candidates {
		if len(selected) >= n {
			break
		}
		ex := po.Examples[c.index]

		// Diversity check: don't select examples too similar to already selected
		tooSimilar := false
		for _, sel := range selected {
			if optimizerJaccardSimilarity(ex.Task, sel.Task) > 0.3 {
				tooSimilar = true
				break
			}
		}
		if tooSimilar {
			continue
		}

		selected = append(selected, ex)
	}

	return selected
}

// BuildOptimizedPrompt injects selected few-shot examples into the base prompt.
func (po *PromptOptimizer) BuildOptimizedPrompt(basePrompt string, task string) string {
	examples := po.SelectExamples(task, po.MaxExamples)
	if len(examples) == 0 {
		return basePrompt
	}

	var b strings.Builder
	b.WriteString(basePrompt)
	b.WriteString("\n\n## Successful Approaches (learn from these)\n")

	for _, ex := range examples {
		b.WriteString("\nTask: ")
		b.WriteString(ex.Task)
		b.WriteString("\nApproach: ")
		b.WriteString(ex.Approach)
		b.WriteString("\nTools used: ")
		b.WriteString(strings.Join(ex.ToolsUsed, ", "))
		b.WriteString("\n")
	}

	return b.String()
}

// ScoreExample computes a relevance score for an example given a task.
// Score is based on: word overlap (Jaccard), recency, success, and diversity.
func (po *PromptOptimizer) ScoreExample(ex DSPyExample, task string) float64 {
	// Jaccard similarity for relevance
	relevance := optimizerJaccardSimilarity(task, ex.Task)

	// Recency bonus: examples from last 24h get full bonus, decays over 7 days
	age := time.Since(ex.Timestamp)
	recencyBonus := 0.0
	if age < 24*time.Hour {
		recencyBonus = 0.3
	} else if age < 7*24*time.Hour {
		recencyBonus = 0.3 * (1.0 - float64(age)/(7*24*float64(time.Hour)))
	}

	// Success bonus
	successBonus := 0.0
	if ex.Outcome == "success" {
		successBonus = 0.2
	}

	// Quality factor from the example's own score
	qualityFactor := ex.Score * 0.2

	total := relevance*0.5 + recencyBonus + successBonus + qualityFactor
	if total > 1.0 {
		total = 1.0
	}
	return total
}

// PruneExamples removes examples older than maxAge and low-scoring examples
// when the pool exceeds the soft limit.
func (po *PromptOptimizer) PruneExamples(maxAge time.Duration) {
	po.mu.Lock()
	defer po.mu.Unlock()

	now := time.Now()
	var kept []DSPyExample

	for _, ex := range po.Examples {
		if now.Sub(ex.Timestamp) > maxAge {
			continue
		}
		kept = append(kept, ex)
	}

	po.Examples = kept

	// If still over the soft limit, remove lowest scoring
	if len(po.Examples) > po.MaxExamples*3 {
		po.pruneLowest()
	}
}

// pruneLowest removes the lowest-scoring examples to get back under the soft limit.
// Must be called with mu held.
func (po *PromptOptimizer) pruneLowest() {
	// Sort by score descending
	sorted := make([]DSPyExample, len(po.Examples))
	copy(sorted, po.Examples)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Score > sorted[i].Score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Keep only MaxExamples * 3
	limit := po.MaxExamples * 3
	if limit > len(sorted) {
		limit = len(sorted)
	}
	po.Examples = sorted[:limit]
}

// ExportExamples serializes all examples to JSON for persistence.
func (po *PromptOptimizer) ExportExamples() ([]byte, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()
	return json.Marshal(po.Examples)
}

// ImportExamples deserializes examples from JSON data.
func (po *PromptOptimizer) ImportExamples(data []byte) error {
	po.mu.Lock()
	defer po.mu.Unlock()

	var examples []DSPyExample
	if err := json.Unmarshal(data, &examples); err != nil {
		return err
	}
	po.Examples = examples
	return nil
}

// NewABTest creates a new A/B test between two prompt variants.
func NewABTest(a, b DSPyVariant) *ABTest {
	return &ABTest{
		VariantA: a,
		VariantB: b,
	}
}

// RecordResult records the outcome of using a variant.
func (ab *ABTest) RecordResult(variant string, success bool) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	switch variant {
	case "A", "a":
		if success {
			ab.SuccessesA++
		} else {
			ab.FailuresA++
		}
		ab.VariantA.UsageCount++
		if ab.VariantA.UsageCount > 0 {
			ab.VariantA.SuccessRate = float64(ab.SuccessesA) / float64(ab.VariantA.UsageCount)
		}
	case "B", "b":
		if success {
			ab.SuccessesB++
		} else {
			ab.FailuresB++
		}
		ab.VariantB.UsageCount++
		if ab.VariantB.UsageCount > 0 {
			ab.VariantB.SuccessRate = float64(ab.SuccessesB) / float64(ab.VariantB.UsageCount)
		}
	}
}

// Winner returns the better variant after sufficient trials (>20 total).
// Uses Thompson sampling (Beta distribution approximation) to determine
// the winner with statistical confidence. Returns empty string if
// insufficient data.
func (ab *ABTest) Winner() string {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	totalA := ab.SuccessesA + ab.FailuresA
	totalB := ab.SuccessesB + ab.FailuresB

	if totalA+totalB < 20 {
		return ""
	}

	// Thompson sampling: sample from Beta(successes+1, failures+1)
	// Use mean + variance approximation for comparison
	// Beta mean = alpha / (alpha + beta)
	// We run multiple samples to approximate
	winsA := 0
	winsB := 0
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	alphaA := float64(ab.SuccessesA + 1)
	betaA := float64(ab.FailuresA + 1)
	alphaB := float64(ab.SuccessesB + 1)
	betaB := float64(ab.FailuresB + 1)

	// Monte Carlo sampling to compare distributions
	for i := 0; i < 1000; i++ {
		sampleA := betaSample(rng, alphaA, betaA)
		sampleB := betaSample(rng, alphaB, betaB)
		if sampleA > sampleB {
			winsA++
		} else {
			winsB++
		}
	}

	// Require >75% confidence to declare a winner
	if winsA > 750 {
		return "A"
	}
	if winsB > 750 {
		return "B"
	}
	return ""
}

// PickVariant uses Thompson sampling to select which variant to try next.
// Returns "A" or "B".
func (ab *ABTest) PickVariant() string {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	alphaA := float64(ab.SuccessesA + 1)
	betaA := float64(ab.FailuresA + 1)
	alphaB := float64(ab.SuccessesB + 1)
	betaB := float64(ab.FailuresB + 1)

	sampleA := betaSample(rng, alphaA, betaA)
	sampleB := betaSample(rng, alphaB, betaB)

	if sampleA >= sampleB {
		return "A"
	}
	return "B"
}

// betaSample approximates a sample from Beta(alpha, beta) using the
// inverse transform method with a normal approximation for large params.
func betaSample(rng *rand.Rand, alpha, beta float64) float64 {
	// For small alpha+beta, use the gamma ratio method
	if alpha+beta < 40 {
		return gammaSample(rng, alpha) / (gammaSample(rng, alpha) + gammaSample(rng, beta))
	}
	// Normal approximation for large parameters
	mean := alpha / (alpha + beta)
	variance := (alpha * beta) / ((alpha + beta) * (alpha + beta) * (alpha + beta + 1))
	stddev := math.Sqrt(variance)
	sample := mean + stddev*rng.NormFloat64()
	if sample < 0 {
		sample = 0.001
	}
	if sample > 1 {
		sample = 0.999
	}
	return sample
}

// gammaSample generates a sample from Gamma(alpha, 1) using Marsaglia's method.
func gammaSample(rng *rand.Rand, alpha float64) float64 {
	if alpha < 1 {
		// Boost: Gamma(alpha) = Gamma(alpha+1) * U^(1/alpha)
		return gammaSample(rng, alpha+1) * math.Pow(rng.Float64(), 1.0/alpha)
	}
	d := alpha - 1.0/3.0
	c := 1.0 / math.Sqrt(9.0*d)
	for {
		var x, v float64
		for {
			x = rng.NormFloat64()
			v = 1 + c*x
			if v > 0 {
				break
			}
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1-0.0331*(x*x)*(x*x) {
			return d * v
		}
		if math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}

// optimizerJaccardSimilarity computes the Jaccard similarity coefficient between
// the word sets of two strings.
func optimizerJaccardSimilarity(a, b string) float64 {
	wordsA := tokenizeWords(a)
	wordsB := tokenizeWords(b)

	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 1.0
	}
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0.0
	}

	setA := make(map[string]bool, len(wordsA))
	for _, w := range wordsA {
		setA[w] = true
	}

	setB := make(map[string]bool, len(wordsB))
	for _, w := range wordsB {
		setB[w] = true
	}

	intersection := 0
	for w := range setA {
		if setB[w] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// tokenizeWords splits a string into lowercase words, filtering short ones.
func tokenizeWords(s string) []string {
	words := strings.Fields(strings.ToLower(s))
	var result []string
	for _, w := range words {
		// Remove punctuation
		w = strings.TrimFunc(w, func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
		})
		if len(w) > 2 {
			result = append(result, w)
		}
	}
	return result
}

// normalizeTask creates a normalized key from a task description.
func normalizeTask(task string) string {
	words := tokenizeWords(task)
	if len(words) > 5 {
		words = words[:5]
	}
	return strings.Join(words, "_")
}
