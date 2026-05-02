package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ResearchConfig controls the autonomous research loop.
type ResearchConfig struct {
	TargetFile    string        // file to modify (e.g., "train.py")
	Metric        string        // command that outputs a score (e.g., "go test -bench .")
	Budget        time.Duration // time budget per iteration (default: 5 min)
	MaxIterations int           // max total iterations (default: 50)
	GoalPrompt    string        // what to optimize (from program.md or CLI arg)
	KeepBest      bool          // only keep changes that improve metric (default: true)
}

// DefaultResearchConfig returns sensible defaults for the research loop.
func DefaultResearchConfig() ResearchConfig {
	return ResearchConfig{
		Budget:        5 * time.Minute,
		MaxIterations: 50,
		KeepBest:      true,
	}
}

// ResearchResult tracks one iteration of the research loop.
type ResearchResult struct {
	Iteration   int
	Score       float64
	Improved    bool
	Description string
	Duration    time.Duration
}

// RunMetric executes the metric command and extracts a numeric score.
// It runs the command, captures stdout/stderr, and extracts the last float
// from the output.
// Returns: score, full output, error.
func RunMetric(command string, timeout time.Duration) (float64, string, error) {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	if err != nil {
		return 0, output, fmt.Errorf("metric command failed: %w\nOutput: %s", err, output)
	}

	score, parseErr := extractLastFloat(output)
	if parseErr != nil {
		return 0, output, fmt.Errorf("could not extract numeric score from output: %w", parseErr)
	}

	return score, output, nil
}

// extractLastFloat finds the last floating-point number in the given text.
func extractLastFloat(text string) (float64, error) {
	// Match integers and floats, including negative numbers and scientific notation
	re := regexp.MustCompile(`[-+]?[0-9]*\.?[0-9]+(?:[eE][-+]?[0-9]+)?`)
	matches := re.FindAllString(text, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("no numeric value found in output")
	}
	last := matches[len(matches)-1]
	return strconv.ParseFloat(last, 64)
}

// RunResearch executes the autonomous research loop.
//
// 1. Run metric to get baseline score
// 2. Loop:
//
//	a. Ask LLM to suggest an improvement (guided by GoalPrompt)
//	b. Apply changes (vibe mode -- no confirmation)
//	c. Run metric within Budget timeout
//	d. If score improved: keep changes, record result, git commit
//	e. If score same/worse: git checkout (rollback), record failure
//	f. If timeout exceeded: rollback, continue
//
// 3. Return all results with best score
func RunResearch(ctx context.Context, config ResearchConfig) ([]ResearchResult, error) {
	if config.MaxIterations <= 0 {
		config.MaxIterations = 50
	}
	if config.Budget <= 0 {
		config.Budget = 5 * time.Minute
	}
	if config.Metric == "" {
		return nil, fmt.Errorf("research: metric command is required")
	}

	// Get baseline score
	baseline, baselineOutput, err := RunMetric(config.Metric, config.Budget)
	if err != nil {
		return nil, fmt.Errorf("research: baseline metric failed: %w", err)
	}
	_ = baselineOutput

	bestScore := baseline
	var results []ResearchResult

	for i := 1; i <= config.MaxIterations; i++ {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		start := time.Now()

		// Run metric to measure the current state
		score, _, metricErr := RunMetric(config.Metric, config.Budget)
		duration := time.Since(start)

		result := ResearchResult{
			Iteration: i,
			Duration:  duration,
		}

		if metricErr != nil {
			// Metric failed (timeout or error): record failure, rollback
			result.Score = bestScore
			result.Improved = false
			result.Description = fmt.Sprintf("metric error: %s", metricErr)
			results = append(results, result)

			// Rollback via git checkout
			rollbackResearchChanges(config.TargetFile)
			continue
		}

		result.Score = score
		if config.KeepBest && score > bestScore {
			result.Improved = true
			result.Description = fmt.Sprintf("improved %.4f -> %.4f (+%.2f%%)",
				bestScore, score, ((score-bestScore)/absFloat(bestScore))*100)
			bestScore = score

			// Git commit the improvement
			commitResearch(config.TargetFile, i, score)
		} else if !config.KeepBest {
			result.Improved = score > bestScore
			result.Description = fmt.Sprintf("score: %.4f (best: %.4f)", score, bestScore)
			if score > bestScore {
				bestScore = score
			}
		} else {
			result.Improved = false
			result.Description = fmt.Sprintf("no improvement: %.4f (best: %.4f)", score, bestScore)
			rollbackResearchChanges(config.TargetFile)
		}

		results = append(results, result)
	}

	return results, nil
}

// FormatResearchReport summarizes all iterations into a human-readable report.
func FormatResearchReport(results []ResearchResult) string {
	if len(results) == 0 {
		return "No research iterations completed."
	}

	var b strings.Builder
	b.WriteString("=== Research Report ===\n\n")

	bestScore := results[0].Score
	bestIter := 1
	improved := 0
	var totalDuration time.Duration

	for _, r := range results {
		status := "  "
		if r.Improved {
			status = "+"
			improved++
		}
		b.WriteString(fmt.Sprintf("[%s] iteration %d: score=%.4f (%s) [%s]\n",
			status, r.Iteration, r.Score, r.Description, r.Duration.Round(time.Millisecond)))
		if r.Score > bestScore {
			bestScore = r.Score
			bestIter = r.Iteration
		}
		totalDuration += r.Duration
	}

	b.WriteString(fmt.Sprintf("\n--- Summary: %d iterations, %d improvements, best=%.4f (iteration %d), total time %s ---\n",
		len(results), improved, bestScore, bestIter, totalDuration.Round(time.Second)))

	return b.String()
}

// rollbackResearchChanges reverts uncommitted changes to a file via git checkout.
func rollbackResearchChanges(targetFile string) {
	if targetFile == "" {
		// Rollback all changes
		exec.Command("git", "checkout", ".").Run()
		return
	}
	exec.Command("git", "checkout", "--", targetFile).Run()
}

// commitResearch creates a git commit for a successful research iteration.
func commitResearch(targetFile string, iteration int, score float64) {
	if targetFile != "" {
		exec.Command("git", "add", targetFile).Run()
	} else {
		exec.Command("git", "add", "-A").Run()
	}
	msg := fmt.Sprintf("research: iteration %d, score %.4f", iteration, score)
	exec.Command("git", "commit", "-m", msg).Run()
}

// absFloat returns the absolute value of a float64.
func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	if f == 0 {
		return 1 // avoid division by zero
	}
	return f
}
