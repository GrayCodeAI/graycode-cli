package mission

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/GrayCodeAI/hawk/engine"
	"github.com/GrayCodeAI/hawk/tool"
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
			feature.ID, feature.Description, feature.ExpectedBehavior, wtPath)

		// Create engine session with tools
		registry := tool.NewRegistry(baseWorkerTools()...)
		sess := engine.NewSession(provider, model, systemPrompt, registry)

		// Configure for autonomous operation
		sess.Autonomy = engine.AutonomyLevel(cfg.AutonomyLevel)
		if sess.Autonomy < engine.AutonomyFull {
			sess.Autonomy = engine.AutonomyFull
		}
		sess.MaxTurns = 30

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

func createWorktree(repoDir, baseBranch, branch string) (string, error) {
	dir, err := exec.Command("mktemp", "-d").Output()
	if err != nil {
		return "", err
	}
	wtPath := strings.TrimSpace(string(dir))

	cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath, baseBranch)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return wtPath, nil
}

func removeWorktree(repoDir, wtPath string) {
	cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = repoDir
	_ = cmd.Run()
}

func getLastCommit(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func getChangedFiles(dir, baseBranch string) []string {
	cmd := exec.Command("git", "diff", "--name-only", baseBranch+"..HEAD")
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
	cmd := exec.Command("go", "test", "./...", "-timeout", "60s")
	cmd.Dir = dir
	return cmd.Run() == nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
