package repomap

import (
	"testing"
)

func TestShouldIndex_ExcludePatterns(t *testing.T) {
	p := IndexPatterns{
		Include: []string{},
		Exclude: []string{"*.min.js", "*_test.go", "vendor/**"},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"app.min.js", false},
		{"handler_test.go", false},
		{"src/utils.go", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := p.ShouldIndex(tt.path); got != tt.want {
				t.Errorf("ShouldIndex(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestShouldIndex_IncludePatterns(t *testing.T) {
	p := IndexPatterns{
		Include: []string{"*.go", "*.py"},
		Exclude: []string{"*_test.go"},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"app.py", true},
		{"handler_test.go", false},     // excluded even though *.go matches
		{"styles.css", false},           // not in include list
		{"README.md", false},            // not in include list
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := p.ShouldIndex(tt.path); got != tt.want {
				t.Errorf("ShouldIndex(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDefaultIndexPatterns(t *testing.T) {
	p := DefaultIndexPatterns()

	if len(p.Include) != 0 {
		t.Errorf("expected empty include list, got %v", p.Include)
	}
	if len(p.Exclude) == 0 {
		t.Error("expected non-empty exclude list")
	}

	// Default excludes should block common generated/lock files
	if p.ShouldIndex("go.sum") {
		t.Error("expected go.sum to be excluded by default")
	}
	if p.ShouldIndex("package-lock.json") {
		t.Error("expected package-lock.json to be excluded by default")
	}
	if p.ShouldIndex("handler_test.go") {
		t.Error("expected *_test.go to be excluded by default")
	}

	// Default should allow normal code files
	if !p.ShouldIndex("main.go") {
		t.Error("expected main.go to be allowed by default")
	}
}

func TestShouldIndex_EmptyPatterns(t *testing.T) {
	p := IndexPatterns{
		Include: []string{},
		Exclude: []string{},
	}

	// Everything should be indexed with no patterns
	if !p.ShouldIndex("anything.txt") {
		t.Error("expected all files to be indexed with empty patterns")
	}
	if !p.ShouldIndex("vendor/lib.go") {
		t.Error("expected all files to be indexed with empty patterns")
	}
}
