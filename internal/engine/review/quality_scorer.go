package review

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/mathutil"
)

// QualityScorer evaluates LLM response quality across multiple dimensions
// and provides feedback for the self-improvement loop.
type QualityScorer struct {
	Weights ScoreWeights
	History []ScoredResponse
	mu      sync.RWMutex
}

// ScoreWeights defines the relative importance of each quality dimension.
// All values should be in [0,1] and sum to 1.
type ScoreWeights struct {
	Completeness float64 // did it address the full request?
	Correctness  float64 // is the code syntactically valid?
	Conciseness  float64 // not overly verbose?
	ToolUsage    float64 // efficient use of tools?
	Safety       float64 // no dangerous operations?
}

// ScoredResponse holds the quality evaluation of a single LLM response.
type ScoredResponse struct {
	Score     float64            // 0-1 overall composite score
	Breakdown map[string]float64 // per-dimension scores
	Feedback  []string           // human-readable improvement suggestions
	Timestamp time.Time
	Model     string
	TaskType  string
}

// ResponseContext provides the context needed to evaluate a response's quality.
type ResponseContext struct {
	UserPrompt        string
	AssistantResponse string
	ToolCallCount     int
	ToolErrors        int
	FilesModified     []string
	TestsPassed       bool
	LintPassed        bool
	TokensUsed        int
	Duration          time.Duration
}

// DefaultWeights returns a balanced set of scoring weights.
func DefaultWeights() ScoreWeights {
	return ScoreWeights{
		Completeness: 0.30,
		Correctness:  0.30,
		Conciseness:  0.15,
		ToolUsage:    0.15,
		Safety:       0.10,
	}
}

// NewQualityScorer creates a QualityScorer with default weights.
func NewQualityScorer() *QualityScorer {
	return &QualityScorer{
		Weights: DefaultWeights(),
		History: make([]ScoredResponse, 0),
	}
}

// Score evaluates a response across all quality dimensions and returns a composite result.
func (qs *QualityScorer) Score(ctx ResponseContext) *ScoredResponse {
	completeness := qs.scoreCompleteness(ctx)
	correctness := qs.scoreCorrectness(ctx)
	conciseness := qs.scoreConciseness(ctx)
	toolUsage := qs.scoreToolUsage(ctx)
	safety := qs.scoreSafety(ctx)

	composite := qs.Weights.Completeness*completeness +
		qs.Weights.Correctness*correctness +
		qs.Weights.Conciseness*conciseness +
		qs.Weights.ToolUsage*toolUsage +
		qs.Weights.Safety*safety

	scored := &ScoredResponse{
		Score: clampFloat(composite, 0, 1),
		Breakdown: map[string]float64{
			"completeness": completeness,
			"correctness":  correctness,
			"conciseness":  conciseness,
			"tool_usage":   toolUsage,
			"safety":       safety,
		},
		Feedback:  qs.GenerateFeedback(&ScoredResponse{Score: composite, Breakdown: map[string]float64{"completeness": completeness, "correctness": correctness, "conciseness": conciseness, "tool_usage": toolUsage, "safety": safety}}),
		Timestamp: time.Now(),
	}

	qs.mu.Lock()
	qs.History = append(qs.History, *scored)
	qs.mu.Unlock()

	return scored
}

// scoreCompleteness evaluates whether the response fully addressed the request.
func (qs *QualityScorer) scoreCompleteness(ctx ResponseContext) float64 {
	if len(ctx.AssistantResponse) == 0 {
		return 0.0
	}

	score := 0.0

	// Response length relative to prompt complexity
	promptLen := len(ctx.UserPrompt)
	responseLen := len(ctx.AssistantResponse)

	if promptLen == 0 {
		promptLen = 1
	}

	// A reasonable response should be at least as long as the prompt for complex tasks
	ratio := float64(responseLen) / float64(promptLen)
	if ratio >= 1.0 {
		score += 0.4
	} else if ratio >= 0.5 {
		score += 0.3
	} else if ratio >= 0.2 {
		score += 0.2
	} else {
		score += 0.1
	}

	// Tool calls made (did it actually do something?)
	if ctx.ToolCallCount > 0 {
		score += 0.3
	}

	// Files modified (for coding tasks)
	if len(ctx.FilesModified) > 0 {
		score += 0.3
	} else if ctx.ToolCallCount > 0 {
		// Some tool calls but no files modified is acceptable for non-coding tasks
		score += 0.15
	}

	return clampFloat(score, 0, 1)
}

// scoreCorrectness evaluates whether the output is syntactically and logically valid.
func (qs *QualityScorer) scoreCorrectness(ctx ResponseContext) float64 {
	score := 0.5 // baseline

	// Tests passed is a strong signal of correctness
	if ctx.TestsPassed {
		score = 1.0
	}

	// Lint passed gives a bonus
	if ctx.LintPassed {
		score = math.Min(score+0.15, 1.0)
	}

	// Tool errors are a penalty
	if ctx.ToolCallCount > 0 && ctx.ToolErrors > 0 {
		errorRate := float64(ctx.ToolErrors) / float64(ctx.ToolCallCount)
		score -= errorRate * 0.4
	}

	// Check for balanced braces in code blocks
	if hasUnbalancedBraces(ctx.AssistantResponse) {
		score -= 0.2
	}

	return clampFloat(score, 0, 1)
}

// scoreConciseness evaluates whether the response avoids unnecessary verbosity.
func (qs *QualityScorer) scoreConciseness(ctx ResponseContext) float64 {
	if ctx.TokensUsed == 0 {
		return 0.8 // no token data, assume reasonable
	}

	// Estimate task complexity from prompt length
	promptWords := len(strings.Fields(ctx.UserPrompt))
	if promptWords == 0 {
		promptWords = 1
	}

	// Ideal response token ratio: roughly 3-10x the prompt word count
	idealMax := promptWords * 10
	idealMin := promptWords * 2

	if ctx.TokensUsed <= idealMax && ctx.TokensUsed >= idealMin {
		return 1.0
	}

	if ctx.TokensUsed < idealMin {
		// Too short might indicate incompleteness, but still concise
		return 0.85
	}

	// Penalize extremely long responses
	overageRatio := float64(ctx.TokensUsed) / float64(idealMax)
	if overageRatio > 5.0 {
		return 0.3
	}
	if overageRatio > 3.0 {
		return 0.5
	}
	if overageRatio > 2.0 {
		return 0.65
	}

	return 0.75
}

// scoreToolUsage evaluates efficiency and correctness of tool use.
func (qs *QualityScorer) scoreToolUsage(ctx ResponseContext) float64 {
	// No tools used — neutral if no files modified, slight penalty if files were expected
	if ctx.ToolCallCount == 0 {
		if len(ctx.FilesModified) == 0 {
			return 0.7 // might be a conversational response
		}
		return 0.5 // files modified without tool calls is unusual
	}

	score := 0.8 // baseline for using tools

	// Excessive tool calls: penalty for more than 15
	if ctx.ToolCallCount > 15 {
		excess := float64(ctx.ToolCallCount-15) / 15.0
		score -= math.Min(excess*0.2, 0.3)
	}

	// Failed tool calls ratio
	if ctx.ToolErrors > 0 {
		errorRate := float64(ctx.ToolErrors) / float64(ctx.ToolCallCount)
		score -= errorRate * 0.4
	}

	// Good tool/file ratio: reading before writing (only if error rate is low)
	errorRate := float64(ctx.ToolErrors) / float64(ctx.ToolCallCount)
	if len(ctx.FilesModified) > 0 && ctx.ToolCallCount >= len(ctx.FilesModified)*2 && errorRate < 0.2 {
		// Likely read before write pattern
		score += 0.1
	}

	return clampFloat(score, 0, 1)
}

// scoreSafety evaluates whether the response avoids dangerous operations.
func (qs *QualityScorer) scoreSafety(ctx ResponseContext) float64 {
	score := 1.0

	response := strings.ToLower(ctx.AssistantResponse)

	// Check for dangerous patterns
	dangerousPatterns := []string{
		"rm -rf /",
		"rm -rf ~",
		"chmod 777",
		"curl | bash",
		"curl | sh",
		"wget | bash",
		"--force",
		"drop database",
		"drop table",
		"truncate table",
		"> /dev/sda",
		"mkfs.",
		"dd if=",
		":(){:|:&};:",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(response, pattern) {
			score -= 0.3
		}
	}

	// Check for potential secret leaks
	secretPatterns := []string{
		"api_key",
		"api_secret",
		"password=",
		"secret_key",
		"private_key",
		"access_token",
		"bearer ",
	}

	for _, pattern := range secretPatterns {
		if strings.Contains(response, pattern) {
			// Only penalize if it looks like an actual value, not just mentioning the concept
			idx := strings.Index(response, pattern)
			if idx >= 0 && idx+len(pattern) < len(response) {
				after := response[idx+len(pattern):]
				if len(after) > 0 && (after[0] == '=' || after[0] == ':' || after[0] == '"') {
					score -= 0.15
				}
			}
		}
	}

	return clampFloat(score, 0, 1)
}

// GenerateFeedback produces human-readable suggestions based on the scored response.
func (qs *QualityScorer) GenerateFeedback(scored *ScoredResponse) []string {
	var feedback []string

	if scored.Breakdown["completeness"] >= 0.9 {
		feedback = append(feedback, fmt.Sprintf("Response was complete and thorough (score: %.2f)", scored.Breakdown["completeness"]))
	} else if scored.Breakdown["completeness"] < 0.5 {
		feedback = append(feedback, "Response may not fully address the request — consider providing more detail")
	}

	if scored.Breakdown["correctness"] >= 0.9 {
		feedback = append(feedback, fmt.Sprintf("Code correctness is excellent (score: %.2f)", scored.Breakdown["correctness"]))
	} else if scored.Breakdown["correctness"] < 0.5 {
		feedback = append(feedback, "Correctness issues detected — check for syntax errors and test failures")
	}

	if scored.Breakdown["conciseness"] >= 0.9 {
		feedback = append(feedback, fmt.Sprintf("Response was concise and effective (score: %.2f)", scored.Breakdown["conciseness"]))
	} else if scored.Breakdown["conciseness"] < 0.5 {
		feedback = append(feedback, "Response was overly verbose — try to be more concise")
	}

	if scored.Breakdown["tool_usage"] >= 0.9 {
		feedback = append(feedback, fmt.Sprintf("Efficient tool usage (score: %.2f)", scored.Breakdown["tool_usage"]))
	} else if scored.Breakdown["tool_usage"] < 0.5 {
		feedback = append(feedback, "Consider reading files before editing next time")
	}

	if scored.Breakdown["safety"] < 0.8 {
		feedback = append(feedback, "Safety concerns detected — avoid dangerous operations and secret exposure")
	}

	if scored.Score >= 0.9 {
		feedback = append(feedback, fmt.Sprintf("Overall excellent quality (%.2f/1.00)", scored.Score))
	} else if scored.Score < 0.5 {
		feedback = append(feedback, fmt.Sprintf("Overall quality needs improvement (%.2f/1.00)", scored.Score))
	}

	return feedback
}

// AverageScore computes the average composite score over the last n responses.
func (qs *QualityScorer) AverageScore(n int) float64 {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	if len(qs.History) == 0 {
		return 0.0
	}

	start := len(qs.History) - n
	if start < 0 {
		start = 0
	}

	var sum float64
	count := 0
	for i := start; i < len(qs.History); i++ {
		sum += qs.History[i].Score
		count++
	}

	if count == 0 {
		return 0.0
	}

	return sum / float64(count)
}

// TrendAnalysis returns a human-readable description of quality trends.
func (qs *QualityScorer) TrendAnalysis() string {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	if len(qs.History) < 4 {
		return "Insufficient data for trend analysis (need at least 4 responses)"
	}

	// Split history into two halves and compare averages
	mid := len(qs.History) / 2
	var firstHalf, secondHalf float64
	for i := 0; i < mid; i++ {
		firstHalf += qs.History[i].Score
	}
	for i := mid; i < len(qs.History); i++ {
		secondHalf += qs.History[i].Score
	}

	firstAvg := firstHalf / float64(mid)
	secondAvg := secondHalf / float64(len(qs.History)-mid)
	diff := secondAvg - firstAvg

	var trend string
	if diff > 0.02 {
		trend = fmt.Sprintf("Quality improving: +%.2f over last %d sessions", diff, len(qs.History))
	} else if diff < -0.02 {
		trend = fmt.Sprintf("Quality declining: %.2f over last %d sessions", diff, len(qs.History))
	} else {
		trend = fmt.Sprintf("Quality stable (%.2f) over last %d sessions", secondAvg, len(qs.History))
	}

	// Check for dimension-specific trends
	if len(qs.History) >= 4 {
		dimensions := []string{"completeness", "correctness", "conciseness", "tool_usage", "safety"}
		for _, dim := range dimensions {
			var firstDim, secondDim float64
			for i := 0; i < mid; i++ {
				if v, ok := qs.History[i].Breakdown[dim]; ok {
					firstDim += v
				}
			}
			for i := mid; i < len(qs.History); i++ {
				if v, ok := qs.History[i].Breakdown[dim]; ok {
					secondDim += v
				}
			}
			firstDimAvg := firstDim / float64(mid)
			secondDimAvg := secondDim / float64(len(qs.History)-mid)
			dimDiff := secondDimAvg - firstDimAvg

			if dimDiff < -0.05 {
				trend += fmt.Sprintf("; %s declining (%.2f)", dim, dimDiff)
			}
		}
	}

	return trend
}

// FormatReport generates a formatted quality report for the last n responses.
func (qs *QualityScorer) FormatReport(last int) string {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	if len(qs.History) == 0 {
		return "No responses scored yet."
	}

	start := len(qs.History) - last
	if start < 0 {
		start = 0
	}

	actual := len(qs.History) - start

	// Calculate averages
	var totalScore float64
	dims := map[string]float64{
		"completeness": 0,
		"correctness":  0,
		"conciseness":  0,
		"tool_usage":   0,
		"safety":       0,
	}

	for i := start; i < len(qs.History); i++ {
		totalScore += qs.History[i].Score
		for k, v := range qs.History[i].Breakdown {
			dims[k] += v
		}
	}

	avgScore := totalScore / float64(actual)
	for k := range dims {
		dims[k] /= float64(actual)
	}

	// Trend calculation
	var trendStr string
	if actual >= 4 {
		mid := start + actual/2
		var first, second float64
		var firstCount, secondCount int
		for i := start; i < mid; i++ {
			first += qs.History[i].Score
			firstCount++
		}
		for i := mid; i < len(qs.History); i++ {
			second += qs.History[i].Score
			secondCount++
		}
		firstAvg := first / float64(firstCount)
		secondAvg := second / float64(secondCount)
		diff := secondAvg - firstAvg
		if diff > 0.01 {
			trendStr = fmt.Sprintf("improving (+%.2f)", diff)
		} else if diff < -0.01 {
			trendStr = fmt.Sprintf("declining (%.2f)", diff)
		} else {
			trendStr = "stable"
		}
	} else {
		trendStr = "insufficient data"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Quality Report (last %d responses):\n", actual))
	b.WriteString(fmt.Sprintf("Average: %.2f/1.00\n", avgScore))
	b.WriteString(fmt.Sprintf("Trend: %s\n", trendStr))
	b.WriteString("\nBreakdown:\n")

	// Format each dimension with a bar
	dimOrder := []string{"completeness", "correctness", "conciseness", "tool_usage", "safety"}
	dimLabels := map[string]string{
		"completeness": "Completeness",
		"correctness":  "Correctness",
		"conciseness":  "Conciseness",
		"tool_usage":   "Tool Usage",
		"safety":       "Safety",
	}

	for _, dim := range dimOrder {
		label := dimLabels[dim]
		val := dims[dim]
		bar := renderBar(val, 20)
		b.WriteString(fmt.Sprintf("  %-13s %.2f %s\n", label+":", val, bar))
	}

	// Collect recent feedback
	var allFeedback []string
	seen := make(map[string]bool)
	for i := len(qs.History) - 1; i >= start; i-- {
		for _, fb := range qs.History[i].Feedback {
			if !seen[fb] {
				seen[fb] = true
				allFeedback = append(allFeedback, fb)
			}
		}
		if len(allFeedback) >= 5 {
			break
		}
	}

	if len(allFeedback) > 0 {
		b.WriteString("\nFeedback:\n")
		for _, fb := range allFeedback {
			b.WriteString(fmt.Sprintf("- %s\n", fb))
		}
	}

	return b.String()
}

// renderBar creates a visual bar representation of a 0-1 value.
func renderBar(value float64, width int) string {
	filled := int(math.Round(value * float64(width)))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("#", filled) + strings.Repeat(".", width-filled)
}

// hasUnbalancedBraces checks if code blocks in the response have balanced braces.
func hasUnbalancedBraces(response string) bool {
	// Extract code blocks
	inCodeBlock := false
	var codeContent strings.Builder

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				// End of code block, check balance
				code := codeContent.String()
				if !bracesBalanced(code) {
					return true
				}
				codeContent.Reset()
			}
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			codeContent.WriteString(line)
			codeContent.WriteString("\n")
		}
	}

	return false
}

// bracesBalanced checks if curly braces, parens, and brackets are balanced.
func bracesBalanced(code string) bool {
	var stack []rune
	openers := map[rune]rune{')': '(', ']': '[', '}': '{'}

	for _, ch := range code {
		switch ch {
		case '(', '[', '{':
			stack = append(stack, ch)
		case ')', ']', '}':
			if len(stack) == 0 {
				return false
			}
			expected := openers[ch]
			if stack[len(stack)-1] != expected {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}

	return len(stack) == 0
}

// clampFloat restricts a float64 value to the [min, max] range.
func clampFloat(v, min, max float64) float64 {
	return mathutil.Clamp(v, min, max)
}
