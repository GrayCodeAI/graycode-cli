package mission

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/GrayCodeAI/eyrie/runtime"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// EngineWorker returns a WorkerFunc that runs an actual engine session
// in an isolated git worktree for each feature.
func EngineWorker(provider, model, systemPrompt string) WorkerFunc {
	return func(ctx context.Context, feature *Feature, missionDir string, cfg Config) (*Handoff, error) {
		if cfg.WorkerModel != "" {
			model = cfg.WorkerModel
		}

		// Create worktree for isolation
		wtPath, err := createWorktree(cfg.RepoDir, cfg.BaseBranch, feature.Branch)
		if err != nil {
			return nil, fmt.Errorf("worktree: %w", err)
		}
		defer removeWorktree(cfg.RepoDir, wtPath)

		// Build the worker prompt
		workerPrompt := fmt.Sprintf(
			"You are working on feature: %s\n\nDescription: %s\n\nExpected behavior: %s\n\n"+
				"Working directory: %s\n\nComplete this feature. Make all necessary code changes, "+
				"then run tests. When done, commit your changes with a descriptive message.",
			feature.ID, feature.Description, feature.ExpectedBehavior, wtPath,
		)

		// Create engine session with tools
		registry := tool.NewRegistry(baseWorkerTools()...)
		selection := runtime.EffectiveSelection(ctx, runtime.SelectionOpts{
			ProviderOverride: provider,
			ModelOverride:    model,
		})
		sess := engine.NewHawkSession(ctx, selection, provider, model, systemPrompt, registry)

		// Configure for autonomous operation
		level := engine.AutonomyLevel(cfg.AutonomyLevel)
		if level < engine.AutonomyFull {
			level = engine.AutonomyFull
		}
		sess.PermSvc().SetAutonomy(level)
		if setErr := sess.SetMaxTurns(30); setErr != nil {
			return nil, fmt.Errorf("set max turns: %w", setErr)
		}

		// Auto-approve everything in mission workers
		sess.PermissionFn = func(req engine.PermissionRequest) {
			if req.Response != nil {
				req.Response <- true
			}
		}

		sess.AddUser(workerPrompt)

		events, err := sess.Stream(ctx)
		if err != nil {
			return nil, fmt.Errorf("stream: %w", err)
		}

		// Collect output
		var response strings.Builder
		for ev := range events {
			if ev.Type == "content" {
				response.WriteString(ev.Content)
			}
		}

		// Check for commit
		commitID := getLastCommit(wtPath)
		filesChanged := getChangedFiles(wtPath, cfg.BaseBranch)
		testsPassed := runTests(wtPath)

		return &Handoff{
			CommitID:     commitID,
			RepoPath:     wtPath,
			Summary:      truncate(response.String(), 500),
			FilesChanged: filesChanged,
			TestsPassed:  testsPassed,
		}, nil
	}
}

func baseWorkerTools() []tool.Tool {
	return []tool.Tool{
		tool.FileReadTool{},
		tool.FileWriteTool{},
		tool.FileEditTool{},
		tool.BashTool{},
		tool.GrepTool{},
		tool.GlobTool{},
		tool.LSTool{},
	}
}

// readOnlyWorkerTools is the tool set for a read-only validation worker. It
// deliberately omits every mutating tool — no Write, no Edit, and no Bash
// (Bash can mutate the tree via rm/git/etc.) — so the validator cannot change
// the implementation it is judging. This mirrors the two-agent pattern where a
// separate read-only agent validates an implementation worker's output: the
// guarantee comes from the registry, not just the prompt.
func readOnlyWorkerTools() []tool.Tool {
	return []tool.Tool{
		tool.FileReadTool{},
		tool.GrepTool{},
		tool.GlobTool{},
		tool.LSTool{},
	}
}

// ReadOnlyValidationWorker returns a WorkerFunc that reviews an already-produced
// implementation without modifying it. It runs in the same worktree the
// implementation worker committed to (passed via cfg.RepoDir / the handed-off
// branch) using a read-only tool registry, and returns its verdict as the
// Handoff.Summary. It never commits and reports no FilesChanged of its own.
//
// This is the validation half of the implement-then-validate pair: pipeline an
// EngineWorker (implementation) into a ReadOnlyValidationWorker (validation) so
// the agent that writes the code is never the agent that signs off on it.
func ReadOnlyValidationWorker(provider, model, systemPrompt string) WorkerFunc {
	return func(ctx context.Context, feature *Feature, missionDir string, cfg Config) (*Handoff, error) {
		if cfg.WorkerModel != "" {
			model = cfg.WorkerModel
		}

		wtPath, err := createWorktree(cfg.RepoDir, cfg.BaseBranch, feature.Branch)
		if err != nil {
			return nil, fmt.Errorf("worktree: %w", err)
		}
		defer removeWorktree(cfg.RepoDir, wtPath)

		validationPrompt := fmt.Sprintf(
			"You are validating the implementation of feature: %s\n\nDescription: %s\n\n"+
				"Expected behavior: %s\n\nWorking directory: %s\n\n"+
				"You are READ-ONLY: you can read, search, and list files but cannot modify, "+
				"write, or run shell commands. Inspect the code against the expected behavior "+
				"and report, for each acceptance criterion, a concrete PASS or FAIL with the "+
				"file:line evidence you based it on. Do not assume — cite what you actually read.",
			feature.ID, feature.Description, feature.ExpectedBehavior, wtPath,
		)

		registry := tool.NewRegistry(readOnlyWorkerTools()...)
		selection := runtime.EffectiveSelection(ctx, runtime.SelectionOpts{
			ProviderOverride: provider,
			ModelOverride:    model,
		})
		sess := engine.NewHawkSession(ctx, selection, provider, model, systemPrompt, registry)

		level := engine.AutonomyLevel(cfg.AutonomyLevel)
		if level < engine.AutonomyFull {
			level = engine.AutonomyFull
		}
		sess.PermSvc().SetAutonomy(level)
		if setErr := sess.SetMaxTurns(30); setErr != nil {
			return nil, fmt.Errorf("set max turns: %w", setErr)
		}
		sess.PermissionFn = func(req engine.PermissionRequest) {
			if req.Response != nil {
				req.Response <- true
			}
		}

		sess.AddUser(validationPrompt)

		events, err := sess.Stream(ctx)
		if err != nil {
			return nil, fmt.Errorf("stream: %w", err)
		}

		var response strings.Builder
		for ev := range events {
			if ev.Type == "content" {
				response.WriteString(ev.Content)
			}
		}

		return &Handoff{
			RepoPath: wtPath,
			Summary:  truncate(response.String(), 500),
		}, nil
	}
}

func createWorktree(repoDir, baseBranch, branch string) (string, error) {
	dir, err := exec.CommandContext(context.Background(), "mktemp", "-d").Output()
	if err != nil {
		return "", err
	}
	wtPath := strings.TrimSpace(string(dir))

	// #nosec G204 -- binary is the fixed string "git"; branch/wtPath/baseBranch come from internal mission state, not raw external input
	cmd := exec.CommandContext(context.Background(), "git", "worktree", "add", "-b", branch, wtPath, baseBranch)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return wtPath, nil
}

func removeWorktree(repoDir, wtPath string) {
	cmd := exec.CommandContext(context.Background(), "git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to remove worktree %s: %v\n", wtPath, err)
	}
}

func getLastCommit(dir string) string {
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func getChangedFiles(dir, baseBranch string) []string {
	// #nosec G204 -- binary is the fixed string "git"; baseBranch comes from internal Config, not raw external input
	cmd := exec.CommandContext(context.Background(), "git", "diff", "--name-only", baseBranch+"..HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func runTests(dir string) bool {
	cmd := exec.CommandContext(context.Background(), "go", "test", "./...", "-timeout", "60s")
	cmd.Dir = dir
	return cmd.Run() == nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
