package parallel

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// initManagerTestRepo creates a temporary git repo suitable for WorktreeManager tests.
func initManagerTestRepo(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "hawk-wm-test-*")
	if err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_CONFIG_COUNT=2",
			"GIT_CONFIG_KEY_0=commit.gpgsign",
			"GIT_CONFIG_VALUE_0=false",
			"GIT_CONFIG_KEY_1=tag.gpgsign",
			"GIT_CONFIG_VALUE_1=false",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %s: %v", args, out, err)
		}
	}

	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@hawk.dev")
	run("git", "config", "user.name", "Hawk Test")
	run("git", "config", "commit.gpgsign", "false")
	run("git", "config", "tag.gpgsign", "false")

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# worktree manager test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "README.md")
	run("git", "commit", "-m", "initial commit")

	return dir
}

func TestNewWorktreeManager(t *testing.T) {
	wm := NewWorktreeManager("/tmp/fake")
	if wm.BaseDir != "/tmp/fake" {
		t.Errorf("expected BaseDir /tmp/fake, got %s", wm.BaseDir)
	}
	if wm.MaxWorktrees != 8 {
		t.Errorf("expected MaxWorktrees 8, got %d", wm.MaxWorktrees)
	}
	if wm.Worktrees == nil {
		t.Error("Worktrees map should not be nil")
	}
}

func TestCreateAndGet(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)
	wt, err := wm.Create("feature/auth", "main", "Implement JWT authentication")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if wt.Branch != "feature/auth" {
		t.Errorf("expected branch feature/auth, got %s", wt.Branch)
	}
	if wt.BaseBranch != "main" {
		t.Errorf("expected base branch main, got %s", wt.BaseBranch)
	}
	if wt.Status != WorktreeActive {
		t.Errorf("expected status active, got %s", wt.Status)
	}
	if wt.TaskDescription != "Implement JWT authentication" {
		t.Errorf("unexpected task description: %s", wt.TaskDescription)
	}

	// Verify worktree directory exists with correct content.
	readmePath := filepath.Join(wt.Path, "README.md")
	if _, err := os.Stat(readmePath); err != nil {
		t.Fatalf("README.md not found in worktree: %v", err)
	}

	// Get should return the same worktree.
	got := wm.Get(wt.ID)
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.ID != wt.ID {
		t.Errorf("Get returned wrong worktree: got %s, want %s", got.ID, wt.ID)
	}

	// Get with unknown ID should return nil.
	if wm.Get("nonexistent") != nil {
		t.Error("Get should return nil for unknown ID")
	}
}

func TestCreateMaxWorktrees(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)
	wm.MaxWorktrees = 2

	_, err := wm.Create("branch-1", "main", "task 1")
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	_, err = wm.Create("branch-2", "main", "task 2")
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	// Third should fail.
	_, err = wm.Create("branch-3", "main", "task 3")
	if err == nil {
		t.Fatal("expected error when exceeding MaxWorktrees")
	}
	if !strings.Contains(err.Error(), "maximum worktrees reached") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRemove(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)
	wt, err := wm.Create("feature/remove-test", "main", "test removal")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	wtPath := wt.Path
	id := wt.ID

	// Remove should succeed.
	if err := wm.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Worktree should no longer be in the map.
	if wm.Get(id) != nil {
		t.Error("worktree should not be found after removal")
	}

	// Directory should be gone (git worktree remove cleans it up).
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree path should not exist after removal")
	}

	// Remove unknown ID should error.
	if err := wm.Remove("nonexistent"); err == nil {
		t.Error("Remove should error for unknown ID")
	}
}

func TestList(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)

	// Empty list initially.
	if len(wm.List()) != 0 {
		t.Error("expected empty list initially")
	}

	wt1, _ := wm.Create("branch-a", "main", "task a")
	time.Sleep(10 * time.Millisecond)
	wt2, _ := wm.Create("branch-b", "main", "task b")

	list := wm.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(list))
	}

	// List is sorted newest first.
	if list[0].ID != wt2.ID {
		t.Errorf("expected newest first, got %s", list[0].ID)
	}
	if list[1].ID != wt1.ID {
		t.Errorf("expected oldest second, got %s", list[1].ID)
	}
}

func TestMergeBackMergeCommit(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)
	wt, err := wm.Create("feature/merge-test", "main", "test merge-commit")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Make a commit in the worktree.
	newFile := filepath.Join(wt.Path, "merged.txt")
	if err := os.WriteFile(newFile, []byte("merge content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, wt.Path, "add", "merged.txt")
	runGitIn(t, wt.Path, "commit", "-m", "add merged.txt")

	// Remove worktree before merging (branch still exists).
	if err := wm.Remove(wt.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Re-register the worktree info for merge (simulating it was marked complete).
	wm.mu.Lock()
	wm.Worktrees[wt.ID] = wt
	wt.Status = WorktreeComplete
	wm.mu.Unlock()

	if err := wm.MergeBack(wt.ID, "merge-commit"); err != nil {
		t.Fatalf("MergeBack: %v", err)
	}

	// Verify status was updated.
	if wm.Get(wt.ID).Status != WorktreeMerged {
		t.Errorf("expected status merged, got %s", wm.Get(wt.ID).Status)
	}

	// Verify the file exists in the repo.
	if _, err := os.Stat(filepath.Join(repo, "merged.txt")); err != nil {
		t.Fatalf("merged.txt not found after merge: %v", err)
	}
}

func TestMergeBackFastForward(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)
	wt, err := wm.Create("feature/ff-test", "main", "test fast-forward")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Make a commit in the worktree.
	newFile := filepath.Join(wt.Path, "ff.txt")
	if err := os.WriteFile(newFile, []byte("fast forward\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, wt.Path, "add", "ff.txt")
	runGitIn(t, wt.Path, "commit", "-m", "add ff.txt")

	// Remove worktree before merging.
	id := wt.ID
	if err := wm.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Re-register for merge.
	wm.mu.Lock()
	wm.Worktrees[id] = wt
	wt.Status = WorktreeComplete
	wm.mu.Unlock()

	if err := wm.MergeBack(id, "fast-forward"); err != nil {
		t.Fatalf("MergeBack fast-forward: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repo, "ff.txt")); err != nil {
		t.Fatalf("ff.txt not found after fast-forward merge: %v", err)
	}
}

func TestMergeBackSquash(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)
	wt, err := wm.Create("feature/squash-test", "main", "test squash")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Make multiple commits in the worktree.
	for i, name := range []string{"a.txt", "b.txt"} {
		f := filepath.Join(wt.Path, name)
		if err := os.WriteFile(f, []byte("content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGitIn(t, wt.Path, "add", name)
		runGitIn(t, wt.Path, "commit", "-m", strings.Repeat("commit ", i+1)+name)
	}

	id := wt.ID
	if err := wm.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	wm.mu.Lock()
	wm.Worktrees[id] = wt
	wt.Status = WorktreeComplete
	wm.mu.Unlock()

	if err := wm.MergeBack(id, "squash"); err != nil {
		t.Fatalf("MergeBack squash: %v", err)
	}

	// Both files should be present.
	for _, name := range []string{"a.txt", "b.txt"} {
		if _, err := os.Stat(filepath.Join(repo, name)); err != nil {
			t.Fatalf("%s not found after squash merge: %v", name, err)
		}
	}
}

func TestMergeBackInvalidStrategy(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)
	wt, err := wm.Create("feature/bad-strategy", "main", "bad strategy test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = wm.MergeBack(wt.ID, "rebase")
	if err == nil {
		t.Fatal("expected error for unsupported strategy")
	}
	if !strings.Contains(err.Error(), "unsupported merge strategy") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCleanup(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)

	wt1, _ := wm.Create("branch-cleanup-1", "main", "task 1")
	wt2, _ := wm.Create("branch-cleanup-2", "main", "task 2")
	wt3, _ := wm.Create("branch-cleanup-3", "main", "task 3")

	// Mark various statuses.
	wm.mu.Lock()
	wt1.Status = WorktreeComplete
	wt2.Status = WorktreeFailed
	// wt3 stays active.
	wm.mu.Unlock()

	if err := wm.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// wt1 and wt2 should be removed, wt3 should remain.
	if wm.Get(wt1.ID) != nil {
		t.Error("completed worktree should be removed after cleanup")
	}
	if wm.Get(wt2.ID) != nil {
		t.Error("failed worktree should be removed after cleanup")
	}
	if wm.Get(wt3.ID) == nil {
		t.Error("active worktree should remain after cleanup")
	}
}

func TestIsClean(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)
	wt, err := wm.Create("feature/clean-check", "main", "check cleanliness")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Initially clean.
	clean, err := wm.IsClean(wt.ID)
	if err != nil {
		t.Fatalf("IsClean: %v", err)
	}
	if !clean {
		t.Error("expected worktree to be clean initially")
	}

	// Create an uncommitted file.
	dirty := filepath.Join(wt.Path, "dirty.txt")
	if err := os.WriteFile(dirty, []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	clean, err = wm.IsClean(wt.ID)
	if err != nil {
		t.Fatalf("IsClean after dirty: %v", err)
	}
	if clean {
		t.Error("expected worktree to be dirty after adding untracked file")
	}

	// Unknown ID should error.
	_, err = wm.IsClean("nonexistent")
	if err == nil {
		t.Error("IsClean should error for unknown ID")
	}
}

func TestGetDiff(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)
	wt, err := wm.Create("feature/diff-test", "main", "diff testing")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Make a commit in the worktree.
	newFile := filepath.Join(wt.Path, "diffed.txt")
	if err := os.WriteFile(newFile, []byte("new content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, wt.Path, "add", "diffed.txt")
	runGitIn(t, wt.Path, "commit", "-m", "add diffed.txt")

	diff, err := wm.GetDiff(wt.ID)
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}

	if !strings.Contains(diff, "diffed.txt") {
		t.Errorf("diff should mention diffed.txt, got:\n%s", diff)
	}
	if !strings.Contains(diff, "new content") {
		t.Errorf("diff should contain file content, got:\n%s", diff)
	}

	// Unknown ID should error.
	_, err = wm.GetDiff("nonexistent")
	if err == nil {
		t.Error("GetDiff should error for unknown ID")
	}
}

func TestFormatMergePreview(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)
	wt, err := wm.Create("feature/preview-test", "main", "preview testing")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Make a commit.
	newFile := filepath.Join(wt.Path, "preview.txt")
	if err := os.WriteFile(newFile, []byte("preview content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, wt.Path, "add", "preview.txt")
	runGitIn(t, wt.Path, "commit", "-m", "add preview.txt for testing")

	preview := wm.FormatMergePreview(wt.ID)

	if !strings.Contains(preview, "Merge Preview:") {
		t.Errorf("preview should contain header, got:\n%s", preview)
	}
	if !strings.Contains(preview, "feature/preview-test") {
		t.Errorf("preview should mention branch, got:\n%s", preview)
	}
	if !strings.Contains(preview, "preview.txt") {
		t.Errorf("preview should mention changed file, got:\n%s", preview)
	}
	if !strings.Contains(preview, "add preview.txt") {
		t.Errorf("preview should show commit message, got:\n%s", preview)
	}

	// Unknown ID returns error message in output.
	unknownPreview := wm.FormatMergePreview("nonexistent")
	if !strings.Contains(unknownPreview, "not found") {
		t.Errorf("preview for unknown ID should say not found, got: %s", unknownPreview)
	}
}

func TestStatusReport(t *testing.T) {
	repo := initManagerTestRepo(t)
	defer os.RemoveAll(repo)

	wm := NewWorktreeManager(repo)

	// Empty report.
	report := wm.StatusReport()
	if !strings.Contains(report, "0/8") {
		t.Errorf("empty report should show 0/8, got:\n%s", report)
	}

	wm.Create("feature/auth", "main", "Implement JWT authentication")
	time.Sleep(10 * time.Millisecond)
	wt2, _ := wm.Create("fix/token-refresh", "main", "Fix token refresh race condition")

	wm.mu.Lock()
	wt2.Status = WorktreeComplete
	wm.mu.Unlock()

	report = wm.StatusReport()

	if !strings.Contains(report, "2/8") {
		t.Errorf("report should show 2/8, got:\n%s", report)
	}
	if !strings.Contains(report, "[active]") {
		t.Errorf("report should contain [active], got:\n%s", report)
	}
	if !strings.Contains(report, "[complete]") {
		t.Errorf("report should contain [complete], got:\n%s", report)
	}
	if !strings.Contains(report, "Implement JWT authentication") {
		t.Errorf("report should contain task description, got:\n%s", report)
	}
	if !strings.Contains(report, "Ready to merge") {
		t.Errorf("report should show Ready to merge for complete worktree, got:\n%s", report)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
	}
	for _, tc := range tests {
		got := formatDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// runGitIn runs a git command in the specified directory.
func runGitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=Hawk Test",
		"GIT_AUTHOR_EMAIL=test@hawk.dev",
		"GIT_COMMITTER_NAME=Hawk Test",
		"GIT_COMMITTER_EMAIL=test@hawk.dev",
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=commit.gpgsign",
		"GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=tag.gpgsign",
		"GIT_CONFIG_VALUE_1=false",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %s: %v", args, dir, out, err)
	}
}
