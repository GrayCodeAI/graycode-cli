package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRunWorkspaceChecksDetectsGoProject(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/checktest\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package checktest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// runWorkspaceChecks detects from ".", so run it with the working dir set.
	t.Chdir(dir)

	results := runWorkspaceChecks()
	if len(results) == 0 {
		t.Fatal("expected at least one discovered check for a Go project")
	}
	found := false
	for _, r := range results {
		if r.Name == "Go tests" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a 'Go tests' check, got %+v", results)
	}
}

func TestRunWorkspaceChecksEmptyDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// An empty dir has no detected runner; Detect returns no checks.
	for _, r := range runWorkspaceChecks() {
		if r.Err != nil {
			t.Fatalf("unexpected error in empty dir: %v", r.Err)
		}
	}
}
