package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandContextReferencesInline(t *testing.T) {
	dir := t.TempDir()
	ref := filepath.Join(dir, "guidelines.md")
	_ = os.WriteFile(ref, []byte("## Guidelines\n- always test\n"), 0o600)

	content := "# Project\n@guidelines.md\n"
	root := gitRoot(dir)
	expanded := expandContextReferences(content, dir, root)
	if !strings.Contains(expanded, "- always test") {
		t.Fatalf("referenced file content was not inlined: %q", expanded)
	}
	if strings.Contains(expanded, "@guidelines.md") {
		t.Fatalf("reference line should be replaced: %q", expanded)
	}
	if !strings.Contains(expanded, "## Guidelines") {
		t.Fatalf("referenced file should be inlined: %q", expanded)
	}
}

func TestExpandContextReferencesRefusesEscape(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "repo")
	_ = os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	secret := filepath.Join(dir, "secret.txt")
	_ = os.WriteFile(secret, []byte("top secret"), 0o600)

	// A reference that escapes the git root must be refused.
	content := "@../secret.txt\n"
	expanded := expandContextReferences(content, filepath.Join(root, "sub"), root)
	if strings.Contains(expanded, "top secret") {
		t.Fatalf("out-of-root reference must not be inlined: %q", expanded)
	}
	if !strings.Contains(expanded, "reference skipped") {
		t.Fatalf("expected a skipped-reference notice: %q", expanded)
	}
}

func TestExpandContextReferencesDepthBudget(t *testing.T) {
	dir := t.TempDir()
	// Chain of references deeper than maxRefDepth must terminate.
	for i := 0; i < maxRefDepth+2; i++ {
		next := filepath.Join(dir, "a"+string(rune('0'+i))+".md")
		link := filepath.Join(dir, "a"+string(rune('0'+i+1))+".md")
		if i+1 <= maxRefDepth+1 {
			_ = os.WriteFile(next, []byte("@a"+string(rune('0'+i+1))+".md\n"), 0o600)
		} else {
			_ = os.WriteFile(next, []byte("leaf\n"), 0o600)
		}
		_ = link
	}
	// The deepest reference should be bounded; verify it terminates without
	// infinite recursion.
	content := "@a0.md\n"
	expanded := expandContextReferences(content, dir, gitRoot(dir))
	_ = expanded // must terminate
}
