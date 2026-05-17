package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates a temporary git repo and changes into it.
// The caller should defer the returned cleanup function.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v failed: %s (%v)", args, out, err)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	// Create an initial commit so HEAD exists.
	initial := filepath.Join(dir, "README")
	os.WriteFile(initial, []byte("init"), 0o644)
	run("git", "add", "README")
	run("git", "commit", "-m", "initial commit")

	return dir, func() { os.Chdir(origDir) }
}

func TestIsGitRepo(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()
	_ = dir

	if !IsGitRepo() {
		t.Fatal("expected IsGitRepo() == true inside test repo")
	}
}

func TestAutoCommitAndRevert(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create a file and auto-commit it.
	file := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(file, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AutoCommit(file, "Write", "wrote file"); err != nil {
		t.Fatalf("AutoCommit: %v", err)
	}

	hash := LastAutoCommitHash()
	if hash == "" {
		t.Fatal("expected non-empty LastAutoCommitHash()")
	}

	// Verify the commit message.
	msg, err := gitHeadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "hawk: Write hello.txt") {
		t.Fatalf("unexpected commit message: %q", msg)
	}

	// Revert the auto-commit.
	if err := RevertLastAutoCommit(); err != nil {
		t.Fatalf("RevertLastAutoCommit: %v", err)
	}

	// After revert, HEAD should be a revert commit.
	msg2, err := gitHeadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg2, "Revert") {
		t.Fatalf("expected revert commit message, got: %q", msg2)
	}
}

func TestRevertNonHawkCommitFails(t *testing.T) {
	_, cleanup := setupTestRepo(t)
	defer cleanup()

	// HEAD is "initial commit" — not a hawk commit.
	if err := RevertLastAutoCommit(); err == nil {
		t.Fatal("expected error reverting non-hawk commit")
	}
}

func TestAutoCommitOutsideGitRepo(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	err := AutoCommit("/tmp/nonexistent", "Write", "test")
	if err == nil {
		t.Fatal("expected error outside git repo")
	}
}
