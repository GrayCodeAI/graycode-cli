package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSkillsFromDir(t *testing.T) {
	dir := t.TempDir()

	// Create two skill files with valid front matter.
	skill1 := `---
name: deploy
description: Deploy the application to production
---

Run the deployment pipeline with health checks.
`
	skill2 := `---
name: test-suite
description: Run the full test suite
---

Execute unit and integration tests with coverage.
`
	if err := os.WriteFile(filepath.Join(dir, "deploy.md"), []byte(skill1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test-suite.md"), []byte(skill2), 0644); err != nil {
		t.Fatal(err)
	}
	// Non-md file should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0644); err != nil {
		t.Fatal(err)
	}

	skills, err := LoadSkillsFromDir(dir)
	if err != nil {
		t.Fatalf("LoadSkillsFromDir returned error: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	// Verify first skill (order depends on ReadDir, which is sorted).
	found := make(map[string]Skill)
	for _, s := range skills {
		found[s.Name] = s
	}

	deploy, ok := found["deploy"]
	if !ok {
		t.Fatal("missing skill 'deploy'")
	}
	if deploy.Description != "Deploy the application to production" {
		t.Errorf("deploy.Description = %q", deploy.Description)
	}
	if deploy.Content != "Run the deployment pipeline with health checks." {
		t.Errorf("deploy.Content = %q", deploy.Content)
	}

	ts, ok := found["test-suite"]
	if !ok {
		t.Fatal("missing skill 'test-suite'")
	}
	if ts.Description != "Run the full test suite" {
		t.Errorf("test-suite.Description = %q", ts.Description)
	}
}

func TestLoadSkillsEmptyDir(t *testing.T) {
	dir := t.TempDir()

	skills, err := LoadSkillsFromDir(dir)
	if err != nil {
		t.Fatalf("LoadSkillsFromDir returned error: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(skills))
	}
}
