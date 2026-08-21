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

func TestParseSmartSkill_AcceptsCommunitySnakeCaseMetadata(t *testing.T) {
	t.Parallel()
	skill := parseSmartSkill(`---
name: community-skill
description: Uses community metadata names
auto_invoke: true
allowed_tools: Read Write
chain_after: [security-review]
chain_before: [test-review]
chain_conflicts: [unsafe-mode]
chain_enhances: [go-review]
source_repo: example/repo
source_ref: main
source_installed_at: 2026-08-21T00:00:00Z
---
body
`)
	if !skill.AutoInvoke {
		t.Fatal("expected auto_invoke to enable auto invocation")
	}
	if skill.AllowedTools != "Read Write" {
		t.Errorf("allowed tools = %q", skill.AllowedTools)
	}
	if len(skill.Chain.After) != 1 || skill.Chain.After[0] != "security-review" {
		t.Errorf("chain after = %#v", skill.Chain.After)
	}
	if len(skill.Chain.Before) != 1 || skill.Chain.Before[0] != "test-review" {
		t.Errorf("chain before = %#v", skill.Chain.Before)
	}
	if len(skill.Chain.Conflicts) != 1 || skill.Chain.Conflicts[0] != "unsafe-mode" {
		t.Errorf("chain conflicts = %#v", skill.Chain.Conflicts)
	}
	if len(skill.Chain.Enhances) != 1 || skill.Chain.Enhances[0] != "go-review" {
		t.Errorf("chain enhances = %#v", skill.Chain.Enhances)
	}
	if skill.Source.Repo != "example/repo" || skill.Source.Ref != "main" {
		t.Errorf("source = %#v", skill.Source)
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
