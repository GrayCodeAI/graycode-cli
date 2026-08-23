package fuzzyfind

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeTree(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestExactBasenameHighestScore(t *testing.T) {
	root := makeTree(
		t,
		"src/config.go",
		"src/config_test.go",
		"internal/app/config_loader.go",
		"docs/readme.md",
	)
	f, _ := New(root)
	matches := f.Search("config.go", 10)
	if len(matches) == 0 || matches[0].Path != "src/config.go" {
		t.Fatalf("top = %+v", matches)
	}
	if matches[0].Score <= matches[1].Score {
		t.Fatalf("exact basename should outscore partial: %+v", matches[:2])
	}
}

func TestSubstringAndMultiWordQuery(t *testing.T) {
	root := makeTree(
		t,
		"internal/engine/cache_gate.go",
		"internal/engine/compact.go",
		"web/gate.json",
	)
	f, _ := New(root)
	matches := f.Search("cache gate", 5)
	if len(matches) == 0 || matches[0].Path != "internal/engine/cache_gate.go" {
		t.Fatalf("multi-word top = %+v", matches)
	}
}

func TestEmptyQueryReturnsNothing(t *testing.T) {
	root := makeTree(t, "a.txt")
	f, _ := New(root)
	if got := f.Search("", 10); len(got) != 0 {
		t.Fatal("empty query should return nothing")
	}
}

func TestSkipDirsExcluded(t *testing.T) {
	root := makeTree(t, "vendor/lib.go", "src/main.go")
	f, _ := New(root)
	matches := f.Search("lib", 20)
	for _, m := range matches {
		if filepath.HasPrefix(m.Path, "vendor") {
			t.Fatal("vendor leaked into results")
		}
	}
}

func TestLimitK(t *testing.T) {
	files := make([]string, 30)
	for i := range files {
		files[i] = filepath.Join("dir", stringsRepeat("f", i+1)+".go")
	}
	root := makeTree(t, files...)
	f, _ := New(root)
	matches := f.Search("f", 3)
	if len(matches) != 3 {
		t.Fatalf("k limit = %d, want 3", len(matches))
	}
}

func TestNewNotDirectory(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	os.WriteFile(p, []byte("x"), 0o644)
	if _, err := New(p); err == nil {
		t.Fatal("expected error for non-dir root")
	}
}

func TestMatchAbbrevCamelCase(t *testing.T) {
	if !matchAbbrev("internal/engine/CachePlanner.go", "CP") {
		t.Fatal("camel-case abbreviation not matched")
	}
	if matchAbbrev("internal/engine/compact.go", "XYZ") {
		t.Fatal("false positive abbreviation match")
	}
}

func stringsRepeat(s string, n int) string {
	return strings.Repeat(s, n)
}
