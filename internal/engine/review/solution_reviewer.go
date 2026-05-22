package review

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// SolutionReviewer implements a multi-attempt solution review pattern inspired
// by SWE-agent's reviewer: run the agent N times, score each solution, and
// select the best one. This improves reliability by sampling multiple approaches.
type SolutionReviewer struct {
	MaxAttempts int
	ScoreFn     func(solution string) float64
	mu          sync.Mutex
}

// Solution represents a single attempted solution with metadata.
type Solution struct {
	ID            int
	Content       string
	Score         float64
	Duration      time.Duration
	TokensUsed    int
	Errors        []string
	FilesModified []string
}

// ReviewResult holds the outcome of the multi-attempt review process.
type ReviewResult struct {
	Best          *Solution
	All           []Solution
	Attempts      int
	TotalDuration time.Duration
	TotalTokens   int
	Agreement     float64
}

// NewSolutionReviewer creates a SolutionReviewer with the given max attempts.
// If maxAttempts is <= 0, it defaults to 3.
func NewSolutionReviewer(maxAttempts int) *SolutionReviewer {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &SolutionReviewer{
		MaxAttempts: maxAttempts,
		ScoreFn:     nil, // uses default scoring via ScoreSolution
	}
}

// ReviewAndSelect runs solveFn up to MaxAttempts times, scores each solution,
// selects the best one, and calculates agreement across attempts.
func (sr *SolutionReviewer) ReviewAndSelect(ctx context.Context, task string, solveFn func(context.Context, string) (*Solution, error)) (*ReviewResult, error) {
	sr.mu.Lock()
	maxAttempts := sr.MaxAttempts
	sr.mu.Unlock()

	var solutions []Solution
	var totalDuration time.Duration
	var totalTokens int

	for i := 0; i < maxAttempts; i++ {
		select {
		case <-ctx.Done():
			if len(solutions) == 0 {
				return nil, ctx.Err()
			}
			goto selectBest
		default:
		}

		start := time.Now()
		sol, err := solveFn(ctx, task)
		elapsed := time.Since(start)

		if err != nil {
			// Record as a failed attempt
			solutions = append(solutions, Solution{
				ID:       i + 1,
				Content:  "",
				Score:    0,
				Duration: elapsed,
				Errors:   []string{err.Error()},
			})
			totalDuration += elapsed
			continue
		}

		sol.ID = i + 1
		if sol.Duration == 0 {
			sol.Duration = elapsed
		}
		sol.Score = sr.ScoreSolution(sol)
		totalDuration += sol.Duration
		totalTokens += sol.TokensUsed
		solutions = append(solutions, *sol)

		// If we get a very high score early, no need to continue
		if sol.Score >= 0.95 {
			break
		}

		// Check if we should retry
		if !ShouldRetry(solutions) && i > 0 {
			break
		}
	}

selectBest:
	if len(solutions) == 0 {
		return nil, fmt.Errorf("no solutions generated")
	}

	// Find the best solution
	bestIdx := 0
	for i := 1; i < len(solutions); i++ {
		if solutions[i].Score > solutions[bestIdx].Score {
			bestIdx = i
		}
	}

	best := solutions[bestIdx]
	agreement := calculateSolutionAgreement(solutions)

	return &ReviewResult{
		Best:          &best,
		All:           solutions,
		Attempts:      len(solutions),
		TotalDuration: totalDuration,
		TotalTokens:   totalTokens,
		Agreement:     agreement,
	}, nil
}

// ScoreSolution evaluates a solution using default scoring criteria:
//   - Has code changes (+0.3)
//   - No errors (+0.3)
//   - Reasonable length (+0.2)
//   - Files modified (+0.2)
func (sr *SolutionReviewer) ScoreSolution(solution *Solution) float64 {
	sr.mu.Lock()
	customFn := sr.ScoreFn
	sr.mu.Unlock()

	if customFn != nil {
		return customFn(solution.Content)
	}

	return defaultScoreSolution(solution)
}

// defaultScoreSolution applies the default scoring heuristics.
func defaultScoreSolution(solution *Solution) float64 {
	if solution == nil {
		return 0.0
	}

	score := 0.0

	// Has code changes (+0.3)
	if hasCodeChanges(solution.Content) {
		score += 0.3
	}

	// No errors (+0.3)
	if len(solution.Errors) == 0 {
		score += 0.3
	}

	// Reasonable length (+0.2)
	score += reasonableLengthScore(solution.Content) * 0.2

	// Files modified (+0.2)
	if len(solution.FilesModified) > 0 {
		score += 0.2
	}

	return score
}

// hasCodeChanges checks whether the solution content contains code modifications.
func hasCodeChanges(content string) bool {
	if content == "" {
		return false
	}
	codeIndicators := []string{
		"```", "diff --", "+++", "---",
		"func ", "def ", "class ", "import ",
		"const ", "var ", "let ", "return ",
	}
	for _, indicator := range codeIndicators {
		if strings.Contains(content, indicator) {
			return true
		}
	}
	return false
}

// reasonableLengthScore returns a score from 0 to 1 based on content length.
// Too short or excessively long content is penalized.
func reasonableLengthScore(content string) float64 {
	length := len(content)
	if length == 0 {
		return 0.0
	}

	const idealMin = 100
	const idealMax = 5000

	if length >= idealMin && length <= idealMax {
		return 1.0
	}

	if length < idealMin {
		return float64(length) / float64(idealMin)
	}

	// Gradually penalize very long content
	excess := float64(length-idealMax) / float64(idealMax)
	s := 1.0 - (excess * 0.2)
	if s < 0.3 {
		return 0.3
	}
	return s
}

// CompareApproaches analyzes how different attempts approached the problem
// and returns a human-readable comparison.
func CompareApproaches(solutions []Solution) string {
	if len(solutions) == 0 {
		return "No solutions to compare"
	}

	if len(solutions) == 1 {
		return "Only one attempt — no comparison available"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Approach Comparison (%d attempts):\n", len(solutions)))
	sb.WriteString(strings.Repeat("─", 40))
	sb.WriteString("\n")

	for _, sol := range solutions {
		summary := summarizeSolution(sol)
		sb.WriteString(fmt.Sprintf("#%d: %s\n", sol.ID, summary))
	}

	sb.WriteString("\n")

	// Pairwise similarity analysis
	if len(solutions) >= 2 {
		totalSim := 0.0
		pairs := 0
		for i := 0; i < len(solutions); i++ {
			for j := i + 1; j < len(solutions); j++ {
				sim := solutionSimilarity(solutions[i].Content, solutions[j].Content)
				totalSim += sim
				pairs++
			}
		}
		avgSim := totalSim / float64(pairs)
		if avgSim > 0.7 {
			sb.WriteString("Approaches are highly similar — converging on same solution\n")
		} else if avgSim > 0.4 {
			sb.WriteString("Approaches share common elements but differ in details\n")
		} else {
			sb.WriteString("Approaches are significantly different — varied strategies\n")
		}
	}

	return sb.String()
}

// FormatReview produces a formatted summary of the review result.
func FormatReview(result *ReviewResult) string {
	if result == nil {
		return "No review result"
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Solution Review (%d attempts):\n", result.Attempts))
	sb.WriteString("───────────────────────────────\n")

	for _, sol := range result.All {
		summary := summarizeSolution(sol)
		marker := ""
		if result.Best != nil && sol.ID == result.Best.ID {
			marker = " ← SELECTED"
		}
		sb.WriteString(fmt.Sprintf("#%d: Score %.2f — %s%s\n", sol.ID, sol.Score, summary, marker))
	}

	sb.WriteString("\n")

	agreementPct := int(math.Round(result.Agreement * 100))
	agreementDesc := "approaches varied"
	if result.Agreement > 0.7 {
		agreementDesc = "high convergence"
	} else if result.Agreement > 0.4 {
		agreementDesc = "moderate agreement"
	}
	sb.WriteString(fmt.Sprintf("Agreement: %d%% (%s)\n", agreementPct, agreementDesc))

	if result.Best != nil {
		sb.WriteString(fmt.Sprintf("Best: #%d (score: %.2f)\n", result.Best.ID, result.Best.Score))
	}

	return sb.String()
}

// ShouldRetry determines whether additional attempts should be made.
// Returns true if the best score so far is below 0.7.
func ShouldRetry(solutions []Solution) bool {
	if len(solutions) == 0 {
		return true
	}

	bestScore := 0.0
	for _, sol := range solutions {
		if sol.Score > bestScore {
			bestScore = sol.Score
		}
	}

	return bestScore < 0.7
}

// --- internal helpers ---

// calculateSolutionAgreement computes the average pairwise similarity
// across all solutions as a measure of how much they agree.
func calculateSolutionAgreement(solutions []Solution) float64 {
	if len(solutions) <= 1 {
		return 1.0
	}

	totalSim := 0.0
	pairs := 0

	for i := 0; i < len(solutions); i++ {
		for j := i + 1; j < len(solutions); j++ {
			totalSim += solutionSimilarity(solutions[i].Content, solutions[j].Content)
			pairs++
		}
	}

	if pairs == 0 {
		return 1.0
	}

	return totalSim / float64(pairs)
}

// solutionSimilarity computes Jaccard similarity between two solution contents.
func solutionSimilarity(a, b string) float64 {
	if a == "" && b == "" {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}

	wordsA := solutionWordSet(a)
	wordsB := solutionWordSet(b)

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

// solutionWordSet creates a word set from content for similarity comparison.
func solutionWordSet(s string) map[string]bool {
	words := strings.Fields(strings.ToLower(s))
	set := make(map[string]bool, len(words))
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'`()[]{}#")
		if w != "" {
			set[w] = true
		}
	}
	return set
}

// summarizeSolution produces a brief description of a solution for display.
func summarizeSolution(sol Solution) string {
	var parts []string

	if len(sol.Errors) > 0 {
		parts = append(parts, "had errors")
	}

	if len(sol.FilesModified) > 0 {
		parts = append(parts, fmt.Sprintf("%d file(s)", len(sol.FilesModified)))
	}

	if hasCodeChanges(sol.Content) {
		if len(sol.FilesModified) > 2 {
			parts = append(parts, "comprehensive")
		} else if len(sol.FilesModified) <= 1 && len(sol.Content) < 500 {
			parts = append(parts, "minimal fix")
		} else {
			parts = append(parts, "code changes")
		}
	}

	// Check if tests were included
	if strings.Contains(sol.Content, "test") || strings.Contains(sol.Content, "Test") {
		parts = append(parts, "tests added")
	}

	if sol.Content == "" && len(sol.Errors) == 0 {
		parts = append(parts, "empty")
	}

	if len(parts) == 0 {
		parts = append(parts, "solution provided")
	}

	return strings.Join(parts, ", ")
}
