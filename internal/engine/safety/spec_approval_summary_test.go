package safety

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecApprovalSummary_EmptySlug(t *testing.T) {
	if got := specApprovalSummary(""); got != "ApproveImplementation" {
		t.Errorf("got %q, want fallback name", got)
	}
}

func TestSpecApprovalSummary_ReadsWrittenFiles(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	specDir := filepath.Join(dir, ".hawk", "specs", "my-task")
	if err := os.MkdirAll(specDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "spec.md"), []byte("the spec content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "plan.md"), []byte("the plan content"), 0o600); err != nil {
		t.Fatal(err)
	}
	// tasks.md deliberately not written, to confirm partial content still works.

	got := specApprovalSummary("my-task")
	if !strings.Contains(got, "the spec content") {
		t.Errorf("summary missing spec content: %q", got)
	}
	if !strings.Contains(got, "the plan content") {
		t.Errorf("summary missing plan content: %q", got)
	}
	if strings.Contains(got, "tasks.md") {
		t.Errorf("summary should not mention tasks.md when it wasn't written: %q", got)
	}
}

func TestSpecApprovalSummary_NoFilesFallsBack(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if got := specApprovalSummary("nonexistent-slug"); got != "ApproveImplementation" {
		t.Errorf("got %q, want fallback name", got)
	}
}
