package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadGitState_ValidRepo(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755)

	// Write HEAD pointing to refs/heads/main
	os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)

	// Write commit hash for main
	commitHash := "abc123def456789012345678901234567890abcd"
	os.WriteFile(filepath.Join(gitDir, "refs", "heads", "main"), []byte(commitHash+"\n"), 0o644)

	state, err := ReadGitState(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Branch != "main" {
		t.Fatalf("expected branch main, got %s", state.Branch)
	}
	if state.Commit != commitHash {
		t.Fatalf("expected commit %s, got %s", commitHash, state.Commit)
	}
	if state.Worktree {
		t.Fatal("expected worktree=false")
	}
}

func TestReadGitState_DetachedHead(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(gitDir, 0o755)

	commitHash := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(commitHash+"\n"), 0o644)

	state, err := ReadGitState(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Branch != "(detached)" {
		t.Fatalf("expected (detached), got %s", state.Branch)
	}
	if state.Commit != commitHash {
		t.Fatalf("expected commit %s, got %s", commitHash, state.Commit)
	}
}

func TestReadGitState_NotARepo(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadGitState(dir)
	if err == nil {
		t.Fatal("expected error for non-repo directory")
	}
}

func TestParsePackedRefs(t *testing.T) {
	dir := t.TempDir()
	content := `# pack-refs with: peeled fully-peeled sorted
abc123def456789012345678901234567890abcd refs/heads/main
def456789012345678901234567890abcd1234 refs/heads/feature
^abc123def456789012345678901234567890abcd
`
	os.WriteFile(filepath.Join(dir, "packed-refs"), []byte(content), 0o644)

	refs, err := parsePackedRefs(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs["refs/heads/main"] != "abc123def456789012345678901234567890abcd" {
		t.Fatalf("wrong hash for refs/heads/main: %s", refs["refs/heads/main"])
	}
}

func TestReadGitState_Worktree(t *testing.T) {
	dir := t.TempDir()
	realGitDir := filepath.Join(dir, "real-git")
	os.MkdirAll(filepath.Join(realGitDir, "refs", "heads"), 0o755)
	os.WriteFile(filepath.Join(realGitDir, "HEAD"), []byte("ref: refs/heads/develop\n"), 0o644)
	os.WriteFile(filepath.Join(realGitDir, "refs", "heads", "develop"), []byte("1234567890abcdef1234567890abcdef12345678\n"), 0o644)

	worktreeDir := filepath.Join(dir, "worktree")
	os.MkdirAll(worktreeDir, 0o755)
	os.WriteFile(filepath.Join(worktreeDir, ".git"), []byte("gitdir: "+realGitDir+"\n"), 0o644)

	state, err := ReadGitState(worktreeDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !state.Worktree {
		t.Fatal("expected worktree=true")
	}
	if state.Branch != "develop" {
		t.Fatalf("expected branch develop, got %s", state.Branch)
	}
}
