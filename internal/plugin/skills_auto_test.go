package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSmartSkills(t *testing.T) {
	dir := t.TempDir()

	// Create a skill with full frontmatter.
	skillDir := filepath.Join(dir, "api-review")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: api-review
description: Reviews API endpoints for consistency
paths: ["src/api/**", "routes/**"]
auto-invoke: true
---
Review all API endpoints and check for naming consistency.
`), 0o644)

	// Create a skill with no frontmatter.
	skill2Dir := filepath.Join(dir, "quick-fix")
	os.MkdirAll(skill2Dir, 0o755)
	os.WriteFile(filepath.Join(skill2Dir, "SKILL.md"), []byte("Fix common issues quickly.\n"), 0o644)

	skills := LoadSmartSkills([]string{dir})
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	// Find the api-review skill.
	var apiSkill *SmartSkill
	for i := range skills {
		if skills[i].Name == "api-review" {
			apiSkill = &skills[i]
		}
	}
	if apiSkill == nil {
		t.Fatal("expected api-review skill")
	}
	if apiSkill.Description != "Reviews API endpoints for consistency" {
		t.Errorf("unexpected description: %q", apiSkill.Description)
	}
	if !apiSkill.AutoInvoke {
		t.Error("expected auto-invoke to be true")
	}
	if len(apiSkill.Paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(apiSkill.Paths))
	}
	if !strings.Contains(apiSkill.Content, "Review all API endpoints") {
		t.Error("expected content from SKILL.md body")
	}
}

func TestMatchSkillsByPath(t *testing.T) {
	skills := []SmartSkill{
		{Name: "api-review", Paths: []string{"src/api/*.go"}},
		{Name: "test-helper", Paths: []string{"*_test.go"}},
		{Name: "docs", Paths: []string{"docs/*.md"}},
	}

	matched := MatchSkillsByPath(skills, "src/api/handler.go")
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if matched[0].Name != "api-review" {
		t.Errorf("expected api-review, got %s", matched[0].Name)
	}

	// Test base name matching.
	matched = MatchSkillsByPath(skills, "pkg/something_test.go")
	if len(matched) != 1 {
		t.Fatalf("expected 1 match for _test.go, got %d", len(matched))
	}
	if matched[0].Name != "test-helper" {
		t.Errorf("expected test-helper, got %s", matched[0].Name)
	}

	// No match.
	matched = MatchSkillsByPath(skills, "README.md")
	if len(matched) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matched))
	}
}

func TestMatchSkillsByContext(t *testing.T) {
	skills := []SmartSkill{
		{Name: "api-review", Description: "Reviews API endpoints for consistency"},
		{Name: "security", Description: "Checks security vulnerabilities in code"},
		{Name: "docs", Description: "Generates documentation"},
	}

	matched := MatchSkillsByContext(skills, "please review the API endpoints")
	found := false
	for _, s := range matched {
		if s.Name == "api-review" {
			found = true
		}
	}
	if !found {
		t.Error("expected api-review to match 'review the API endpoints'")
	}

	matched = MatchSkillsByContext(skills, "check for security vulnerabilities")
	found = false
	for _, s := range matched {
		if s.Name == "security" {
			found = true
		}
	}
	if !found {
		t.Error("expected security to match 'check for security vulnerabilities'")
	}
}

func TestFormatSkillsForPrompt(t *testing.T) {
	skills := []SmartSkill{
		{Name: "api-review", Description: "Reviews API endpoints", Content: "Check naming patterns."},
		{Name: "security", Description: "Security review", Content: "Look for injection risks."},
	}

	output := FormatSkillsForPrompt(skills)
	if !strings.Contains(output, "## Available Skills") {
		t.Error("expected header in output")
	}
	if !strings.Contains(output, "### api-review") {
		t.Error("expected api-review skill")
	}
	if !strings.Contains(output, "Check naming patterns") {
		t.Error("expected content for api-review")
	}
	if !strings.Contains(output, "### security") {
		t.Error("expected security skill")
	}

	// Empty skills.
	empty := FormatSkillsForPrompt(nil)
	if empty != "" {
		t.Errorf("expected empty output for nil skills, got %q", empty)
	}
}
