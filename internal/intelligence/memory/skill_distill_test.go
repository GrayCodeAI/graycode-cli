package memory

import (
	"strings"
	"testing"
)

func TestBuildSkillPrompt(t *testing.T) {
	sd := &SkillDistiller{}
	prompt := sd.BuildSkillPrompt(
		"fix failing tests",
		[]string{"Read", "Edit", "Bash"},
		[]string{"main.go", "main_test.go"},
		"all tests pass",
	)
	if !strings.Contains(prompt, "fix failing tests") {
		t.Fatal("prompt missing task description")
	}
	if !strings.Contains(prompt, "Read, Edit, Bash") {
		t.Fatal("prompt missing tools")
	}
	if !strings.Contains(prompt, "main.go, main_test.go") {
		t.Fatal("prompt missing files")
	}
	if !strings.Contains(prompt, "all tests pass") {
		t.Fatal("prompt missing outcome")
	}
}

func TestParseSkill_Valid(t *testing.T) {
	sd := &SkillDistiller{}
	resp := `Here is the skill:
{
  "name": "fix-tests",
  "description": "Fix failing Go tests",
  "steps": ["read test output", "identify failure", "edit source"],
  "tools": ["Bash", "Read", "Edit"],
  "patterns": ["run tests first to see failures"]
}`
	skill, err := sd.ParseSkill(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.Name != "fix-tests" {
		t.Fatalf("expected name 'fix-tests', got %q", skill.Name)
	}
	if len(skill.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(skill.Steps))
	}
	if len(skill.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(skill.Tools))
	}
	if len(skill.Patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(skill.Patterns))
	}
}

func TestParseSkill_NoJSON(t *testing.T) {
	sd := &SkillDistiller{}
	_, err := sd.ParseSkill("no json here")
	if err == nil {
		t.Fatal("expected error for missing JSON")
	}
}

func TestParseSkill_EmptyName(t *testing.T) {
	sd := &SkillDistiller{}
	_, err := sd.ParseSkill(`{"name":"","description":"test"}`)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}
