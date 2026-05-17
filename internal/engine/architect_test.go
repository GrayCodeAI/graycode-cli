package engine

import (
	"context"
	"strings"
	"testing"
)

func TestParsePlan_WellFormatted(t *testing.T) {
	response := `GOAL: Add caching layer to the API
COMPLEXITY: moderate
FILES: cache.go, handler.go, cache_test.go

STEPS:
1. [cache.go] CREATE: Implement an in-memory LRU cache with TTL support
2. [handler.go] MODIFY: Wrap database calls with cache lookups
3. [cache_test.go] CREATE: Add unit tests for cache hit/miss/expiry scenarios`

	plan, err := ParsePlan(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Goal != "Add caching layer to the API" {
		t.Errorf("goal = %q, want %q", plan.Goal, "Add caching layer to the API")
	}
	if plan.EstimatedComplexity != "moderate" {
		t.Errorf("complexity = %q, want %q", plan.EstimatedComplexity, "moderate")
	}
	if len(plan.FilesToModify) != 3 {
		t.Fatalf("files count = %d, want 3", len(plan.FilesToModify))
	}
	if plan.FilesToModify[0] != "cache.go" {
		t.Errorf("files[0] = %q, want %q", plan.FilesToModify[0], "cache.go")
	}
	if plan.FilesToModify[1] != "handler.go" {
		t.Errorf("files[1] = %q, want %q", plan.FilesToModify[1], "handler.go")
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("steps count = %d, want 3", len(plan.Steps))
	}
	if plan.Steps[0].Action != "create" {
		t.Errorf("step[0].Action = %q, want %q", plan.Steps[0].Action, "create")
	}
	if plan.Steps[0].File != "cache.go" {
		t.Errorf("step[0].File = %q, want %q", plan.Steps[0].File, "cache.go")
	}
	if plan.Steps[1].Action != "modify" {
		t.Errorf("step[1].Action = %q, want %q", plan.Steps[1].Action, "modify")
	}
	if plan.Steps[2].File != "cache_test.go" {
		t.Errorf("step[2].File = %q, want %q", plan.Steps[2].File, "cache_test.go")
	}
}

func TestParsePlan_MessyFormat(t *testing.T) {
	// Varying format: inconsistent spacing, no brackets on files,
	// different action format, extra whitespace
	response := `
  Goal:   Refactor the authentication module

  Complexity:   Simple

  Files:   auth.go,  middleware.go

STEPS:
  1. MODIFY auth.go: Extract token validation into a separate function
  2. MODIFY middleware.go - update the auth middleware to use the new validator
  3. Run tests to verify nothing broke
`

	plan, err := ParsePlan(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Goal != "Refactor the authentication module" {
		t.Errorf("goal = %q, want %q", plan.Goal, "Refactor the authentication module")
	}
	if plan.EstimatedComplexity != "simple" {
		t.Errorf("complexity = %q, want %q", plan.EstimatedComplexity, "simple")
	}
	if len(plan.FilesToModify) != 2 {
		t.Fatalf("files count = %d, want 2; got %v", len(plan.FilesToModify), plan.FilesToModify)
	}
	if len(plan.Steps) < 2 {
		t.Fatalf("steps count = %d, want at least 2", len(plan.Steps))
	}
	if plan.Steps[0].File != "auth.go" {
		t.Errorf("step[0].File = %q, want %q", plan.Steps[0].File, "auth.go")
	}
}

func TestParsePlan_EmptyResponse(t *testing.T) {
	_, err := ParsePlan("")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestParsePlan_NoGoalFallback(t *testing.T) {
	response := `Something without labels

STEPS:
1. [main.go] MODIFY: Add error handling`

	plan, err := ParsePlan(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back to first non-empty line
	if plan.Goal != "Something without labels" {
		t.Errorf("goal = %q, want %q", plan.Goal, "Something without labels")
	}
	if plan.EstimatedComplexity != "moderate" {
		t.Errorf("complexity should default to moderate, got %q", plan.EstimatedComplexity)
	}
}

func TestShouldUseArchitect_ShortPrompt(t *testing.T) {
	tests := []struct {
		prompt string
		want   bool
	}{
		{"fix the typo", false},
		{"what is this?", false},
		{"explain how context works", false},
		{"how does the router work?", false},
	}

	for _, tt := range tests {
		got := ShouldUseArchitect(tt.prompt, 5)
		if got != tt.want {
			t.Errorf("ShouldUseArchitect(%q, 5) = %v, want %v", tt.prompt, got, tt.want)
		}
	}
}

func TestShouldUseArchitect_LongComplexPrompt(t *testing.T) {
	longPrompt := "I need to refactor the entire authentication system to support OAuth2 with multiple providers including Google, GitHub, and Microsoft. " +
		"This involves creating new middleware, updating the user model, adding token refresh logic, " +
		"modifying the database schema, and updating all the API endpoints that currently use basic auth. " +
		"We also need to add proper error handling and logging across all these files."

	if !ShouldUseArchitect(longPrompt, 1) {
		t.Error("expected ShouldUseArchitect to return true for long complex prompt")
	}
}

func TestShouldUseArchitect_SpeedRequest(t *testing.T) {
	prompt := "quick fix: add a nil check to the handler"
	if ShouldUseArchitect(prompt, 1) {
		t.Error("expected ShouldUseArchitect to return false for speed request")
	}
}

func TestShouldUseArchitect_MultiFile(t *testing.T) {
	prompt := "update the error handling across multiple files in the engine package"
	if !ShouldUseArchitect(prompt, 3) {
		t.Error("expected ShouldUseArchitect to return true for multi-file reference")
	}
}

func TestShouldUseArchitect_RefactorKeyword(t *testing.T) {
	prompt := "refactor the database layer"
	if !ShouldUseArchitect(prompt, 1) {
		t.Error("expected ShouldUseArchitect to return true for refactor keyword")
	}
}

func TestBuildEditorPrompt(t *testing.T) {
	plan := &ArchitectPlan{
		Goal:                "Add caching to API",
		EstimatedComplexity: "moderate",
	}
	step := PlanStep{
		Description: "create cache.go: Implement LRU cache",
		File:        "cache.go",
		Action:      "create",
		Details:     "Implement an LRU cache with TTL support and max 1000 entries",
	}

	result := BuildEditorPrompt(plan, step)

	if !strings.Contains(result, "Add caching to API") {
		t.Error("editor prompt should contain the plan goal")
	}
	if !strings.Contains(result, "cache.go") {
		t.Error("editor prompt should contain the file name")
	}
	if !strings.Contains(result, "create") {
		t.Error("editor prompt should contain the action")
	}
	if !strings.Contains(result, "LRU cache with TTL") {
		t.Error("editor prompt should contain the step details")
	}
	if !strings.Contains(result, "Do not deviate from the plan") {
		t.Error("editor prompt should contain the instruction not to deviate")
	}
}

func TestBuildEditorPrompt_NoDetails(t *testing.T) {
	plan := &ArchitectPlan{Goal: "Fix bug"}
	step := PlanStep{
		Description: "modify main.go",
		File:        "main.go",
		Action:      "modify",
	}

	result := BuildEditorPrompt(plan, step)
	if strings.Contains(result, "Details:") {
		t.Error("editor prompt should not contain Details line when details are empty")
	}
}

func TestEstimateSavings(t *testing.T) {
	plan := &ArchitectPlan{
		Goal: "Add feature",
		Steps: []PlanStep{
			{File: "a.go", Action: "create"},
			{File: "b.go", Action: "modify"},
			{File: "c.go", Action: "modify"},
		},
	}

	// haiku-class: $0.25/M, sonnet-class: $3.0/M
	result := EstimateSavings(plan, 0.25, 3.0)

	if !strings.Contains(result, "savings") && !strings.Contains(result, "Estimated") {
		t.Errorf("expected savings info, got: %s", result)
	}
	if !strings.Contains(result, "reduction") {
		t.Errorf("expected percentage reduction in output, got: %s", result)
	}
}

func TestEstimateSavings_NilPlan(t *testing.T) {
	result := EstimateSavings(nil, 0.25, 3.0)
	if !strings.Contains(result, "No savings") {
		t.Errorf("expected no savings message for nil plan, got: %s", result)
	}
}

func TestEstimateSavings_EmptySteps(t *testing.T) {
	plan := &ArchitectPlan{Goal: "test", Steps: []PlanStep{}}
	result := EstimateSavings(plan, 0.25, 3.0)
	if !strings.Contains(result, "No savings") {
		t.Errorf("expected no savings message for empty steps, got: %s", result)
	}
}

func TestEstimateSavings_SamePrice(t *testing.T) {
	plan := &ArchitectPlan{
		Goal:  "test",
		Steps: []PlanStep{{File: "a.go", Action: "modify"}},
	}
	// Same price for both models means no savings
	result := EstimateSavings(plan, 3.0, 3.0)
	if !strings.Contains(result, "No cost savings") {
		t.Errorf("expected no cost savings when prices are equal, got: %s", result)
	}
}

func TestPlan_MultipleSteps(t *testing.T) {
	called := false
	arch := &Architect{
		Config: ArchitectConfig{
			ArchitectModel:  "haiku",
			EditorModel:     "sonnet",
			PlanTokenBudget: 4096,
			Enabled:         true,
		},
		ChatFn: func(ctx context.Context, model string, messages []ArchitectMessage) (string, error) {
			called = true
			if model != "haiku" {
				t.Errorf("expected model %q, got %q", "haiku", model)
			}
			return `GOAL: Implement user authentication
COMPLEXITY: complex
FILES: auth.go, middleware.go, user.go, auth_test.go

STEPS:
1. [auth.go] CREATE: Implement JWT token generation and validation
2. [middleware.go] CREATE: Add auth middleware that validates tokens
3. [user.go] MODIFY: Add password hashing to user model
4. [auth_test.go] CREATE: Add comprehensive tests for auth flow`, nil
		},
	}

	plan, err := arch.Plan(context.Background(), "Implement user authentication with JWT", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("ChatFn was not called")
	}
	if plan.Goal != "Implement user authentication" {
		t.Errorf("goal = %q", plan.Goal)
	}
	if plan.EstimatedComplexity != "complex" {
		t.Errorf("complexity = %q, want %q", plan.EstimatedComplexity, "complex")
	}
	if len(plan.Steps) != 4 {
		t.Fatalf("steps = %d, want 4", len(plan.Steps))
	}
	if len(plan.FilesToModify) != 4 {
		t.Errorf("files = %d, want 4", len(plan.FilesToModify))
	}
	if plan.Steps[2].Action != "modify" {
		t.Errorf("step[2].Action = %q, want %q", plan.Steps[2].Action, "modify")
	}
}

func TestPlan_SingleTrivialStep(t *testing.T) {
	arch := &Architect{
		Config: ArchitectConfig{
			ArchitectModel: "haiku",
			EditorModel:    "sonnet",
			Enabled:        true,
		},
		ChatFn: func(ctx context.Context, model string, messages []ArchitectMessage) (string, error) {
			return `GOAL: Fix nil pointer in handler
COMPLEXITY: trivial
FILES: handler.go

STEPS:
1. [handler.go] MODIFY: Add nil check before accessing request.Body`, nil
		},
	}

	plan, err := arch.Plan(context.Background(), "fix nil pointer", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.EstimatedComplexity != "trivial" {
		t.Errorf("complexity = %q, want %q", plan.EstimatedComplexity, "trivial")
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(plan.Steps))
	}
	if plan.Steps[0].File != "handler.go" {
		t.Errorf("step file = %q, want %q", plan.Steps[0].File, "handler.go")
	}
	if plan.Steps[0].Action != "modify" {
		t.Errorf("step action = %q, want %q", plan.Steps[0].Action, "modify")
	}
}

func TestPlan_WithRepoContext(t *testing.T) {
	var receivedContent string
	arch := &Architect{
		Config: ArchitectConfig{ArchitectModel: "haiku"},
		ChatFn: func(ctx context.Context, model string, messages []ArchitectMessage) (string, error) {
			for _, m := range messages {
				if m.Role == "user" {
					receivedContent = m.Content
				}
			}
			return `GOAL: test
COMPLEXITY: trivial
FILES: a.go

STEPS:
1. [a.go] MODIFY: do something`, nil
		},
	}

	_, err := arch.Plan(context.Background(), "add logging", "package main\n\nfunc main() {}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(receivedContent, "Repository context:") {
		t.Error("expected repo context in user message")
	}
	if !strings.Contains(receivedContent, "package main") {
		t.Error("expected repo context content in user message")
	}
	if !strings.Contains(receivedContent, "add logging") {
		t.Error("expected goal in user message")
	}
}

func TestPlan_NoChatFn(t *testing.T) {
	arch := &Architect{
		Config: ArchitectConfig{Enabled: true},
	}
	_, err := arch.Plan(context.Background(), "test", "")
	if err == nil {
		t.Fatal("expected error when ChatFn is nil")
	}
	if !strings.Contains(err.Error(), "ChatFn is not configured") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestPlan_ChatFnError(t *testing.T) {
	arch := &Architect{
		Config: ArchitectConfig{ArchitectModel: "haiku"},
		ChatFn: func(ctx context.Context, model string, messages []ArchitectMessage) (string, error) {
			return "", context.DeadlineExceeded
		},
	}
	_, err := arch.Plan(context.Background(), "test", "")
	if err == nil {
		t.Fatal("expected error when ChatFn fails")
	}
	if !strings.Contains(err.Error(), "planning failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParsePlan_ComplexityVariants(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"COMPLEXITY: trivial", "trivial"},
		{"COMPLEXITY: Simple", "simple"},
		{"COMPLEXITY: MODERATE", "moderate"},
		{"COMPLEXITY: Complex", "complex"},
		{"COMPLEXITY: medium difficulty", "moderate"},
		{"COMPLEXITY: very complex task", "complex"},
		{"COMPLEXITY: fairly simple change", "simple"},
		{"COMPLEXITY: unknown", "moderate"}, // default
	}

	for _, tt := range tests {
		response := tt.input + "\n\nSTEPS:\n1. [a.go] MODIFY: test"
		plan, err := ParsePlan(response)
		if err != nil {
			t.Fatalf("ParsePlan(%q): unexpected error: %v", tt.input, err)
		}
		if plan.EstimatedComplexity != tt.want {
			t.Errorf("ParsePlan(%q): complexity = %q, want %q", tt.input, plan.EstimatedComplexity, tt.want)
		}
	}
}
