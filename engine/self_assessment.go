package engine

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// Assessment captures the result of a self-evaluation after completing a task.
type Assessment struct {
	Score        float64            // overall score 0.0 - 1.0
	Dimensions   map[string]float64 // per-dimension scores
	Strengths    []string           // things that went well
	Weaknesses   []string           // things that went poorly
	Improvements []string           // actionable suggestions for next time
	TaskType     string             // classification of the task
	Timestamp    time.Time          // when the assessment was made
}

// SelfAssessor evaluates agent performance after each task and tracks trends.
type SelfAssessor struct {
	History []Assessment
	mu      sync.RWMutex
}

// TaskContext captures all relevant metrics from a completed task for assessment.
type TaskContext struct {
	Goal          string
	ToolCalls     int
	Errors        int
	Retries       int
	FilesModified int
	TestsPassed   bool
	Duration      time.Duration
	TokensUsed   int
	UserFeedback  string
}

// NewSelfAssessor creates a new SelfAssessor with an empty history.
func NewSelfAssessor() *SelfAssessor {
	return &SelfAssessor{
		History: make([]Assessment, 0),
	}
}

// Assess evaluates the agent's performance on a completed task across multiple
// dimensions and records the assessment in history.
func (sa *SelfAssessor) Assess(ctx TaskContext) *Assessment {
	dims := make(map[string]float64)

	dims["efficiency"] = sa.scoreEfficiency(ctx)
	dims["accuracy"] = sa.scoreAccuracy(ctx)
	dims["completeness"] = sa.scoreCompleteness(ctx)
	dims["speed"] = sa.scoreSpeed(ctx)
	dims["safety"] = sa.scoreSafety(ctx)

	// Overall score is the weighted average of dimensions.
	overall := 0.0
	weights := map[string]float64{
		"efficiency":   0.20,
		"accuracy":     0.25,
		"completeness": 0.25,
		"speed":        0.15,
		"safety":       0.15,
	}
	for dim, w := range weights {
		overall += dims[dim] * w
	}

	a := &Assessment{
		Score:        math.Round(overall*100) / 100,
		Dimensions:   dims,
		Strengths:    sa.IdentifyStrengths(ctx),
		Weaknesses:   sa.IdentifyWeaknesses(ctx),
		Improvements: sa.SuggestImprovements(ctx),
		TaskType:     classifyTask(ctx),
		Timestamp:    time.Now(),
	}

	sa.mu.Lock()
	sa.History = append(sa.History, *a)
	sa.mu.Unlock()

	return a
}

// scoreEfficiency evaluates token usage and tool call efficiency.
func (sa *SelfAssessor) scoreEfficiency(ctx TaskContext) float64 {
	score := 1.0

	// Penalize excessive tool calls relative to files modified.
	expectedCalls := ctx.FilesModified*3 + 2
	if expectedCalls < 3 {
		expectedCalls = 3
	}
	if ctx.ToolCalls > expectedCalls {
		excess := float64(ctx.ToolCalls-expectedCalls) / float64(expectedCalls)
		score -= excess * 0.3
	}

	// Penalize high token usage (over 4000 tokens per file modified).
	filesBase := ctx.FilesModified
	if filesBase < 1 {
		filesBase = 1
	}
	tokensPerFile := ctx.TokensUsed / filesBase
	if tokensPerFile > 4000 {
		penalty := float64(tokensPerFile-4000) / 8000.0
		score -= penalty
	}

	return clampScore(score)
}

// scoreAccuracy evaluates error rate and retries needed.
func (sa *SelfAssessor) scoreAccuracy(ctx TaskContext) float64 {
	score := 1.0

	// Each error costs 0.15.
	score -= float64(ctx.Errors) * 0.15

	// Each retry costs 0.10.
	score -= float64(ctx.Retries) * 0.10

	return clampScore(score)
}

// scoreCompleteness evaluates whether the goal was achieved.
func (sa *SelfAssessor) scoreCompleteness(ctx TaskContext) float64 {
	score := 0.5 // base: goal attempted

	if ctx.TestsPassed {
		score += 0.35
	}

	if ctx.Errors == 0 {
		score += 0.15
	}

	// Positive user feedback boosts completeness.
	if ctx.UserFeedback == "positive" {
		score = 1.0
	} else if ctx.UserFeedback == "negative" {
		score *= 0.5
	}

	return clampScore(score)
}

// scoreSpeed evaluates duration relative to task complexity.
func (sa *SelfAssessor) scoreSpeed(ctx TaskContext) float64 {
	// Estimate expected duration based on complexity signals.
	complexity := ctx.FilesModified + ctx.ToolCalls/2
	if complexity < 1 {
		complexity = 1
	}
	expectedSeconds := float64(complexity) * 10.0

	actualSeconds := ctx.Duration.Seconds()
	if actualSeconds <= 0 {
		return 1.0
	}

	ratio := actualSeconds / expectedSeconds
	if ratio <= 1.0 {
		return 1.0
	}
	// Penalize proportionally for taking longer than expected.
	score := 1.0 - (ratio-1.0)*0.25
	return clampScore(score)
}

// scoreSafety evaluates whether dangerous operations occurred.
func (sa *SelfAssessor) scoreSafety(ctx TaskContext) float64 {
	// Without explicit signals of dangerous ops, default to perfect.
	// Errors might indicate attempted dangerous operations.
	if ctx.Errors > 3 {
		return 0.85
	}
	return 1.0
}

// IdentifyStrengths returns a list of things that went well.
func (sa *SelfAssessor) IdentifyStrengths(ctx TaskContext) []string {
	var strengths []string

	if ctx.Retries == 0 {
		strengths = append(strengths, "Completed in single attempt")
	}

	filesBase := ctx.FilesModified
	if filesBase < 1 {
		filesBase = 1
	}
	tokensPerFile := ctx.TokensUsed / filesBase
	if tokensPerFile < 2000 {
		strengths = append(strengths, "Low token usage for complexity")
	}

	if ctx.TestsPassed {
		strengths = append(strengths, "All tests passing")
	}

	if ctx.Errors == 0 {
		strengths = append(strengths, "No errors encountered")
	}

	if ctx.Duration > 0 && ctx.Duration < 30*time.Second && ctx.FilesModified > 0 {
		strengths = append(strengths, "Fast completion")
	}

	return strengths
}

// IdentifyWeaknesses returns a list of things that went poorly.
func (sa *SelfAssessor) IdentifyWeaknesses(ctx TaskContext) []string {
	var weaknesses []string

	if ctx.Retries >= 3 {
		weaknesses = append(weaknesses, fmt.Sprintf("High retry count (%d retries)", ctx.Retries))
	}

	expectedCalls := ctx.FilesModified*3 + 2
	if expectedCalls < 5 {
		expectedCalls = 5
	}
	if ctx.ToolCalls > expectedCalls*2 {
		weaknesses = append(weaknesses, fmt.Sprintf("Excessive tool calls (%d for simple task)", ctx.ToolCalls))
	}

	// Check if duration seems too long for the complexity.
	complexity := ctx.FilesModified + ctx.ToolCalls/2
	if complexity < 1 {
		complexity = 1
	}
	expectedDuration := time.Duration(complexity*10) * time.Second
	if ctx.Duration > expectedDuration*2 {
		weaknesses = append(weaknesses, "Took longer than expected")
	}

	filesBase := ctx.FilesModified
	if filesBase < 1 {
		filesBase = 1
	}
	tokensPerFile := ctx.TokensUsed / filesBase
	if tokensPerFile > 5000 {
		weaknesses = append(weaknesses, "High token usage")
	}

	if ctx.Errors > 2 {
		weaknesses = append(weaknesses, fmt.Sprintf("Multiple errors (%d)", ctx.Errors))
	}

	return weaknesses
}

// SuggestImprovements returns actionable suggestions for future tasks.
func (sa *SelfAssessor) SuggestImprovements(ctx TaskContext) []string {
	var improvements []string

	if ctx.Errors > 0 && ctx.FilesModified > 0 {
		improvements = append(improvements, "Read files before editing to reduce failures")
	}

	if ctx.ToolCalls > 10 && ctx.FilesModified <= 2 {
		improvements = append(improvements, "Use grep to find symbols instead of reading multiple files")
	}

	if !ctx.TestsPassed && ctx.FilesModified > 0 {
		improvements = append(improvements, "Consider running tests earlier in the workflow")
	}

	if ctx.Retries > 1 {
		improvements = append(improvements, "Validate approach before implementation to reduce retries")
	}

	filesBase := ctx.FilesModified
	if filesBase < 1 {
		filesBase = 1
	}
	tokensPerFile := ctx.TokensUsed / filesBase
	if tokensPerFile > 5000 {
		improvements = append(improvements, "Be more concise in responses to reduce token usage")
	}

	if ctx.Duration > 2*time.Minute && ctx.FilesModified <= 1 {
		improvements = append(improvements, "For simple edits, act immediately instead of over-analyzing")
	}

	return improvements
}

// GetTrend analyzes the trend of a given dimension over the last 10 assessments.
// Returns "improving", "stable", or "declining".
func (sa *SelfAssessor) GetTrend(dimension string) string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	n := len(sa.History)
	if n < 3 {
		return "stable"
	}

	// Look at last 10 assessments (or all if fewer than 10).
	start := 0
	if n > 10 {
		start = n - 10
	}
	recent := sa.History[start:]

	// Compare average of first half to second half.
	mid := len(recent) / 2
	firstHalf := recent[:mid]
	secondHalf := recent[mid:]

	firstAvg := avgDimension(firstHalf, dimension)
	secondAvg := avgDimension(secondHalf, dimension)

	diff := secondAvg - firstAvg
	if diff > 0.05 {
		return "improving"
	} else if diff < -0.05 {
		return "declining"
	}
	return "stable"
}

// FormatSelfAssessment produces a human-readable summary of an assessment.
func FormatSelfAssessment(a *Assessment) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Self-Assessment: %.2f/1.00\n", a.Score))
	sb.WriteString("───────────────────────────────\n")

	order := []string{"efficiency", "accuracy", "completeness", "speed", "safety"}
	labels := map[string]string{
		"efficiency":   "Efficiency:  ",
		"accuracy":     "Accuracy:    ",
		"completeness": "Completeness:",
		"speed":        "Speed:       ",
		"safety":       "Safety:      ",
	}

	for _, dim := range order {
		score := a.Dimensions[dim]
		bar := assessmentBar(score)
		sb.WriteString(fmt.Sprintf("%s %.2f  %s\n", labels[dim], score, bar))
	}

	if len(a.Strengths) > 0 {
		sb.WriteString(fmt.Sprintf("\nStrengths: %s\n", strings.Join(a.Strengths, ", ")))
	}
	if len(a.Weaknesses) > 0 {
		sb.WriteString(fmt.Sprintf("Weaknesses: %s\n", strings.Join(a.Weaknesses, ", ")))
	}
	if len(a.Improvements) > 0 {
		sb.WriteString(fmt.Sprintf("Improvements: %s\n", strings.Join(a.Improvements, ", ")))
	}

	return sb.String()
}

// AverageScore computes the average overall score of the last n assessments.
// If n is 0 or exceeds history length, all assessments are averaged.
func (sa *SelfAssessor) AverageScore(n int) float64 {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	total := len(sa.History)
	if total == 0 {
		return 0.0
	}

	if n <= 0 || n > total {
		n = total
	}

	sum := 0.0
	start := total - n
	for i := start; i < total; i++ {
		sum += sa.History[i].Score
	}

	return math.Round((sum/float64(n))*100) / 100
}

// assessmentBar creates a visual bar chart using block characters (10 chars wide).
func assessmentBar(score float64) string {
	filled := int(math.Round(score * 10))
	if filled > 10 {
		filled = 10
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", 10-filled)
}

// clampScore restricts a score to the range [0.0, 1.0].
func clampScore(score float64) float64 {
	if score < 0.0 {
		return 0.0
	}
	if score > 1.0 {
		return 1.0
	}
	return math.Round(score*100) / 100
}

// classifyTask determines the task type from context signals.
func classifyTask(ctx TaskContext) string {
	if ctx.FilesModified == 0 {
		return "research"
	}
	if ctx.TestsPassed && ctx.FilesModified > 3 {
		return "feature"
	}
	if ctx.FilesModified == 1 && ctx.ToolCalls < 5 {
		return "quick-fix"
	}
	return "edit"
}

// avgDimension computes the average of a dimension score across assessments.
func avgDimension(assessments []Assessment, dimension string) float64 {
	if len(assessments) == 0 {
		return 0.0
	}
	sum := 0.0
	count := 0
	for _, a := range assessments {
		if v, ok := a.Dimensions[dimension]; ok {
			sum += v
			count++
		}
	}
	if count == 0 {
		return 0.0
	}
	return sum / float64(count)
}
