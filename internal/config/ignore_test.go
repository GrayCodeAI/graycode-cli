package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultIgnorePatterns(t *testing.T) {
	patterns := DefaultIgnorePatterns()
	if len(patterns) != 8 {
		t.Fatalf("expected 8 default patterns, got %d", len(patterns))
	}
	expected := map[string]bool{
		"node_modules": true,
		".git":         true,
		"__pycache__":  true,
		".venv":        true,
		"dist":         true,
		"build":        true,
		"*.pyc":        true,
		".DS_Store":    true,
	}
	for _, p := range patterns {
		if !expected[p] {
			t.Errorf("unexpected default pattern: %q", p)
		}
	}
}

func TestLoadIgnorePatterns_NoFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	patterns := LoadIgnorePatterns()
	if len(patterns) != 8 {
		t.Fatalf("expected defaults when no file, got %d patterns", len(patterns))
	}
}

func TestLoadIgnorePatterns_HawkIgnoreFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	content := "vendor\n# comment\n\n*.log\ntmp\n"
	os.WriteFile(filepath.Join(dir, ".hawkignore"), []byte(content), 0o644)

	patterns := LoadIgnorePatterns()
	if len(patterns) != 3 {
		t.Fatalf("expected 3 patterns, got %d: %v", len(patterns), patterns)
	}
	if patterns[0] != "vendor" || patterns[1] != "*.log" || patterns[2] != "tmp" {
		t.Fatalf("unexpected patterns: %v", patterns)
	}
}

func TestLoadIgnorePatterns_DotHawkIgnore(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll(filepath.Join(dir, ".hawk"), 0o755)
	os.WriteFile(filepath.Join(dir, ".hawk", "ignore"), []byte("custom_dir\n"), 0o644)

	patterns := LoadIgnorePatterns()
	if len(patterns) != 1 || patterns[0] != "custom_dir" {
		t.Fatalf("expected [custom_dir], got %v", patterns)
	}
}

func TestShouldIgnore_ExactMatch(t *testing.T) {
	patterns := []string{"node_modules", ".git", "*.pyc"}
	tests := []struct {
		path     string
		expected bool
	}{
		{"node_modules", true},
		{"src/node_modules/pkg", true},
		{".git", true},
		{"repo/.git/config", true},
		{"main.pyc", true},
		{"src/main.pyc", true},
		{"main.py", false},
		{"src/main.go", false},
		{"readme.md", false},
	}
	for _, tt := range tests {
		got := ShouldIgnore(tt.path, patterns)
		if got != tt.expected {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestShouldIgnore_GlobPattern(t *testing.T) {
	patterns := []string{"*.log", "*.tmp"}
	tests := []struct {
		path     string
		expected bool
	}{
		{"app.log", true},
		{"debug.tmp", true},
		{"src/debug.tmp", true},
		{"app.txt", false},
	}
	for _, tt := range tests {
		got := ShouldIgnore(tt.path, patterns)
		if got != tt.expected {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestShouldIgnore_PathWithSlash(t *testing.T) {
	patterns := []string{"vendor/cache"}
	tests := []struct {
		path     string
		expected bool
	}{
		{"vendor/cache", true},
		{"vendor/other", false},
		{"src/vendor/cache", false}, // pattern with slash matches from root
	}
	for _, tt := range tests {
		got := ShouldIgnore(tt.path, patterns)
		if got != tt.expected {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestShouldIgnore_EmptyPatterns(t *testing.T) {
	if ShouldIgnore("anything", nil) {
		t.Fatal("expected false for nil patterns")
	}
	if ShouldIgnore("anything", []string{}) {
		t.Fatal("expected false for empty patterns")
	}
}

func TestShouldIgnore_DotDSStore(t *testing.T) {
	patterns := DefaultIgnorePatterns()
	if !ShouldIgnore(".DS_Store", patterns) {
		t.Fatal("expected .DS_Store to be ignored")
	}
	if !ShouldIgnore("subdir/.DS_Store", patterns) {
		t.Fatal("expected subdir/.DS_Store to be ignored")
	}
}
