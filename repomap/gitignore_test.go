package repomap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGitignoreRules_NestedLoading(t *testing.T) {
	// Create a temp directory structure with nested .gitignore files
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	os.MkdirAll(sub, 0o755)

	// Root .gitignore
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\nbuild/\n"), 0o644)

	// Nested .gitignore
	os.WriteFile(filepath.Join(sub, ".gitignore"), []byte("*.tmp\n"), 0o644)

	gr := LoadGitignoreRules(sub)
	if gr == nil {
		t.Fatal("expected non-nil GitignoreRules")
	}

	// Should ignore *.log from root
	if !gr.ShouldIgnore("app.log") {
		t.Error("expected app.log to be ignored (from root .gitignore)")
	}

	// Should ignore *.tmp from nested
	if !gr.ShouldIgnore("cache.tmp") {
		t.Error("expected cache.tmp to be ignored (from sub .gitignore)")
	}

	// Should not ignore normal files
	if gr.ShouldIgnore("main.go") {
		t.Error("expected main.go to NOT be ignored")
	}
}

func TestGitignoreRules_Negation(t *testing.T) {
	root := t.TempDir()

	// Ignore all .log files but keep important.log
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n!important.log\n"), 0o644)

	gr := LoadGitignoreRules(root)

	if !gr.ShouldIgnore("debug.log") {
		t.Error("expected debug.log to be ignored")
	}
	if gr.ShouldIgnore("important.log") {
		t.Error("expected important.log to NOT be ignored (negation rule)")
	}
}

func TestGitignoreRules_DirectoryPatterns(t *testing.T) {
	root := t.TempDir()

	// The pattern "build/" means directory only; for basename matching
	// our implementation checks the base name
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("build\nnode_modules\n"), 0o644)

	gr := LoadGitignoreRules(root)

	if !gr.ShouldIgnore("build") {
		t.Error("expected 'build' to be ignored")
	}
	if !gr.ShouldIgnore("node_modules") {
		t.Error("expected 'node_modules' to be ignored")
	}
	if gr.ShouldIgnore("src") {
		t.Error("expected 'src' to NOT be ignored")
	}
}

func TestGitignoreRules_CommentsAndEmptyLines(t *testing.T) {
	root := t.TempDir()

	content := `# This is a comment
*.log

# Another comment

*.tmp
`
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte(content), 0o644)

	gr := LoadGitignoreRules(root)

	// Comments and empty lines should be skipped; only *.log and *.tmp effective
	if !gr.ShouldIgnore("app.log") {
		t.Error("expected app.log to be ignored")
	}
	if !gr.ShouldIgnore("cache.tmp") {
		t.Error("expected cache.tmp to be ignored")
	}
	if gr.ShouldIgnore("main.go") {
		t.Error("expected main.go to NOT be ignored")
	}

	// Verify nil rules are safe
	var nilRules *GitignoreRules
	if nilRules.ShouldIgnore("anything") {
		t.Error("nil GitignoreRules should not ignore anything")
	}
}
