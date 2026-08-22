package gitsnapshot

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGit is a test helper that runs git in a directory.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// setupRepo creates a temp git repo with some committed files and returns its dir.
func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "init")
	return dir
}

func TestNewCreatesLinkedRepo(t *testing.T) {
	src := setupRepo(t)
	snap := filepath.Join(t.TempDir(), "snap")
	m, err := New(src, snap)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := os.Stat(filepath.Join(snap, ".git", "objects", "info", "alternates")); err != nil {
		t.Fatalf("alternates file missing: %v", err)
	}
	// The alternates file must reference the source object db.
	b, err := os.ReadFile(filepath.Join(snap, ".git", "objects", "info", "alternates"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "objects") {
		t.Fatalf("alternates = %q", string(b))
	}
	_ = m
}

func TestCaptureReturnsTreeID(t *testing.T) {
	src := setupRepo(t)
	m, err := New(src, filepath.Join(t.TempDir(), "snap"))
	if err != nil {
		t.Fatal(err)
	}
	tid, err := m.Capture(context.Background(), nil)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if tid == "" {
		t.Fatal("empty tree id")
	}
}

func TestDiffDetectsChanges(t *testing.T) {
	src := setupRepo(t)
	m, _ := New(src, filepath.Join(t.TempDir(), "snap"))
	ctx := context.Background()

	base, err := m.Capture(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Modify a.txt and add c.txt.
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("alpha2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "c.txt"), []byte("charlie\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now, err := m.Capture(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	changes, err := m.Diff(ctx, base, now)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	byPath := map[string]string{}
	for _, c := range changes {
		byPath[c.Path] = c.Status
	}
	if byPath["a.txt"] != "modified" {
		t.Fatalf("a.txt status = %q, want modified; changes=%+v", byPath["a.txt"], changes)
	}
	if byPath["c.txt"] != "added" {
		t.Fatalf("c.txt status = %q, want added", byPath["c.txt"])
	}
}

func TestWriteIfUnchangedSuccessAndStale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("expected"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Manager{}
	if err := m.WriteIfUnchanged(path, []byte("expected"), []byte("new")); err != nil {
		t.Fatalf("WriteIfUnchanged: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, []byte("new")) {
		t.Fatalf("content = %q, want new", got)
	}

	// Now the file is "new"; writing with stale expected must fail.
	err := m.WriteIfUnchanged(path, []byte("expected"), []byte("other"))
	if err == nil {
		t.Fatal("expected stale content error")
	}
	if _, ok := err.(*StaleContentError); !ok {
		t.Fatalf("err = %T, want *StaleContentError", err)
	}
}

func TestRestoreChecksOutAndDeletes(t *testing.T) {
	src := setupRepo(t)
	m, _ := New(src, filepath.Join(t.TempDir(), "snap"))
	ctx := context.Background()

	base, err := m.Capture(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Modify and capture a new tree.
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "c.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now, err := m.Capture(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Restore a.txt from base (revert the change) and delete c.txt (absent in base).
	if err := m.Restore(ctx, base, []string{"a.txt", "c.txt"}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(src, "a.txt"))
	if string(got) != "alpha\n" {
		t.Fatalf("a.txt = %q, want alpha", got)
	}
	if _, err := os.Stat(filepath.Join(src, "c.txt")); !os.IsNotExist(err) {
		t.Fatalf("c.txt should be deleted, got %v", err)
	}
	_ = now
}

func TestPreviewDetectsModificationWithoutTouchingWorktree(t *testing.T) {
	src := setupRepo(t)
	m, _ := New(src, filepath.Join(t.TempDir(), "snap"))
	ctx := context.Background()

	base, err := m.Capture(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Modify a.txt after capturing the baseline.
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Preview the worktree against the baseline for a.txt only.
	changes, err := m.Preview(ctx, base, []string{"a.txt"})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(changes) != 1 || changes[0].Path != "a.txt" || changes[0].Status != "modified" {
		t.Fatalf("changes = %+v, want single modified a.txt", changes)
	}

	// The worktree file must be untouched by the preview.
	got, _ := os.ReadFile(filepath.Join(src, "a.txt"))
	if string(got) != "modified\n" {
		t.Fatalf("preview mutated worktree: %q", got)
	}
}

func TestPreviewNoChangeWhenFileMatchesTree(t *testing.T) {
	src := setupRepo(t)
	m, _ := New(src, filepath.Join(t.TempDir(), "snap"))
	ctx := context.Background()

	base, err := m.Capture(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := m.Preview(ctx, base, []string{"a.txt", "b.txt"})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %+v", changes)
	}
}
