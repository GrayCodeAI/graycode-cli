package review

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/tok"
)

// ConsensusSampler implements the multi-sample consensus pattern inspired by
// SWE-agent's "Ask Colleagues" approach: generate N solutions in parallel,
// then select the best one using a configurable strategy.
type ConsensusSampler struct {
	NumSamples int
	Strategy   string // "majority", "best_score", "synthesize"
	ScoreFn    func(solution string) float64
	mu         sync.Mutex
}

// Sample represents a single generated solution with metadata.
type Sample struct {
	ID         int
	Content    string
	Score      float64
	Duration   time.Duration
	TokensUsed int
}

// ConsensusResult holds the outcome of multi-sample consensus.
type ConsensusResult struct {
	Winner     *Sample
	AllSamples []Sample
	Agreement  float64
	Method     string
}

// NewConsensusSampler creates a ConsensusSampler with the given number of samples.
// If numSamples is <= 0, it defaults to 3.
func NewConsensusSampler(numSamples int) *ConsensusSampler {
	if numSamples <= 0 {
		numSamples = 3
	}
	return &ConsensusSampler{
		NumSamples: numSamples,
		Strategy:   "majority",
		ScoreFn:    DefaultScoreFn,
	}
}

// DefaultScoreFn combines length and completeness scoring.
func DefaultScoreFn(solution string) float64 {
	return (ScoreByLength(solution) + ScoreByCompleteness(solution)) / 2.0
}

// SampleSolutions generates N solutions in parallel, scores each, and selects
// a winner based on the configured strategy.
func (cs *ConsensusSampler) SampleSolutions(ctx context.Context, prompt string, generateFn func(context.Context, string) (string, error)) (*ConsensusResult, error) {
	cs.mu.Lock()
	numSamples := cs.NumSamples
	strategy := cs.Strategy
	scoreFn := cs.ScoreFn
	cs.mu.Unlock()

	if scoreFn == nil {
		scoreFn = DefaultScoreFn
	}

	type sampleResult struct {
		sample Sample
		err    error
	}

	results := make(chan sampleResult, numSamples)

	for i := 0; i < numSamples; i++ {
		go func(id int) {
			start := time.Now()
			content, err := generateFn(ctx, prompt)
			duration := time.Since(start)
			if err != nil {
				results <- sampleResult{err: err}
				return
			}
			s := Sample{
				ID:         id + 1,
				Content:    content,
				Duration:   duration,
				TokensUsed: estimateTokens(content),
			}
			results <- sampleResult{sample: s}
		}(i)
	}

	var samples []Sample
	var firstErr error
	for i := 0; i < numSamples; i++ {
		select {
		case <-ctx.Done():
			if len(samples) == 0 {
				return nil, ctx.Err()
			}
			// Use whatever we have so far
			goto score
		case r := <-results:
			if r.err != nil {
				if firstErr == nil {
					firstErr = r.err
				}
				continue
			}
			samples = append(samples, r.sample)
		}
	}

score:
	if len(samples) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("all samples failed: %w", firstErr)
		}
		return nil, fmt.Errorf("no samples generated")
	}

	// Score all samples
	for i := range samples {
		samples[i].Score = scoreFn(samples[i].Content)
	}

	// Select winner based on strategy
	var winner *Sample
	switch strategy {
	case "best_score":
		winner = BestScore(samples)
	case "synthesize":
		winner = Synthesize(samples)
	default: // "majority"
		winner = MajorityVote(samples)
	}

	// Calculate agreement
	agreement := calculateAgreement(samples, winner)

	return &ConsensusResult{
		Winner:     winner,
		AllSamples: samples,
		Agreement:  agreement,
		Method:     strategy,
	}, nil
}

// MajorityVote finds the most similar/common solution using pairwise similarity.
// The sample with the highest average similarity to all others wins.
func MajorityVote(samples []Sample) *Sample {
	if len(samples) == 0 {
		return nil
	}
	if len(samples) == 1 {
		return &samples[0]
	}

	bestIdx := 0
	bestAvgSim := -1.0

	for i := range samples {
		totalSim := 0.0
		for j := range samples {
			if i == j {
				continue
			}
			totalSim += PairwiseSimilarity(samples[i].Content, samples[j].Content)
		}
		avgSim := totalSim / float64(len(samples)-1)
		if avgSim > bestAvgSim {
			bestAvgSim = avgSim
			bestIdx = i
		}
	}

	result := samples[bestIdx]
	return &result
}

// BestScore picks the highest-scored solution.
func BestScore(samples []Sample) *Sample {
	if len(samples) == 0 {
		return nil
	}

	bestIdx := 0
	for i := 1; i < len(samples); i++ {
		if samples[i].Score > samples[bestIdx].Score {
			bestIdx = i
		}
	}

	result := samples[bestIdx]
	return &result
}

// Synthesize combines elements from all solutions, weighted by score.
// It selects unique paragraphs from higher-scoring samples first.
func Synthesize(samples []Sample) *Sample {
	if len(samples) == 0 {
		return nil
	}
	if len(samples) == 1 {
		return &samples[0]
	}

	// Sort by score descending (simple selection)
	sorted := make([]Sample, len(samples))
	copy(sorted, samples)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Score > sorted[i].Score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Build synthesized content from paragraphs of each solution
	seen := make(map[string]bool)
	var parts []string

	for _, s := range sorted {
		paragraphs := strings.Split(s.Content, "\n\n")
		for _, p := range paragraphs {
			trimmed := strings.TrimSpace(p)
			if trimmed == "" {
				continue
			}
			// Use first few words as a dedup key
			key := normalizeKey(trimmed)
			if !seen[key] {
				seen[key] = true
				parts = append(parts, trimmed)
			}
		}
	}

	synthesized := strings.Join(parts, "\n\n")

	return &Sample{
		ID:         0, // synthetic
		Content:    synthesized,
		Score:      sorted[0].Score, // inherit best score
		TokensUsed: estimateTokens(synthesized),
	}
}

// ScoreByLength scores content based on reasonable length.
// Too short or too long content gets penalized.
func ScoreByLength(content string) float64 {
	length := len(content)
	if length == 0 {
		return 0.0
	}

	// Ideal range: 200-2000 characters
	const idealMin = 200
	const idealMax = 2000

	if length >= idealMin && length <= idealMax {
		return 1.0
	}

	if length < idealMin {
		return float64(length) / float64(idealMin)
	}

	// Gradually penalize overly long content
	excess := float64(length-idealMax) / float64(idealMax)
	score := 1.0 - (excess * 0.3)
	if score < 0.3 {
		return 0.3
	}
	return score
}

// ScoreByCompleteness scores based on structural indicators:
// code blocks, file mentions, numbered steps.
func ScoreByCompleteness(content string) float64 {
	if content == "" {
		return 0.0
	}

	score := 0.0
	checks := 0.0

	// Has code blocks
	checks++
	if strings.Contains(content, "```") || strings.Contains(content, "    ") {
		score++
	}

	// Mentions files (paths with extensions)
	checks++
	words := strings.Fields(content)
	for _, w := range words {
		if strings.Contains(w, "/") && strings.Contains(w, ".") {
			score++
			break
		}
		if strings.HasSuffix(w, ".go") || strings.HasSuffix(w, ".py") ||
			strings.HasSuffix(w, ".js") || strings.HasSuffix(w, ".ts") ||
			strings.HasSuffix(w, ".rs") || strings.HasSuffix(w, ".java") {
			score++
			break
		}
	}

	// Has steps or structured content
	checks++
	if strings.Contains(content, "1.") || strings.Contains(content, "- ") ||
		strings.Contains(content, "Step ") || strings.Contains(content, "First") {
		score++
	}

	// Has explanatory text (at least a few sentences)
	checks++
	sentences := strings.Count(content, ".") + strings.Count(content, "!") + strings.Count(content, "?")
	if sentences >= 3 {
		score++
	}

	if checks == 0 {
		return 0.0
	}
	return score / checks
}

// PairwiseSimilarity computes the Jaccard similarity between two strings
// based on their word sets.
func PairwiseSimilarity(a, b string) float64 {
	if a == "" && b == "" {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}

	wordsA := wordSet(a)
	wordsB := wordSet(b)

	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}

	union := len(wordsA) + len(wordsB) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// FormatConsensus produces a human-readable summary of the consensus result.
func FormatConsensus(result *ConsensusResult) string {
	if result == nil {
		return "No consensus result"
	}

	var sb strings.Builder

	strategyName := result.Method
	switch strategyName {
	case "majority":
		strategyName = "majority vote"
	case "best_score":
		strategyName = "best score"
	}

	sb.WriteString(fmt.Sprintf("Consensus (%d samples, %s):\n",
		len(result.AllSamples), strategyName))

	if result.Winner != nil {
		sb.WriteString(fmt.Sprintf("Winner: Sample #%d (score: %.2f, agreement: %d%%)\n",
			result.Winner.ID,
			result.Winner.Score,
			int(math.Round(result.Agreement*100))))
	}

	sb.WriteString("\n")

	for _, s := range result.AllSamples {
		summary := summarizeSample(s.Content)
		marker := ""
		if result.Winner != nil && s.ID == result.Winner.ID {
			marker = " ← selected"
		}
		sb.WriteString(fmt.Sprintf("Sample #%d: %.2f — %s%s\n",
			s.ID, s.Score, summary, marker))
	}

	return sb.String()
}

// --- internal helpers ---

func wordSet(s string) map[string]bool {
	words := strings.Fields(strings.ToLower(s))
	set := make(map[string]bool, len(words))
	for _, w := range words {
		// Strip punctuation
		w = strings.Trim(w, ".,;:!?\"'`()[]{}#")
		if w != "" {
			set[w] = true
		}
	}
	return set
}

func normalizeKey(s string) string {
	words := strings.Fields(strings.ToLower(s))
	if len(words) > 6 {
		words = words[:6]
	}
	return strings.Join(words, " ")
}

func estimateTokens(content string) int {
	return tok.EstimateTokens(content)
}

func calculateAgreement(samples []Sample, winner *Sample) float64 {
	if winner == nil || len(samples) <= 1 {
		return 1.0
	}

	totalSim := 0.0
	count := 0
	for _, s := range samples {
		if s.ID == winner.ID {
			continue
		}
		totalSim += PairwiseSimilarity(winner.Content, s.Content)
		count++
	}

	if count == 0 {
		return 1.0
	}
	return totalSim / float64(count)
}

func summarizeSample(content string) string {
	// Take first meaningful line as summary
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "```" {
			continue
		}
		// Truncate if too long
		if len(trimmed) > 50 {
			trimmed = trimmed[:47] + "..."
		}
		return trimmed
	}
	if len(content) > 50 {
		return content[:47] + "..."
	}
	return content
}
