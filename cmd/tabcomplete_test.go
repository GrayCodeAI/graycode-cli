package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/hawk/tool"
)

func TestFilePathCompletions_CurrentDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "alpha.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "beta.go"), []byte("package main"), 0o644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0o755)

	matches := filePathCompletions(filepath.Join(dir, "a"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for 'a' prefix, got %d: %v", len(matches), matches)
	}
	if filepath.Base(matches[0]) != "alpha.go" {
		t.Fatalf("expected alpha.go, got %s", matches[0])
	}
}

func TestFilePathCompletions_Empty(t *testing.T) {
	dir := t.TempDir()
	// Empty directory should return nothing meaningful
	matches := filePathCompletions(filepath.Join(dir, "nonexistent"))
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches for nonexistent prefix, got %d", len(matches))
	}
}

func TestFilePathCompletions_DirectorySlash(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644)

	matches := filePathCompletions(dir + string(filepath.Separator))
	found := false
	for _, m := range matches {
		if filepath.Base(m) == "file.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected file.txt in matches: %v", matches)
	}
}

func TestToolNameCompletions(t *testing.T) {
	registry := tool.NewRegistry(tool.BashTool{}, tool.LSTool{})

	matches := toolNameCompletions("B", registry)
	if len(matches) != 1 || matches[0] != "Bash" {
		t.Fatalf("expected [Bash], got %v", matches)
	}

	matches = toolNameCompletions("", registry)
	if len(matches) != 0 {
		t.Fatalf("expected no matches for empty partial, got %v", matches)
	}

	matches = toolNameCompletions("Z", registry)
	if len(matches) != 0 {
		t.Fatalf("expected no matches for Z prefix, got %v", matches)
	}
}

func TestToolNameCompletions_NilRegistry(t *testing.T) {
	matches := toolNameCompletions("B", nil)
	if matches != nil {
		t.Fatalf("expected nil for nil registry, got %v", matches)
	}
}
