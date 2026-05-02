package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRulesFrom(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".hawk", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Rule with frontmatter
	rule1 := "---\npaths: [\"src/api/**\", \"internal/api/**\"]\n---\nAlways validate input parameters.\n"
	if err := os.WriteFile(filepath.Join(rulesDir, "api-validation.md"), []byte(rule1), 0o644); err != nil {
		t.Fatal(err)
	}

	// Rule without frontmatter (always active)
	rule2 := "Use descriptive variable names.\n"
	if err := os.WriteFile(filepath.Join(rulesDir, "naming.md"), []byte(rule2), 0o644); err != nil {
		t.Fatal(err)
	}

	rules := LoadRulesFrom(dir)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}

	// Find the api-validation rule
	var apiRule, namingRule *Rule
	for i := range rules {
		if rules[i].Name == "api-validation" {
			apiRule = &rules[i]
		}
		if rules[i].Name == "naming" {
			namingRule = &rules[i]
		}
	}

	if apiRule == nil {
		t.Fatal("api-validation rule not found")
	}
	if len(apiRule.Paths) != 2 {
		t.Fatalf("expected 2 paths for api-validation, got %d", len(apiRule.Paths))
	}
	if !strings.Contains(apiRule.Content, "validate input") {
		t.Fatalf("unexpected content: %q", apiRule.Content)
	}

	if namingRule == nil {
		t.Fatal("naming rule not found")
	}
	if len(namingRule.Paths) != 0 {
		t.Fatalf("expected 0 paths for naming rule, got %d", len(namingRule.Paths))
	}
}

func TestActiveRulesAlwaysActive(t *testing.T) {
	rules := []Rule{
		{Name: "global", Content: "global rule", Paths: nil},
		{Name: "api-only", Content: "api rule", Paths: []string{"src/api/**"}},
	}

	active := ActiveRules(rules, []string{"src/web/handler.go"})
	if len(active) != 1 {
		t.Fatalf("expected 1 active rule (global only), got %d", len(active))
	}
	if active[0].Name != "global" {
		t.Fatalf("expected 'global' rule, got %q", active[0].Name)
	}
}

func TestActiveRulesPathMatch(t *testing.T) {
	rules := []Rule{
		{Name: "global", Content: "global rule", Paths: nil},
		{Name: "api-only", Content: "api rule", Paths: []string{"src/api/**"}},
	}

	active := ActiveRules(rules, []string{"src/api/handler.go"})
	if len(active) != 2 {
		t.Fatalf("expected 2 active rules, got %d", len(active))
	}
}

func TestActiveRulesNoMatch(t *testing.T) {
	rules := []Rule{
		{Name: "api-only", Content: "api rule", Paths: []string{"src/api/**"}},
	}

	active := ActiveRules(rules, []string{"cmd/main.go"})
	if len(active) != 0 {
		t.Fatalf("expected 0 active rules, got %d", len(active))
	}
}

func TestFormatActiveRules(t *testing.T) {
	rules := []Rule{
		{Name: "naming", Content: "Use descriptive names."},
		{Name: "testing", Content: "Write tests for all functions."},
	}

	result := FormatActiveRules(rules)
	if !strings.HasPrefix(result, "## Project Rules") {
		t.Fatalf("expected header, got %q", result)
	}
	if !strings.Contains(result, "### naming") {
		t.Fatal("expected rule name in output")
	}
	if !strings.Contains(result, "descriptive names") {
		t.Fatal("expected rule content in output")
	}

	// Empty rules
	if got := FormatActiveRules(nil); got != "" {
		t.Fatalf("expected empty for nil rules, got %q", got)
	}
}

func TestParseFrontmatterPathsListFormat(t *testing.T) {
	fm := "paths:\n- src/api/**\n- internal/api/**"
	paths := parseFrontmatterPaths(fm)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d: %v", len(paths), paths)
	}
	if paths[0] != "src/api/**" {
		t.Fatalf("unexpected path[0]: %q", paths[0])
	}
}
