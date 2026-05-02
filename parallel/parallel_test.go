package parallel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// initTestRepo creates a temporary git repo with one commit on "main" and
// returns its path. The caller should defer os.RemoveAll on the returned path.
func initTestRepo(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "hawk-parallel-test-*")
	if err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %s: %v", args, out, err)
		}
	}

	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@hawk.dev")
	run("git", "config", "user.name", "Hawk Test")

	// Create an initial commit so branches can be made.
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# test repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "README.md")
	run("git", "commit", "-m", "initial commit")

	return dir
}

func TestCreateAndRemoveWorktree(t *testing.T) {
	repo := initTestRepo(t)
	defer os.RemoveAll(repo)

	branch := "hawk-parallel/wt-test"
	wtPath, err := createWorktree(repo, "main", branch)
	if err != nil {
		t.Fatalf("createWorktree: %v", err)
	}

	// The worktree directory should exist and contain the README.
	if _, err := os.Stat(filepath.Join(wtPath, "README.md")); err != nil {
		t.Fatalf("README.md not found in worktree: %v", err)
	}

	// Remove the worktree.
	if err := removeWorktree(repo, wtPath); err != nil {
		t.Fatalf("removeWorktree: %v", err)
	}

	// The worktree directory should be gone.
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree directory still exists after removal")
	}
}

func TestCleanupIdempotent(t *testing.T) {
	repo := initTestRepo(t)
	defer os.RemoveAll(repo)

	pool := NewPool(repo, "main", 2)
	pool.AddTask("task a")

	ctx := context.Background()
	err := pool.Run(ctx, func(_ context.Context, wtPath string, task *Task) (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// First cleanup.
	if err := pool.Cleanup(); err != nil {
		t.Fatalf("first Cleanup: %v", err)
	}
	// Second cleanup should also succeed (idempotent).
	if err := pool.Cleanup(); err != nil {
		t.Fatalf("second Cleanup: %v", err)
	}
}

func TestParallelExecution(t *testing.T) {
	repo := initTestRepo(t)
	defer os.RemoveAll(repo)

	pool := NewPool(repo, "main", 4)
	numTasks := 4
	for i := 0; i < numTasks; i++ {
		pool.AddTask(fmt.Sprintf("task-%d", i))
	}

	var running atomic.Int32
	var maxConcurrent atomic.Int32

	ctx := context.Background()
	err := pool.Run(ctx, func(_ context.Context, wtPath string, task *Task) (string, error) {
		cur := running.Add(1)
		// Track the maximum observed concurrency.
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}

		// Simulate a small amount of work.
		time.Sleep(50 * time.Millisecond)

		// Verify the worktree is functional by checking for README.
		if _, err := os.Stat(filepath.Join(wtPath, "README.md")); err != nil {
			running.Add(-1)
			return "", fmt.Errorf("README not found: %v", err)
		}

		running.Add(-1)
		return fmt.Sprintf("completed %s", task.ID), nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer pool.Cleanup()

	results := pool.Results()
	if len(results) != numTasks {
		t.Fatalf("expected %d results, got %d", numTasks, len(results))
	}

	doneCount := 0
	for _, r := range results {
		if r.Status == StatusDone {
			doneCount++
		}
		if r.Status == StatusFailed {
			t.Errorf("task %s failed: %v", r.ID, r.Error)
		}
	}
	if doneCount != numTasks {
		t.Errorf("expected %d done tasks, got %d", numTasks, doneCount)
	}

	if maxConcurrent.Load() < 2 {
		t.Logf("warning: max concurrency was %d (expected >= 2); may be due to scheduling", maxConcurrent.Load())
	}
}

func TestErrorHandling(t *testing.T) {
	repo := initTestRepo(t)
	defer os.RemoveAll(repo)

	pool := NewPool(repo, "main", 4)
	pool.AddTask("good-1")
	pool.AddTask("fail-1")
	pool.AddTask("good-2")

	ctx := context.Background()
	err := pool.Run(ctx, func(_ context.Context, wtPath string, task *Task) (string, error) {
		if task.Description == "fail-1" {
			return "", fmt.Errorf("intentional failure")
		}
		return "success", nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer pool.Cleanup()

	results := pool.Results()
	var doneCount, failCount int
	for _, r := range results {
		switch r.Status {
		case StatusDone:
			doneCount++
			if r.Result != "success" {
				t.Errorf("task %s: unexpected result %q", r.ID, r.Result)
			}
		case StatusFailed:
			failCount++
			if r.Error == nil {
				t.Errorf("task %s: status is failed but error is nil", r.ID)
			}
		}
	}

	if doneCount != 2 {
		t.Errorf("expected 2 done tasks, got %d", doneCount)
	}
	if failCount != 1 {
		t.Errorf("expected 1 failed task, got %d", failCount)
	}
}

func TestCleanupRemovesAllWorktrees(t *testing.T) {
	repo := initTestRepo(t)
	defer os.RemoveAll(repo)

	pool := NewPool(repo, "main", 2)
	pool.AddTask("cleanup-a")
	pool.AddTask("cleanup-b")

	var paths []string
	ctx := context.Background()
	err := pool.Run(ctx, func(_ context.Context, wtPath string, task *Task) (string, error) {
		paths = append(paths, wtPath)
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// All worktree paths should exist before cleanup.
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("worktree %s should exist before cleanup: %v", p, err)
		}
	}

	if err := pool.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// All worktree paths should be gone after cleanup.
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("worktree %s should not exist after cleanup", p)
		}
	}

	// git worktree list should only show the main repo.
	cmd := exec.Command("git", "worktree", "list")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree list: %v", err)
	}
	// There should be exactly one line (the main worktree).
	lines := 0
	for _, line := range splitLines(string(out)) {
		if line != "" {
			lines++
		}
	}
	if lines != 1 {
		t.Errorf("expected 1 worktree (main), got %d:\n%s", lines, out)
	}
}

func TestContextCancellation(t *testing.T) {
	repo := initTestRepo(t)
	defer os.RemoveAll(repo)

	pool := NewPool(repo, "main", 1) // single worker to serialize tasks
	pool.AddTask("slow-task")
	pool.AddTask("blocked-task")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := pool.Run(ctx, func(ctx context.Context, wtPath string, task *Task) (string, error) {
		// First task blocks until context expires.
		select {
		case <-time.After(5 * time.Second):
			return "done", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer pool.Cleanup()

	// At least one task should have failed due to context.
	results := pool.Results()
	failedCount := 0
	for _, r := range results {
		if r.Status == StatusFailed {
			failedCount++
		}
	}
	if failedCount == 0 {
		t.Error("expected at least one task to fail due to context cancellation")
	}
}

func TestMergeWorktree(t *testing.T) {
	repo := initTestRepo(t)
	defer os.RemoveAll(repo)

	branch := "hawk-parallel/merge-test"
	wtPath, err := createWorktree(repo, "main", branch)
	if err != nil {
		t.Fatalf("createWorktree: %v", err)
	}

	// Make a change in the worktree and commit it.
	newFile := filepath.Join(wtPath, "new.txt")
	if err := os.WriteFile(newFile, []byte("hello from worktree\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v in %s: %s: %v", args, dir, out, err)
		}
	}
	runGit(wtPath, "add", "new.txt")
	runGit(wtPath, "commit", "-m", "add new.txt from worktree")

	// Remove the worktree before merging.
	if err := removeWorktree(repo, wtPath); err != nil {
		t.Fatalf("removeWorktree: %v", err)
	}

	// Merge the branch back.
	if err := mergeWorktree(repo, "main", branch); err != nil {
		t.Fatalf("mergeWorktree: %v", err)
	}

	// Verify the merged file exists in the main worktree.
	merged := filepath.Join(repo, "new.txt")
	if _, err := os.Stat(merged); err != nil {
		t.Fatalf("new.txt not found after merge: %v", err)
	}
}

// splitLines is a small helper to split output into non-empty lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
