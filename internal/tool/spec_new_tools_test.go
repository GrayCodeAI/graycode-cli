package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClarifyTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := ClarifyTool{}
	if tool.Name() != "Clarify" {
		t.Errorf("expected name Clarify, got %s", tool.Name())
	}
	aliases := tool.Aliases()
	if len(aliases) == 0 {
		t.Error("expected at least one alias")
	}
}

func TestClarifyTool_RequiresActiveSpec(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())
	input, _ := json.Marshal(map[string]string{"artifact": "spec.md"})
	if _, err := (ClarifyTool{}).Execute(ctx, input); err == nil {
		t.Fatal("expected error when no spec is active")
	}
}

func TestClarifyTool_AnalyzesSpec(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	// Create a spec
	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "Some spec content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	// Write a vague spec
	dir, _ := specDir(ctx)
	os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec\n\nMaybe we should do something fast and small.\n\n## Requirements\n\n- Something TBD"), 0o600)

	input, _ := json.Marshal(map[string]string{"artifact": "spec.md"})
	result, err := (ClarifyTool{}).Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "clarification") && !strings.Contains(result, "area") {
		t.Errorf("expected clarification questions, got: %s", result)
	}
}

func TestClarifyTool_EmptySpec(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "Some content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	dir, _ := specDir(ctx)
	os.WriteFile(filepath.Join(dir, "spec.md"), []byte(""), 0o600)

	input, _ := json.Marshal(map[string]string{"artifact": "spec.md"})
	if _, err := (ClarifyTool{}).Execute(ctx, input); err == nil {
		t.Fatal("expected error for empty spec")
	}
}

func TestAnalyzeTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := AnalyzeTool{}
	if tool.Name() != "Analyze" {
		t.Errorf("expected name Analyze, got %s", tool.Name())
	}
}

func TestAnalyzeTool_NoArtifacts(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	// Create a spec but don't write artifacts
	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	result, err := (AnalyzeTool{}).Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Spec dir exists but spec.md/plan.md/tasks.md don't → quality score with missing artifact warnings
	if !strings.Contains(result, "Quality score") {
		t.Errorf("expected quality score, got: %s", result)
	}
	if !strings.Contains(result, "Missing Artifact") {
		t.Errorf("expected 'Missing Artifact' warning, got: %s", result)
	}
}

func TestAnalyzeTool_WithArtifacts(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	dir, _ := specDir(ctx)
	os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec\n\n### Requirement: Feature X\nSHALL do something\n\n#### Scenario: Basic\n- WHEN user clicks\n- THEN it works"), 0o600)
	os.WriteFile(filepath.Join(dir, "plan.md"), []byte("# Plan\n\n## Decision: Use Go\n\n## Risks: Low"), 0o600)
	os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("# Tasks\n\n## 1. Setup\n\n- [ ] 1.1 Create module\n- [ ] 1.2 Add deps\n\n## 2. Implement\n\n- [ ] 2.1 Code it"), 0o600)

	result, err := (AnalyzeTool{}).Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Quality score") {
		t.Errorf("expected quality score, got: %s", result)
	}
}

func TestChecklistTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := ChecklistTool{}
	if tool.Name() != "Checklist" {
		t.Errorf("expected name Checklist, got %s", tool.Name())
	}
}

func TestChecklistTool_GeneratesChecklist(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	dir, _ := specDir(ctx)
	os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec\n\n### Requirement: Feature X\nSHALL do something\n\n#### Scenario: Basic\n- WHEN user clicks\n- THEN it works"), 0o600)

	input, _ := json.Marshal(map[string]bool{"include_references": true})
	result, err := (ChecklistTool{}).Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Checklist") {
		t.Errorf("expected Checklist in result, got: %s", result)
	}

	// Verify checklist file was created
	checklistPath := filepath.Join(dir, "checklist.md")
	if _, err := os.Stat(checklistPath); os.IsNotExist(err) {
		t.Error("expected checklist.md to be created")
	}
}

func TestChecklistTool_NoReferences(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	dir, _ := specDir(ctx)
	os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec\n\n### Requirement: Feature X\nSHALL do something"), 0o600)

	input, _ := json.Marshal(map[string]bool{"include_references": false})
	result, err := (ChecklistTool{}).Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result, "Accessibility Checklist") {
		t.Error("should not include reference checklists when include_references=false")
	}
}

func TestConstitutionTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := ConstitutionTool{}
	if tool.Name() != "Constitution" {
		t.Errorf("expected name Constitution, got %s", tool.Name())
	}
}

func TestConstitutionTool_InitAndGet(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	// Init
	initInput, _ := json.Marshal(map[string]string{"action": "init"})
	result, err := (ConstitutionTool{}).Execute(ctx, initInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Created constitution") {
		t.Errorf("expected 'Created constitution', got: %s", result)
	}

	// Get
	getInput, _ := json.Marshal(map[string]string{"action": "get"})
	result, err = (ConstitutionTool{}).Execute(ctx, getInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Project Constitution") {
		t.Errorf("expected 'Project Constitution', got: %s", result)
	}
}

func TestConstitutionTool_InitTwiceFails(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	initInput, _ := json.Marshal(map[string]string{"action": "init"})
	if _, err := (ConstitutionTool{}).Execute(ctx, initInput); err != nil {
		t.Fatal(err)
	}

	// Second init should fail
	if _, err := (ConstitutionTool{}).Execute(ctx, initInput); err == nil {
		t.Fatal("expected error on second init")
	}
}

func TestConstitutionTool_SetAndGet(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	// Set
	setInput, _ := json.Marshal(map[string]string{"action": "set", "rules": "## Custom Rules\n\n1. Rule one\n2. Rule two"})
	result, err := (ConstitutionTool{}).Execute(ctx, setInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Updated constitution") {
		t.Errorf("expected 'Updated constitution', got: %s", result)
	}

	// Get
	getInput, _ := json.Marshal(map[string]string{"action": "get"})
	result, err = (ConstitutionTool{}).Execute(ctx, getInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Custom Rules") {
		t.Errorf("expected 'Custom Rules', got: %s", result)
	}
}

func TestConstitutionTool_SetRequiresRules(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	setInput, _ := json.Marshal(map[string]string{"action": "set"})
	if _, err := (ConstitutionTool{}).Execute(ctx, setInput); err == nil {
		t.Fatal("expected error when rules is empty")
	}
}

func TestConstitutionTool_Validate(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	// Create constitution
	initInput, _ := json.Marshal(map[string]string{"action": "init"})
	if _, err := (ConstitutionTool{}).Execute(ctx, initInput); err != nil {
		t.Fatal(err)
	}

	// Write a spec with SHALL
	dir, _ := specDir(ctx)
	os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec\n\n### Requirement: Feature X\nSHALL do something\n\n#### Scenario: Basic\n- WHEN user clicks\n- THEN it works"), 0o600)

	// Validate
	validateInput, _ := json.Marshal(map[string]string{"action": "validate"})
	result, err := (ConstitutionTool{}).Execute(ctx, validateInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "passes constitution validation") {
		t.Errorf("expected 'passes constitution validation', got: %s", result)
	}
}

func TestConvergeTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := ConvergeTool{}
	if tool.Name() != "Converge" {
		t.Errorf("expected name Converge, got %s", tool.Name())
	}
}

func TestConvergeTool_NoActiveSpec(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())
	if _, err := (ConvergeTool{}).Execute(ctx, nil); err == nil {
		t.Fatal("expected error when no spec is active")
	}
}

func TestConvergeTool_AssessesGaps(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	// Write incomplete spec
	dir, _ := specDir(ctx)
	os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec\n\n### Requirement: Feature X\nSHALL do something"), 0o600)
	os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("# Tasks\n\n- [ ] 1.1 Do something\n- [ ] 1.2 Do something else"), 0o600)

	result, err := (ConvergeTool{}).Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "gap") {
		t.Errorf("expected 'gap' in result, got: %s", result)
	}
}

func TestConvergeTool_AppendTasks(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	dir, _ := specDir(ctx)
	os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec\n\n### Requirement: Feature X\nSHALL do something"), 0o600)
	os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("# Tasks\n\n- [ ] 1.1 Do something"), 0o600)

	input, _ := json.Marshal(map[string]bool{"append_tasks": true})
	result, err := (ConvergeTool{}).Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Convergence tasks appended") {
		t.Errorf("expected 'Convergence tasks appended', got: %s", result)
	}
}

func TestTasksToIssuesTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := TasksToIssuesTool{}
	if tool.Name() != "TasksToIssues" {
		t.Errorf("expected name TasksToIssues, got %s", tool.Name())
	}
}

func TestTasksToIssuesTool_NoGHCli(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	dir, _ := specDir(ctx)
	os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("# Tasks\n\n- [ ] 1.1 Do something"), 0o600)

	// This will fail because gh CLI is not authenticated or not present
	input, _ := json.Marshal(map[string]bool{"dry_run": true})
	_, err := (TasksToIssuesTool{}).Execute(ctx, input)
	// Error is expected since we're not in a real git repo with gh auth
	if err == nil {
		// If it succeeds, that's fine too (gh is available and authenticated)
		return
	}
	if !strings.Contains(err.Error(), "gh") {
		t.Errorf("expected error about gh CLI, got: %v", err)
	}
}

func TestSpecStatusTool_DodCheck(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	// Write all artifacts
	dir, _ := specDir(ctx)
	os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec\n\n### Requirement: Feature X\nSHALL do something\n\n#### Scenario: Basic\n- WHEN user clicks\n- THEN it works"), 0o600)
	os.WriteFile(filepath.Join(dir, "plan.md"), []byte("# Plan\n\n## Decision: Use Go"), 0o600)
	os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("# Tasks\n\n- [x] 1.1 Done task"), 0o600)
	os.WriteFile(filepath.Join(dir, "constitution.md"), []byte("## Rules\n\n1. Rule one"), 0o600)

	result, err := (SpecStatusTool{}).Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Definition of Done") {
		t.Errorf("expected 'Definition of Done' in result, got: %s", result)
	}

	// Check that completed tasks are marked
	if !strings.Contains(result, "All tasks marked complete") {
		t.Errorf("expected 'All tasks marked complete', got: %s", result)
	}
}

func TestSpecStatusTool_DodIncomplete(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "test", "spec": "content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	dir, _ := specDir(ctx)
	os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec content"), 0o600)
	os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("# Tasks\n\n- [ ] 1.1 Incomplete task"), 0o600)

	result, err := (SpecStatusTool{}).Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result, "✓ All tasks marked complete") {
		t.Error("should show ✗ (not ✓) for 'All tasks marked complete' when tasks are incomplete")
	}
}
