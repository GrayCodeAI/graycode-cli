package scaffold

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewSkillRegistry(t *testing.T) {
	reg := NewSkillRegistry("/tmp/skills")
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
	if reg.Dir != "/tmp/skills" {
		t.Errorf("expected dir /tmp/skills, got %s", reg.Dir)
	}
	if reg.Skills == nil {
		t.Error("expected non-nil Skills map")
	}
}

func TestSkillRegistryRegister(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())

	// Register a valid skill.
	skill := &Skill{
		ID:   "test-1",
		Name: "Test Skill",
		Steps: []SkillStep{
			{Order: 1, Action: "prompt", Content: "Do something"},
		},
		Tags:      []string{"go", "testing"},
		Author:    "user",
		CreatedAt: time.Now(),
	}

	err := reg.Register(skill)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := reg.Get("test-1")
	if got == nil {
		t.Fatal("expected to find registered skill")
	}
	if got.Name != "Test Skill" {
		t.Errorf("expected name 'Test Skill', got %q", got.Name)
	}
}

func TestSkillRegistryRegisterValidation(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())

	// Nil skill.
	if err := reg.Register(nil); err == nil {
		t.Error("expected error for nil skill")
	}

	// Empty ID.
	if err := reg.Register(&Skill{Name: "foo"}); err == nil {
		t.Error("expected error for empty ID")
	}

	// Empty Name.
	if err := reg.Register(&Skill{ID: "foo"}); err == nil {
		t.Error("expected error for empty Name")
	}
}

func TestSkillRegistryGet(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())
	_ = reg.Register(&Skill{ID: "s1", Name: "Skill One"})

	if got := reg.Get("s1"); got == nil {
		t.Error("expected to find skill s1")
	}
	if got := reg.Get("nonexistent"); got != nil {
		t.Error("expected nil for nonexistent skill")
	}
}

func TestSkillRegistrySearch(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())
	_ = reg.Register(&Skill{
		ID:          "go-test",
		Name:        "Add Go test file",
		Description: "Creates a Go test file with table-driven tests",
		Tags:        []string{"go", "testing"},
		SuccessRate: 0.95,
		UsageCount:  20,
	})
	_ = reg.Register(&Skill{
		ID:          "py-lint",
		Name:        "Run Python linter",
		Description: "Runs pylint on the project",
		Tags:        []string{"python", "linting"},
		SuccessRate: 0.80,
		UsageCount:  10,
	})
	_ = reg.Register(&Skill{
		ID:          "go-bench",
		Name:        "Go benchmark",
		Description: "Runs benchmarks for Go code",
		Tags:        []string{"go", "performance"},
		SuccessRate: 0.70,
		UsageCount:  5,
	})

	// Search by keyword.
	results := reg.Search("test", nil)
	if len(results) == 0 {
		t.Fatal("expected results for 'test'")
	}
	if results[0].ID != "go-test" {
		t.Errorf("expected go-test first, got %s", results[0].ID)
	}

	// Search by tag.
	results = reg.Search("", []string{"go"})
	if len(results) != 2 {
		t.Errorf("expected 2 results for tag 'go', got %d", len(results))
	}

	// Search by keyword + tag.
	results = reg.Search("benchmark", []string{"go"})
	if len(results) == 0 {
		t.Fatal("expected results for benchmark + go tag")
	}
	if results[0].ID != "go-bench" {
		t.Errorf("expected go-bench first, got %s", results[0].ID)
	}

	// No matches.
	results = reg.Search("nonexistent", nil)
	if len(results) != 0 {
		t.Errorf("expected no results, got %d", len(results))
	}
}

func TestExecute(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())
	_ = reg.Register(&Skill{
		ID:   "exec-test",
		Name: "Exec Test",
		Steps: []SkillStep{
			{Order: 1, Action: "prompt", Content: "Read {{file}}"},
			{Order: 2, Action: "tool_call", Content: "Write test for {{file}}"},
			{Order: 3, Action: "check", Content: "Run tests"},
		},
		SuccessRate: 1.0,
		UsageCount:  5,
	})

	callLog := []string{}
	execFn := func(content string) (string, error) {
		callLog = append(callLog, content)
		return "ok: " + content, nil
	}

	vars := map[string]string{"file": "main.go"}
	result, err := reg.Execute(context.Background(), "exec-test", vars, execFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if result.StepsCompleted != 3 {
		t.Errorf("expected 3 steps completed, got %d", result.StepsCompleted)
	}
	if result.TotalSteps != 3 {
		t.Errorf("expected 3 total steps, got %d", result.TotalSteps)
	}
	if len(result.Outputs) != 3 {
		t.Errorf("expected 3 outputs, got %d", len(result.Outputs))
	}

	// Check variable substitution.
	if !strings.Contains(callLog[0], "main.go") {
		t.Errorf("expected variable substitution in first call, got %q", callLog[0])
	}
}

func TestExecuteWithFallback(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())
	_ = reg.Register(&Skill{
		ID:   "fallback-test",
		Name: "Fallback Test",
		Steps: []SkillStep{
			{Order: 1, Action: "tool_call", Content: "primary action", Fallback: "fallback action"},
		},
		SuccessRate: 0.5,
		UsageCount:  2,
	})

	callCount := 0
	execFn := func(content string) (string, error) {
		callCount++
		if content == "primary action" {
			return "", fmt.Errorf("primary failed")
		}
		return "fallback succeeded", nil
	}

	result, err := reg.Execute(context.Background(), "fallback-test", nil, execFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success via fallback")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (primary + fallback), got %d", callCount)
	}
}

func TestExecuteContextCancellation(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())
	_ = reg.Register(&Skill{
		ID:   "cancel-test",
		Name: "Cancel Test",
		Steps: []SkillStep{
			{Order: 1, Action: "prompt", Content: "step 1"},
			{Order: 2, Action: "prompt", Content: "step 2"},
		},
		SuccessRate: 1.0,
		UsageCount:  1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := reg.Execute(ctx, "cancel-test", nil, func(s string) (string, error) {
		return s, nil
	})
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestExecuteNotFound(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())
	_, err := reg.Execute(context.Background(), "nope", nil, nil)
	if err == nil {
		t.Error("expected error for missing skill")
	}
}

func TestLearnFromSession(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())

	toolCalls := []string{
		"read /home/user/project/main.go",
		"check:go test ./...",
		"prompt:explain the error",
		"write /home/user/project/main_test.go",
	}

	skill := reg.LearnFromSession("Add Go test file", toolCalls, "tests pass")
	if skill == nil {
		t.Fatal("expected non-nil skill")
	}
	if skill.Name != "Add Go test file" {
		t.Errorf("unexpected name: %s", skill.Name)
	}
	if skill.Author != "agent" {
		t.Errorf("expected author 'agent', got %s", skill.Author)
	}
	if len(skill.Steps) != 4 {
		t.Errorf("expected 4 steps, got %d", len(skill.Steps))
	}

	// Check action types.
	if skill.Steps[0].Action != "tool_call" {
		t.Errorf("expected tool_call for step 1, got %s", skill.Steps[0].Action)
	}
	if skill.Steps[1].Action != "check" {
		t.Errorf("expected check for step 2, got %s", skill.Steps[1].Action)
	}
	if skill.Steps[2].Action != "prompt" {
		t.Errorf("expected prompt for step 3, got %s", skill.Steps[2].Action)
	}

	// Check tags were extracted.
	hasGoTag := false
	hasTestingTag := false
	for _, tag := range skill.Tags {
		if tag == "go" {
			hasGoTag = true
		}
		if tag == "testing" {
			hasTestingTag = true
		}
	}
	if !hasGoTag {
		t.Error("expected 'go' tag")
	}
	if !hasTestingTag {
		t.Error("expected 'testing' tag")
	}

	// Check path generalization.
	if strings.Contains(skill.Steps[0].Content, "/home/user/project/") {
		t.Error("expected paths to be generalized")
	}
}

func TestFormatSkill(t *testing.T) {
	skill := &Skill{
		Name: "Add Go test file",
		Tags: []string{"go", "testing"},
		Steps: []SkillStep{
			{Order: 1, Content: "Read source file to understand API"},
			{Order: 2, Content: "Create _test.go with table-driven tests"},
			{Order: 3, Content: "Run go test to verify"},
			{Order: 4, Content: "Fix any failures"},
		},
		SuccessRate: 0.95,
		UsageCount:  20,
	}

	formatted := FormatSkill(skill)
	if !strings.Contains(formatted, `"Add Go test file"`) {
		t.Error("expected skill name in output")
	}
	if !strings.Contains(formatted, "[go testing]") {
		t.Error("expected tags in output")
	}
	if !strings.Contains(formatted, "95%") {
		t.Error("expected success rate in output")
	}
	if !strings.Contains(formatted, "19/20") {
		t.Error("expected usage count fraction in output")
	}
	if !strings.Contains(formatted, "1. Read source file") {
		t.Error("expected step 1 in output")
	}

	// Nil skill.
	if FormatSkill(nil) != "" {
		t.Error("expected empty string for nil skill")
	}
}

func TestSkillRegistryListByTag(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())
	_ = reg.Register(&Skill{ID: "s1", Name: "S1", Tags: []string{"go", "testing"}})
	_ = reg.Register(&Skill{ID: "s2", Name: "S2", Tags: []string{"python"}})
	_ = reg.Register(&Skill{ID: "s3", Name: "S3", Tags: []string{"go", "performance"}})

	goSkills := reg.ListByTag("go")
	if len(goSkills) != 2 {
		t.Errorf("expected 2 go skills, got %d", len(goSkills))
	}

	pySkills := reg.ListByTag("python")
	if len(pySkills) != 1 {
		t.Errorf("expected 1 python skill, got %d", len(pySkills))
	}

	// Case-insensitive.
	goSkills2 := reg.ListByTag("Go")
	if len(goSkills2) != 2 {
		t.Errorf("expected case-insensitive match, got %d", len(goSkills2))
	}

	none := reg.ListByTag("rust")
	if len(none) != 0 {
		t.Errorf("expected 0 rust skills, got %d", len(none))
	}
}

func TestSkillRegistrySaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	reg := NewSkillRegistry(dir)
	_ = reg.Register(&Skill{
		ID:          "save-test",
		Name:        "Save Test",
		Description: "Tests persistence",
		Tags:        []string{"testing"},
		Steps: []SkillStep{
			{Order: 1, Action: "prompt", Content: "Hello"},
		},
		SuccessRate: 0.9,
		UsageCount:  10,
		CreatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Author:      "user",
	})

	if err := reg.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists.
	path := filepath.Join(dir, "skills.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("skills.json not created: %v", err)
	}

	// Load into new registry.
	reg2 := NewSkillRegistry(dir)
	if err := reg2.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	skill := reg2.Get("save-test")
	if skill == nil {
		t.Fatal("expected to find skill after load")
	}
	if skill.Name != "Save Test" {
		t.Errorf("expected 'Save Test', got %q", skill.Name)
	}
	if skill.SuccessRate != 0.9 {
		t.Errorf("expected 0.9 success rate, got %f", skill.SuccessRate)
	}
	if len(skill.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(skill.Steps))
	}
}

func TestSkillRegistryLoadNonexistent(t *testing.T) {
	reg := NewSkillRegistry(filepath.Join(t.TempDir(), "nonexistent"))
	// Should not error on missing file.
	if err := reg.Load(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reg.Skills) != 0 {
		t.Error("expected empty registry")
	}
}

func TestSkillRegistryRemove(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())
	_ = reg.Register(&Skill{ID: "rm1", Name: "Remove Me"})

	if err := reg.Remove("rm1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.Get("rm1") != nil {
		t.Error("expected skill to be removed")
	}

	// Remove nonexistent.
	if err := reg.Remove("nope"); err == nil {
		t.Error("expected error removing nonexistent skill")
	}
}

func TestUpdateStats(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())
	_ = reg.Register(&Skill{
		ID:          "stats-test",
		Name:        "Stats Test",
		SuccessRate: 1.0,
		UsageCount:  4,
	})

	// Success.
	reg.UpdateStats("stats-test", true)
	skill := reg.Get("stats-test")
	if skill.UsageCount != 5 {
		t.Errorf("expected usage count 5, got %d", skill.UsageCount)
	}
	if skill.SuccessRate != 1.0 {
		t.Errorf("expected success rate 1.0, got %f", skill.SuccessRate)
	}

	// Failure.
	reg.UpdateStats("stats-test", false)
	skill = reg.Get("stats-test")
	if skill.UsageCount != 6 {
		t.Errorf("expected usage count 6, got %d", skill.UsageCount)
	}
	expectedRate := 5.0 / 6.0
	if skill.SuccessRate < expectedRate-0.01 || skill.SuccessRate > expectedRate+0.01 {
		t.Errorf("expected success rate ~%f, got %f", expectedRate, skill.SuccessRate)
	}

	// Update nonexistent (should not panic).
	reg.UpdateStats("nonexistent", true)
}

func TestSkillRegistryConcurrentAccess(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("skill-%d", n)
			_ = reg.Register(&Skill{ID: id, Name: "Skill " + id})
			reg.Get(id)
			reg.Search("skill", nil)
			reg.ListByTag("go")
			reg.UpdateStats(id, true)
		}(i)
	}
	wg.Wait()

	if len(reg.Skills) != 50 {
		t.Errorf("expected 50 skills, got %d", len(reg.Skills))
	}
}

func TestSkillRegistrySubstituteVars(t *testing.T) {
	tests := []struct {
		content  string
		vars     map[string]string
		expected string
	}{
		{"Hello {{name}}", map[string]string{"name": "world"}, "Hello world"},
		{"{{a}} and {{b}}", map[string]string{"a": "X", "b": "Y"}, "X and Y"},
		{"no vars", nil, "no vars"},
		{"{{missing}}", map[string]string{}, "{{missing}}"},
	}

	for _, tt := range tests {
		got := substituteVars(tt.content, tt.vars)
		if got != tt.expected {
			t.Errorf("substituteVars(%q, %v) = %q, want %q", tt.content, tt.vars, got, tt.expected)
		}
	}
}

func TestExecuteStepFailure(t *testing.T) {
	reg := NewSkillRegistry(t.TempDir())
	_ = reg.Register(&Skill{
		ID:   "fail-test",
		Name: "Fail Test",
		Steps: []SkillStep{
			{Order: 1, Action: "prompt", Content: "step 1"},
			{Order: 2, Action: "tool_call", Content: "step 2"},
			{Order: 3, Action: "check", Content: "step 3"},
		},
		SuccessRate: 1.0,
		UsageCount:  1,
	})

	execFn := func(content string) (string, error) {
		if content == "step 2" {
			return "", fmt.Errorf("step 2 failed")
		}
		return "ok", nil
	}

	result, err := reg.Execute(context.Background(), "fail-test", nil, execFn)
	if err == nil {
		t.Fatal("expected error")
	}
	if result.Success {
		t.Error("expected failure")
	}
	if result.StepsCompleted != 1 {
		t.Errorf("expected 1 step completed, got %d", result.StepsCompleted)
	}
}
