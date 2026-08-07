package mission

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// EngineWorker returns a WorkerFunc that runs an actual engine session
// in an isolated git worktree for each feature.
func EngineWorker(provider, model, systemPrompt string) WorkerFunc {
	return func(ctx context.Context, feature *Feature, missionDir string, cfg Config) (*Handoff, error) {
		if cfg.WorkerModel != "" {
			model = cfg.WorkerModel
		}

		// Create worktree for isolation
		wtPath, err := createWorktree(ctx, cfg.RepoDir, cfg.BaseBranch, feature.Branch)
		if err != nil {
			return nil, fmt.Errorf("worktree: %w", err)
		}
		// Use a detached context for cleanup so that cancellation of the
		// mission context (Ctrl-C, timeout) does not kill the cleanup
		// command. Without this, git worktree remove is killed before it
		// runs and the worktree leaks on disk permanently (C4 fix). The
		// feature branch is deleted alongside so retries (H9) never collide
		// with the previous attempt's branch.
		defer removeWorktreeDetached(cfg.RepoDir, wtPath, feature.Branch)

		// Build the worker prompt
		workerPrompt := fmt.Sprintf(
			"You are working on feature: %s\n\nDescription: %s\n\nExpected behavior: %s\n\n"+
				"Working directory: %s\n\nComplete this feature. Make all necessary code changes, "+
				"then run tests. When done, commit your changes with a descriptive message.",
			feature.ID, feature.Description, feature.ExpectedBehavior, wtPath,
		)

		// Transcript resume: check for an existing transcript for this feature.
		tpath := TranscriptPath(missionDir, feature.ID)
		existingHandoff := checkExistingTranscript(tpath)
		if existingHandoff != nil {
			// Already completed in a previous run — reuse the handoff.
			return existingHandoff, nil
		}

		// Create engine session with tools
		registry := tool.NewRegistry(baseWorkerTools()...)
		selection := hawkconfig.EffectiveSelection(ctx, hawkconfig.SelectionOptions{
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

		// Auto-approve everything in mission workers unless a human approval
		// gate is configured: risky calls (network, destructive file ops)
		// block until the operator responds. The gate's Await uses the worker
		// ctx, so mission cancellation still unblocks it.
		sess.SetPermissionFn(func(req engine.PermissionRequest) {
			if cfg.ApprovalGate != nil && req.Response != nil {
				if err := cfg.ApprovalGate.Check(ctx, req.ToolName, req.Summary); err != nil {
					req.Response <- false
					return
				}
			}
			if req.Response != nil {
				req.Response <- true
			}
		})

		// Transcript resume: if an incomplete transcript exists, load its
		// messages so the session continues from where it left off.
		if resumeMsgs, ok := incompleteTranscriptMessages(tpath); ok && len(resumeMsgs) > 0 {
			sess.LoadMessages(resumeMsgs)
		}

		// Set up transcript persistence for this run.
		writer, err := NewPersistWriter(tpath)
		if err != nil {
			return nil, fmt.Errorf("transcript writer: %w", err)
		}
		defer func() { _ = writer.Close() }()

		// Persist the initial user prompt.
		_ = writer.Write(types.EyrieMessage{Role: "user", Content: workerPrompt})
		sess.AddUser(workerPrompt)

		events, err := sess.Stream(ctx)
		if err != nil {
			return nil, fmt.Errorf("stream: %w", err)
		}

		// Collect output and persist assistant content as it streams.
		var response strings.Builder
		for ev := range events {
			switch ev.Type {
			case "content":
				response.WriteString(ev.Content)
				_ = writer.Write(types.EyrieMessage{Role: "assistant", Content: ev.Content})
			case "tool_use":
				_ = writer.Write(types.EyrieMessage{
					Role: "assistant", Content: "",
					ToolUse: []types.ToolCall{{Name: ev.ToolName, ID: ev.ToolID}},
				})
			}
		}

		// Check for commit
		commitID := getLastCommit(ctx, wtPath)
		filesChanged := getChangedFiles(ctx, wtPath, cfg.BaseBranch)
		testsPassed := runTests(ctx, wtPath)

		handoff := &Handoff{
			CommitID:     commitID,
			RepoPath:     wtPath,
			Summary:      truncate(response.String(), 500),
			FilesChanged: filesChanged,
			TestsPassed:  testsPassed,
		}

		// Mark the transcript complete with the handoff result.
		_ = writer.MarkComplete(handoff)
		return handoff, nil
	}
}

// checkExistingTranscript returns the handoff from a completed transcript, or
// nil if the transcript does not exist or is incomplete.
func checkExistingTranscript(path string) *Handoff {
	_, handoff, complete, err := LoadTranscript(path)
	if err != nil || !complete || handoff == nil {
		return nil
	}
	return handoff
}

// incompleteTranscriptMessages returns the messages from an incomplete
// transcript (one without a completion marker). Returns false if the transcript
// is missing or complete.
func incompleteTranscriptMessages(path string) ([]types.EyrieMessage, bool) {
	exists, complete, err := IsTranscriptComplete(path)
	if err != nil || !exists || complete {
		return nil, false
	}
	msgs, _, _, err := LoadTranscript(path)
	if err != nil {
		return nil, false
	}
	return msgs, true
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

		wtPath, err := createWorktree(ctx, cfg.RepoDir, cfg.BaseBranch, feature.Branch)
		if err != nil {
			return nil, fmt.Errorf("worktree: %w", err)
		}
		// Detached-context cleanup (M6): the old `defer removeWorktree(ctx,...)`
		// used the caller's cancellable context, so a cancelled mission killed
		// `git worktree remove` before it ran and the worktree leaked on disk
		// permanently. Also delete the branch so a subsequent validation or
		// retry cannot collide with it.
		defer removeWorktreeDetached(cfg.RepoDir, wtPath, feature.Branch)

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
		selection := hawkconfig.EffectiveSelection(ctx, hawkconfig.SelectionOptions{
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
		sess.SetPermissionFn(func(req engine.PermissionRequest) {
			if req.Response != nil {
				req.Response <- true
			}
		})

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

func createWorktree(ctx context.Context, repoDir, baseBranch, branch string) (string, error) {
	dir, err := exec.CommandContext(ctx, "mktemp", "-d").Output()
	if err != nil {
		return "", err
	}
	wtPath := strings.TrimSpace(string(dir))

	// #nosec G204 -- binary is the fixed string "git"; branch/wtPath/baseBranch come from internal mission state, not raw external input
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-b", branch, wtPath, baseBranch)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		// The branch already exists (retry with leaked branch, or a
		// validation worker on the implementation worker's branch): fall
		// back to checking out the existing branch instead of failing (H9).
		if strings.Contains(string(out), "already exists") {
			// #nosec G204 -- binary is the fixed string "git"; wtPath/branch come from internal mission state, not raw external input
			fallback := exec.CommandContext(ctx, "git", "worktree", "add", wtPath, branch)
			fallback.Dir = repoDir
			if fout, ferr := fallback.CombinedOutput(); ferr == nil {
				return wtPath, nil
			} else if !strings.Contains(string(fout), "already") {
				_ = os.RemoveAll(wtPath)
				return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(fout)), ferr)
			}
		}
		// Clean up the temp directory created by mktemp so it doesn't
		// leak on disk when git worktree add fails (C5 fix).
		_ = os.RemoveAll(wtPath)
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return wtPath, nil
}

// removeWorktreeDetached removes a git worktree using a fresh, non-cancellable
// context with a generous timeout. This ensures cleanup runs even when the
// mission context was cancelled (C4 fix). The worktree's branch is deleted
// afterwards (best-effort) so retries never collide with a leaked branch (H9).
// The original removeWorktree used the caller's context, which meant a
// cancelled mission would kill the cleanup command before it could run,
// leaking the worktree directory permanently.
func removeWorktreeDetached(repoDir, wtPath, branch string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	removeWorktree(cleanupCtx, repoDir, wtPath)
	if branch == "" {
		return
	}
	// #nosec G204 -- binary is the fixed string "git"; branch comes from internal mission state
	del := exec.CommandContext(cleanupCtx, "git", "branch", "-D", branch)
	del.Dir = repoDir
	if out, err := del.CombinedOutput(); err != nil && !strings.Contains(string(out), "not found") {
		fmt.Fprintf(os.Stderr, "warning: failed to delete mission branch %s: %v\n", branch, err)
	}
}

func removeWorktree(ctx context.Context, repoDir, wtPath string) {
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", wtPath) // #nosec G204 -- fixed git executable
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		// Best-effort: if git worktree remove fails (e.g. the worktree
		// metadata is already gone), still try to remove the directory
		// itself so we don't leak the temp dir from mktemp.
		_ = os.RemoveAll(wtPath)
		fmt.Fprintf(os.Stderr, "warning: failed to remove worktree %s: %v\n", wtPath, err)
	}
}

func getLastCommit(ctx context.Context, dir string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func getChangedFiles(ctx context.Context, dir, baseBranch string) []string {
	// #nosec G204 -- binary is the fixed string "git"; baseBranch comes from internal Config, not raw external input
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", baseBranch+"..HEAD")
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

func runTests(ctx context.Context, dir string) bool {
	cmd := exec.CommandContext(ctx, "go", "test", "./...", "-timeout", "60s")
	cmd.Dir = dir
	return cmd.Run() == nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
