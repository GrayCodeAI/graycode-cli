package context

import (
	"testing"
)

func TestParseFrontmatter_WithGlobs(t *testing.T) {
	content := `---
description: TypeScript defaults
globs: ["**/*.ts", "**/*.tsx"]
alwaysApply: false
---

Prefer strict TypeScript.`

	fm, body := ParseFrontmatter(content)
	if fm == nil {
		t.Fatal("expected frontmatter")
	}
	if fm.Description != "TypeScript defaults" {
		t.Errorf("expected 'TypeScript defaults', got %q", fm.Description)
	}
	if len(fm.Globs) != 2 {
		t.Errorf("expected 2 globs, got %d", len(fm.Globs))
	}
	if fm.AlwaysApply == nil || *fm.AlwaysApply {
		t.Error("expected alwaysApply=false")
	}
	if body != "Prefer strict TypeScript." {
		t.Errorf("body mismatch: %q", body)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "# Just a regular markdown file"
	fm, body := ParseFrontmatter(content)
	if fm != nil {
		t.Error("expected nil frontmatter")
	}
	if body != content {
		t.Errorf("body should equal content")
	}
}

func TestParseFrontmatter_MultilineGlobs(t *testing.T) {
	content := `---
description: Go rules
globs:
  - "**/*.go"
  - "**/go.mod"
alwaysApply: true
---

Use gofmt.`

	fm, body := ParseFrontmatter(content)
	if fm == nil {
		t.Fatal("expected frontmatter")
	}
	if len(fm.Globs) != 2 {
		t.Errorf("expected 2 globs, got %d: %v", len(fm.Globs), fm.Globs)
	}
	if body != "Use gofmt." {
		t.Errorf("body mismatch: %q", body)
	}
}

func TestParseFrontmatter_AlwaysApplyNil(t *testing.T) {
	content := `---
description: Always active
---

Content here.`

	fm, _ := ParseFrontmatter(content)
	if fm == nil {
		t.Fatal("expected frontmatter")
	}
	if fm.AlwaysApply != nil {
		t.Error("expected alwaysApply to be nil when not specified")
	}
}

func TestShouldInject_AlwaysApplyTrue(t *testing.T) {
	b := true
	fm := &RuleFrontmatter{AlwaysApply: &b, Globs: []string{"*.ts"}}
	if !ShouldInject(fm, "main.go") {
		t.Error("alwaysApply=true should inject for any file")
	}
}

func TestShouldInject_GlobMatch(t *testing.T) {
	b := false
	fm := &RuleFrontmatter{AlwaysApply: &b, Globs: []string{"**/*.ts", "**/*.tsx"}}

	if !ShouldInject(fm, "src/app.ts") {
		t.Error("should match *.ts")
	}
	if !ShouldInject(fm, "src/app.tsx") {
		t.Error("should match *.tsx")
	}
	if ShouldInject(fm, "main.go") {
		t.Error("should not match *.go")
	}
}

func TestShouldInject_NoFrontmatter(t *testing.T) {
	if !ShouldInject(nil, "anything.go") {
		t.Error("nil frontmatter should always inject")
	}
}

func TestShouldInject_EmptyGlobs(t *testing.T) {
	b := false
	fm := &RuleFrontmatter{AlwaysApply: &b}
	if !ShouldInject(fm, "anything.go") {
		t.Error("empty globs should always inject")
	}
}

func TestMatchGlobs_SimplePatterns(t *testing.T) {
	tests := []struct {
		globs []string
		path  string
		want  bool
	}{
		{[]string{"*.go"}, "main.go", true},
		{[]string{"*.go"}, "main.ts", false},
		{[]string{"*.ts", "*.tsx"}, "app.tsx", true},
		{[]string{"*.json"}, "package.json", true},
	}
	for _, tt := range tests {
		if got := MatchGlobs(tt.globs, tt.path); got != tt.want {
			t.Errorf("MatchGlobs(%v, %q) = %v, want %v", tt.globs, tt.path, got, tt.want)
		}
	}
}

func TestMatchGlobs_DoubleStar(t *testing.T) {
	tests := []struct {
		glob string
		path string
		want bool
	}{
		{"**/*.ts", "src/app.ts", true},
		{"**/*.ts", "app.ts", true},
		{"**/*.ts", "src/deep/nested/app.ts", true},
		{"**/*.ts", "main.go", false},
		{"src/**/*.go", "src/pkg/main.go", true},
		{"src/**/*.go", "pkg/main.go", false},
	}
	for _, tt := range tests {
		if got := matchGlob(tt.glob, tt.path); got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.glob, tt.path, got, tt.want)
		}
	}
}

func TestMatchGlobs_PathPatterns(t *testing.T) {
	tests := []struct {
		glob string
		path string
		want bool
	}{
		{"**/test/*.go", "src/test/main_test.go", true},
		{"**/test/*.go", "main.go", false},
	}
	for _, tt := range tests {
		if got := matchGlob(tt.glob, tt.path); got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.glob, tt.path, got, tt.want)
		}
	}
}
