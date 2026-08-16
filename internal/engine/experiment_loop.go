package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// gitRollbackTimeout bounds rollback git operations in the experiment loop.
// Rollback deliberately uses a detached context: reverting an experiment must
// complete even when the surrounding request context has already been
// canceled, but it must not hang forever on a stuck git invocation.
const gitRollbackTimeout = 2 * time.Minute

// ExperimentResult holds the outcome of a single autonomous experiment.
type ExperimentResult struct {
	ID       int
	Change   string // description of what was tried
	Passed   bool
	Metric   string // validation output
	Duration time.Duration
	Kept     bool
}

// ExperimentLoop runs autonomous iterations: modify → validate → keep/discard.
type ExperimentLoop struct {
	WorkDir     string
	MaxIters    int
	Timeout     time.Duration // per-iteration timeout
	ValidateCmd string        // command to validate (e.g. "go test ./...")
	Results     []ExperimentResult
}

// NewExperimentLoop creates a loop with sensible defaults for coding.
func NewExperimentLoop(workDir, validateCmd string, maxIters int) *ExperimentLoop {
	if maxIters <= 0 {
		maxIters = 10
	}
	return &ExperimentLoop{
		WorkDir:     workDir,
		MaxIters:    maxIters,
		Timeout:     5 * time.Minute,
		ValidateCmd: validateCmd,
	}
}

// Run executes the autonomous loop. For each iteration:
// 1. Call modifyFn to get a code change (via LLM)
// 2. Apply the change
// 3. Run validation
// 4. If passes → keep. If fails → revert.
// 5. Repeat until maxIters or all passing.
func (el *ExperimentLoop) Run(ctx context.Context, modifyFn func(ctx context.Context, iteration int, history []ExperimentResult) (change string, err error)) error {
	for i := 0; i < el.MaxIters; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Snapshot current state
		snapshot, err := el.snapshot(ctx)
		if err != nil {
			return fmt.Errorf("snapshot failed: %w", err)
		}

		// Get modification from LLM
		start := time.Now()
		change, err := modifyFn(ctx, i, el.Results)
		if err != nil {
			return fmt.Errorf("modify failed at iteration %d: %w", i, err)
		}

		// Validate
		passed, metric := el.validate(ctx)
		duration := time.Since(start)

		result := ExperimentResult{
			ID:       i + 1,
			Change:   change,
			Passed:   passed,
			Metric:   metric,
			Duration: duration,
		}

		if passed {
			result.Kept = true
			el.Results = append(el.Results, result)
		} else {
			// Revert
			result.Kept = false
			el.Results = append(el.Results, result)
			el.restore(snapshot)
		}

		// If validation passes with no changes needed, we're done
		if passed && strings.Contains(change, "no changes needed") {
			break
		}
	}
	return nil
}

// validate runs the validation command and returns pass/fail + output.
func (el *ExperimentLoop) validate(ctx context.Context) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, el.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", el.ValidateCmd) // #nosec G204 -- shell invocation with validate command from project/task config
	cmd.Dir = el.WorkDir
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if len(output) > 2000 {
		output = output[len(output)-2000:]
	}
	return err == nil, output
}

// snapshot captures git state for rollback.
func (el *ExperimentLoop) snapshot(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "stash", "create")
	cmd.Dir = el.WorkDir
	out, err := cmd.Output()
	if err != nil {
		// No changes to stash — use HEAD
		cmd = exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
		cmd.Dir = el.WorkDir
		out, err = cmd.Output()
		if err != nil {
			// Fall back to the empty-ref rollback path, but make the failure
			// visible instead of silently discarding it.
			slog.Warn("experiment loop: git rev-parse HEAD failed; falling back to checkout rollback", "dir", el.WorkDir, "error", err)
		}
	}
	return strings.TrimSpace(string(out)), nil
}

// restore reverts to a snapshot. It uses a detached, time-bounded context
// (gitRollbackTimeout) rather than the request context because a rollback
// must still complete when the request has just been canceled.
func (el *ExperimentLoop) restore(ref string) {
	ctx, cancel := context.WithTimeout(context.Background(), gitRollbackTimeout)
	defer cancel()

	if ref == "" {
		// NOTE: no WorkDir here (pre-existing behaviour): rolls back the
		// process CWD when no snapshot ref was captured.
		if err := exec.CommandContext(ctx, "git", "checkout", "--", ".").Run(); err != nil {
			slog.Warn("experiment loop: rollback (checkout) failed", "error", err)
		}
		return
	}
	cmd := exec.CommandContext(ctx, "git", "checkout", "--", ".")
	cmd.Dir = el.WorkDir
	if err := cmd.Run(); err != nil {
		slog.Warn("experiment loop: rollback (checkout) failed", "dir", el.WorkDir, "error", err)
	}
}

// Summary returns a formatted summary of all experiments.
func (el *ExperimentLoop) Summary() string {
	if len(el.Results) == 0 {
		return "No experiments run."
	}
	var sb strings.Builder
	kept, discarded := 0, 0
	for _, r := range el.Results {
		status := icons.CloseThick() + " REVERTED"
		if r.Kept {
			status = icons.CheckBold() + " KEPT"
			kept++
		} else {
			discarded++
		}
		sb.WriteString(fmt.Sprintf("#%d [%s] %s (%s)\n", r.ID, status, r.Change, r.Duration.Round(time.Second)))
	}
	sb.WriteString(fmt.Sprintf("\nTotal: %d experiments | %d kept | %d reverted\n", len(el.Results), kept, discarded))
	return sb.String()
}

// ExperimentPrompt generates the prompt for the LLM to suggest the next change.
func ExperimentPrompt(iteration int, validateCmd string, history []ExperimentResult, lastOutput string) string {
	var historySection string
	if len(history) > 0 {
		historySection = "\n\nPrevious experiments:\n"
		for _, r := range history {
			status := "KEPT"
			if !r.Kept {
				status = "REVERTED"
			}
			historySection += fmt.Sprintf("- #%d [%s]: %s\n", r.ID, status, r.Change)
		}
	}

	return fmt.Sprintf(`You are an autonomous code experimenter. Iteration %d.

VALIDATION COMMAND: %s
LAST OUTPUT:
%s
%s
RULES:
- Make ONE focused change that you believe will fix the failing validation or improve the code
- If validation already passes, look for optimizations or say "no changes needed"
- Don't repeat changes that were already reverted
- Be bold but focused — one hypothesis per iteration

What single change should we try next? Make the edit, then explain in one line what you changed.`,
		iteration+1, validateCmd, lastOutput, historySection)
}

// DefaultValidateCmd detects the project type and returns the appropriate test command.
func DefaultValidateCmd(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return "go test ./..."
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		return "npm test"
	}
	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		return "cargo test"
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
		return "pytest"
	}
	if _, err := os.Stat(filepath.Join(dir, "Makefile")); err == nil {
		return "make test"
	}
	return "echo 'no test command detected'"
}
