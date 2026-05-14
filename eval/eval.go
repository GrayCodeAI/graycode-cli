package eval

// Framework for evaluating hawk's coding performance against benchmarks.

import (
	"context"
	"fmt"
	"os"
	"time"
)

// BenchmarkSuite represents a collection of benchmark tasks for evaluation.
type BenchmarkSuite struct {
	Name    string
	Tasks   []BenchmarkTask
	Results []TaskResult
}

// BenchmarkTask defines a single coding task to evaluate.
type BenchmarkTask struct {
	ID          string
	Description string
	SetupFn     func(workDir string) error
	ValidateFn  func(workDir string) (bool, string)
	Prompt      string
	TimeLimit   time.Duration
	Tags        []string
}

// TaskResult captures the outcome of running a single benchmark task.
type TaskResult struct {
	TaskID     string
	Passed     bool
	Duration   time.Duration
	TokensUsed int
	CostUSD    float64
	Attempts   int
	Error      string
}

// SuiteResult aggregates results from running an entire benchmark suite.
type SuiteResult struct {
	Suite         string
	TotalTasks    int
	Passed        int
	Failed        int
	TotalDuration time.Duration
	TotalTokens   int
	TotalCostUSD  float64
	PassRate      float64
	Results       []TaskResult
}

// Runner executes benchmark tasks against a specific model/provider.
type Runner struct {
	Model       string
	Provider    string
	MaxAttempts int
	Timeout     time.Duration
}

// NewRunner creates a Runner configured for the given model and provider.
func NewRunner(model, provider string) *Runner {
	return &Runner{
		Model:       model,
		Provider:    provider,
		MaxAttempts: 3,
		Timeout:     5 * time.Minute,
	}
}

// Run executes all tasks in a benchmark suite and returns aggregated results.
func (r *Runner) Run(ctx context.Context, suite *BenchmarkSuite) (*SuiteResult, error) {
	if suite == nil {
		return nil, fmt.Errorf("suite cannot be nil")
	}

	result := &SuiteResult{
		Suite:      suite.Name,
		TotalTasks: len(suite.Tasks),
		Results:    make([]TaskResult, 0, len(suite.Tasks)),
	}

	startTime := time.Now()

	for i := range suite.Tasks {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		taskResult, err := r.RunSingle(ctx, &suite.Tasks[i])
		if err != nil {
			taskResult = &TaskResult{
				TaskID: suite.Tasks[i].ID,
				Passed: false,
				Error:  err.Error(),
			}
		}

		result.Results = append(result.Results, *taskResult)
		if taskResult.Passed {
			result.Passed++
		} else {
			result.Failed++
		}
		result.TotalTokens += taskResult.TokensUsed
		result.TotalCostUSD += taskResult.CostUSD
	}

	result.TotalDuration = time.Since(startTime)
	if result.TotalTasks > 0 {
		result.PassRate = float64(result.Passed) / float64(result.TotalTasks)
	}

	// Store results back into the suite for reference.
	suite.Results = result.Results

	return result, nil
}

// RunSingle executes a single benchmark task in an isolated temporary directory.
func (r *Runner) RunSingle(ctx context.Context, task *BenchmarkTask) (*TaskResult, error) {
	if task == nil {
		return nil, fmt.Errorf("task cannot be nil")
	}

	timeout := r.Timeout
	if task.TimeLimit > 0 {
		timeout = task.TimeLimit
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := &TaskResult{
		TaskID:   task.ID,
		Attempts: 0,
	}

	var lastErr error

	for attempt := 1; attempt <= r.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			result.Error = fmt.Sprintf("context cancelled after %d attempts: %v", attempt-1, ctx.Err())
			return result, nil
		default:
		}

		result.Attempts = attempt

		// Create isolated work directory for this attempt.
		workDir, err := os.MkdirTemp("", fmt.Sprintf("hawk-eval-%s-*", task.ID))
		if err != nil {
			lastErr = fmt.Errorf("failed to create temp dir: %w", err)
			continue
		}
		defer func() { _ = os.RemoveAll(workDir) }()

		// Run setup to create the initial buggy/incomplete code.
		startTime := time.Now()
		if err := task.SetupFn(workDir); err != nil {
			lastErr = fmt.Errorf("setup failed: %w", err)
			continue
		}

		// In a real integration, this is where hawk would be invoked with task.Prompt
		// to attempt to fix/complete the code. For the framework itself, we simulate
		// by just validating the setup (which should fail) and measuring the harness.
		//
		// The actual LLM invocation would be plugged in here by the caller:
		//   err = invokeLLM(ctx, r.Model, r.Provider, task.Prompt, workDir)
		//
		// For now, we run validation to test the framework mechanics.

		passed, msg := task.ValidateFn(workDir)
		result.Duration = time.Since(startTime)

		if passed {
			result.Passed = true
			return result, nil
		}

		lastErr = fmt.Errorf("validation failed: %s", msg)
	}

	if lastErr != nil {
		result.Error = lastErr.Error()
	}
	return result, nil
}
