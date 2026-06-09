package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupTestRepo creates a temporary git repo with some commits for testing.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "git-context-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	cleanup := func() { os.RemoveAll(dir) }

	runInDir := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=Alice",
			"GIT_AUTHOR_EMAIL=alice@example.com",
			"GIT_COMMITTER_NAME=Alice",
			"GIT_COMMITTER_EMAIL=alice@example.com",
			"GIT_CONFIG_COUNT=2",
			"GIT_CONFIG_KEY_0=commit.gpgsign",
			"GIT_CONFIG_VALUE_0=false",
			"GIT_CONFIG_KEY_1=tag.gpgsign",
			"GIT_CONFIG_VALUE_1=false",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %v\noutput: %s", args, err, out)
		}
	}

	runInDirAs := func(author, email string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME="+author,
			"GIT_AUTHOR_EMAIL="+email,
			"GIT_COMMITTER_NAME="+author,
			"GIT_COMMITTER_EMAIL="+email,
			"GIT_CONFIG_COUNT=2",
			"GIT_CONFIG_KEY_0=commit.gpgsign",
			"GIT_CONFIG_VALUE_0=false",
			"GIT_CONFIG_KEY_1=tag.gpgsign",
			"GIT_CONFIG_VALUE_1=false",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %v\noutput: %s", args, err, out)
		}
	}

	// Initialize repo
	runInDir("git", "init")
	runInDir("git", "checkout", "-b", "main")

	// Create initial file and commit
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		cleanup()
		t.Fatalf("write file: %v", err)
	}
	runInDir("git", "add", "main.go")
	runInDir("git", "commit", "-m", "initial commit")

	// Add another file by Bob
	if err := os.WriteFile(filepath.Join(dir, "utils.go"), []byte("package main\n\nfunc helper() {}\n"), 0o644); err != nil {
		cleanup()
		t.Fatalf("write file: %v", err)
	}
	runInDirAs("Bob", "bob@example.com", "git", "add", "utils.go")
	runInDirAs("Bob", "bob@example.com", "git", "commit", "-m", "add utils helper")

	// Modify main.go again (by Alice)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"), 0o644); err != nil {
		cleanup()
		t.Fatalf("write file: %v", err)
	}
	runInDir("git", "add", "main.go")
	runInDir("git", "commit", "-m", "add hello print")

	// Add related file (modified together with main.go)
	if err := os.WriteFile(filepath.Join(dir, "config.go"), []byte("package main\n\nvar cfg = \"default\"\n"), 0o644); err != nil {
		cleanup()
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(cfg)\n}\n"), 0o644); err != nil {
		cleanup()
		t.Fatalf("write file: %v", err)
	}
	runInDir("git", "add", ".")
	runInDir("git", "commit", "-m", "add config and update main")

	// One more commit touching both main.go and config.go
	if err := os.WriteFile(filepath.Join(dir, "config.go"), []byte("package main\n\nvar cfg = \"production\"\n"), 0o644); err != nil {
		cleanup()
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"app:\", cfg)\n}\n"), 0o644); err != nil {
		cleanup()
		t.Fatalf("write file: %v", err)
	}
	runInDir("git", "add", ".")
	runInDir("git", "commit", "-m", "update config to production")

	// Create a feature branch
	runInDir("git", "checkout", "-b", "feature/auth-v2")

	// Add a commit on the feature branch
	if err := os.WriteFile(filepath.Join(dir, "auth.go"), []byte("package main\n\nfunc authenticate() bool {\n\treturn true\n}\n"), 0o644); err != nil {
		cleanup()
		t.Fatalf("write file: %v", err)
	}
	runInDir("git", "add", "auth.go")
	runInDir("git", "commit", "-m", "add authentication")

	return dir, cleanup
}

func TestGetBranch(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	gc := NewGitContext(dir)
	branch, err := gc.GetBranch()
	if err != nil {
		t.Fatalf("GetBranch error: %v", err)
	}
	if branch != "feature/auth-v2" {
		t.Errorf("expected branch 'feature/auth-v2', got '%s'", branch)
	}
}

func TestGetUncommitted(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	gc := NewGitContext(dir)

	// Initially clean
	files, err := gc.GetUncommitted()
	if err != nil {
		t.Fatalf("GetUncommitted error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected no uncommitted files, got %v", files)
	}

	// Modify a file
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\n// modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err = gc.GetUncommitted()
	if err != nil {
		t.Fatalf("GetUncommitted error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 uncommitted file, got %d: %v", len(files), files)
	}
	if files[0] != "main.go" {
		t.Errorf("expected 'main.go', got '%s'", files[0])
	}
}

func TestGetRecentChanges(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	gc := NewGitContext(dir)
	commits, err := gc.GetRecentChanges(30)
	if err != nil {
		t.Fatalf("GetRecentChanges error: %v", err)
	}
	// We created 6 commits in setup
	if len(commits) < 5 {
		t.Errorf("expected at least 5 recent commits, got %d", len(commits))
	}
	// Check that commits have expected fields
	for _, c := range commits {
		if c.Hash == "" {
			t.Error("commit hash should not be empty")
		}
		if c.Author == "" {
			t.Error("commit author should not be empty")
		}
		if c.Message == "" {
			t.Error("commit message should not be empty")
		}
	}
}

func TestGetFileInfo(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	gc := NewGitContext(dir)
	info, err := gc.GetFileInfo("main.go")
	if err != nil {
		t.Fatalf("GetFileInfo error: %v", err)
	}

	if info.Path != "main.go" {
		t.Errorf("expected path 'main.go', got '%s'", info.Path)
	}
	if info.LastAuthor != "Alice" {
		t.Errorf("expected last author 'Alice', got '%s'", info.LastAuthor)
	}
	if info.CommitCount < 4 {
		t.Errorf("expected at least 4 commits, got %d", info.CommitCount)
	}
	if len(info.Contributors) == 0 {
		t.Error("expected at least one contributor")
	}
	if info.Contributors[0] != "Alice" {
		t.Errorf("expected top contributor 'Alice', got '%s'", info.Contributors[0])
	}
	if info.LastModified.IsZero() {
		t.Error("last modified should not be zero")
	}
}

func TestBuildContextForFile(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	gc := NewGitContext(dir)
	ctx := gc.BuildContextForFile("main.go")

	if !strings.Contains(ctx, "## Git Context for main.go") {
		t.Error("context should contain file header")
	}
	if !strings.Contains(ctx, "Last modified:") {
		t.Error("context should contain last modified info")
	}
	if !strings.Contains(ctx, "@Alice") {
		t.Error("context should contain author name")
	}
	if !strings.Contains(ctx, "Contributors:") {
		t.Error("context should contain contributors section")
	}
	if !strings.Contains(ctx, "Branch: feature/auth-v2") {
		t.Error("context should contain branch name")
	}
	if !strings.Contains(ctx, "Recent changes") {
		t.Error("context should contain recent changes")
	}
}

func TestBuildContextForSession(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	gc := NewGitContext(dir)
	ctx := gc.BuildContextForSession()

	if !strings.Contains(ctx, "## Repository Context") {
		t.Error("session context should contain header")
	}
	if !strings.Contains(ctx, "Branch: feature/auth-v2") {
		t.Error("session context should contain branch info")
	}
	if !strings.Contains(ctx, "Uncommitted:") {
		t.Error("session context should contain uncommitted info")
	}
	if !strings.Contains(ctx, "Recent activity:") {
		t.Error("session context should contain recent activity")
	}
	if !strings.Contains(ctx, "Active contributors:") {
		t.Error("session context should contain active contributors")
	}
}

func TestGetRelatedFiles(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	gc := NewGitContext(dir)
	related, err := gc.GetRelatedFiles("main.go")
	if err != nil {
		t.Fatalf("GetRelatedFiles error: %v", err)
	}

	// config.go was modified together with main.go in 2 commits
	found := false
	for _, f := range related {
		if f == "config.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected config.go in related files, got %v", related)
	}
}

func TestCacheReusesResults(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	gc := NewGitContext(dir)

	// First call populates cache
	info1, err := gc.GetFileInfo("main.go")
	if err != nil {
		t.Fatalf("GetFileInfo error: %v", err)
	}

	// Second call should use cache
	info2, err := gc.GetFileInfo("main.go")
	if err != nil {
		t.Fatalf("GetFileInfo error: %v", err)
	}

	// Should be the same pointer (from cache)
	if info1 != info2 {
		t.Error("expected cached result to return same pointer")
	}
}

func TestNonGitDirectory(t *testing.T) {
	dir, err := os.MkdirTemp("", "non-git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	gc := NewGitContext(dir)

	// GetBranch should return error
	_, err = gc.GetBranch()
	if err == nil {
		t.Error("expected error for non-git directory")
	}

	// GetFileInfo should return error
	_, err = gc.GetFileInfo("nonexistent.go")
	if err == nil {
		t.Error("expected error for non-git directory")
	}

	// GetUncommitted should return error
	_, err = gc.GetUncommitted()
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestIsRecentlyModified(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	gc := NewGitContext(dir)

	// main.go was just committed, should be recent
	if !gc.IsRecentlyModified("main.go", 1*time.Hour) {
		t.Error("main.go should be recently modified (within 1 hour)")
	}

	// Should not be modified "within 0 seconds ago" effectively
	if gc.IsRecentlyModified("main.go", 0) {
		t.Error("main.go should not be modified within 0 duration")
	}

	// Non-existent file should return false
	if gc.IsRecentlyModified("nonexistent.go", 24*time.Hour) {
		t.Error("nonexistent file should not be recently modified")
	}
}

func TestGetBlame(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	gc := NewGitContext(dir)

	// Blame lines 1-3 of main.go
	blameLines, err := gc.GetBlame("main.go", 1, 3)
	if err != nil {
		t.Fatalf("GetBlame error: %v", err)
	}

	if len(blameLines) == 0 {
		t.Fatal("expected blame lines, got none")
	}

	for _, bl := range blameLines {
		if bl.Commit == "" {
			t.Error("blame line commit should not be empty")
		}
		if bl.Author == "" {
			t.Error("blame line author should not be empty")
		}
		if bl.LineNo < 1 {
			t.Errorf("blame line number should be >= 1, got %d", bl.LineNo)
		}
	}
}

func TestGetDiffSummary(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	gc := NewGitContext(dir)

	// Clean state
	summary, err := gc.GetDiffSummary()
	if err != nil {
		t.Fatalf("GetDiffSummary error: %v", err)
	}
	if !strings.Contains(summary, "No uncommitted changes") {
		t.Errorf("expected 'No uncommitted changes', got: %s", summary)
	}

	// Modify a file
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\n// changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err = gc.GetDiffSummary()
	if err != nil {
		t.Fatalf("GetDiffSummary error: %v", err)
	}
	if !strings.Contains(summary, "Unstaged changes") {
		t.Errorf("expected 'Unstaged changes' in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "main.go") {
		t.Errorf("expected 'main.go' in diff summary, got: %s", summary)
	}
}

func TestFormatTimeAgo(t *testing.T) {
	tests := []struct {
		input    time.Time
		contains string
	}{
		{time.Now().Add(-30 * time.Second), "just now"},
		{time.Now().Add(-5 * time.Minute), "5m ago"},
		{time.Now().Add(-3 * time.Hour), "3h ago"},
		{time.Now().Add(-2 * 24 * time.Hour), "2d ago"},
		{time.Time{}, "unknown"},
	}

	for _, tt := range tests {
		result := formatTimeAgo(tt.input)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("formatTimeAgo(%v) = %q, want containing %q", tt.input, result, tt.contains)
		}
	}
}
