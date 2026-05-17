package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyScale(t *testing.T) {
	tests := []struct {
		input string
		want  TaskScale
	}{
		{"fix the typo in main.go", ScalePatch},
		{"rename getUserName to getUsername", ScalePatch},
		{"add error handling to the API endpoint", ScaleMinor},
		{"implement pagination for /users", ScaleMinor},
		{"refactor the auth module to use JWT", ScaleMajor},
		{"migrate from REST to gRPC", ScaleMajor},
		{"build a new notification service from scratch", ScaleEpic},
		{"create new microservice architecture", ScaleEpic},
		{"hi", ScalePatch},
	}
	for _, tt := range tests {
		got := ClassifyScale(tt.input)
		if got != tt.want {
			t.Errorf("ClassifyScale(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestGetBehavior(t *testing.T) {
	b := GetBehavior(ScalePatch)
	if b.MaxTurns != 3 {
		t.Errorf("patch max turns = %d, want 3", b.MaxTurns)
	}
	if !b.AutoApprove {
		t.Error("patch should auto-approve")
	}

	b = GetBehavior(ScaleEpic)
	if b.MaxTurns != 50 {
		t.Errorf("epic max turns = %d, want 50", b.MaxTurns)
	}
	if !b.PlanRequired {
		t.Error("epic should require plan")
	}
}

func TestAdversarialReview_FormatFindings(t *testing.T) {
	ar := &AdversarialReview{
		Findings: []ReviewFinding{
			{Severity: "HIGH", File: "auth.go", Line: 42, Category: "security", Issue: "No rate limiting", Fix: "Add rate limiter"},
			{Severity: "LOW", File: "utils.go", Line: 10, Category: "logic", Issue: "Magic number", Fix: "Use constant"},
		},
	}
	output := ar.FormatFindings()
	if !hasSubstr(output, "HIGH") {
		t.Error("expected HIGH in output")
	}
	if !hasSubstr(output, "PASS") {
		// 1 HIGH = FAIL
		if !hasSubstr(output, "FAIL") {
			t.Error("expected FAIL verdict with HIGH finding")
		}
	}
}

func TestAdversarialReview_Empty(t *testing.T) {
	ar := &AdversarialReview{}
	output := ar.FormatFindings()
	if !hasSubstr(output, "re-analyze") {
		t.Error("empty findings should demand re-analysis")
	}
}

func TestProjectContext_Load(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hawk"), 0o755)
	os.WriteFile(filepath.Join(dir, ".hawk", "project-context.md"), []byte("## Stack\n- Go 1.21\n- PostgreSQL"), 0o644)

	pc := NewProjectContext(dir)
	content := pc.Load()
	if !hasSubstr(content, "Go 1.21") {
		t.Error("expected project context content")
	}
	if !pc.HasContext() {
		t.Error("expected HasContext = true")
	}
}

func TestProjectContext_NoFiles(t *testing.T) {
	pc := NewProjectContext(t.TempDir())
	content := pc.Load()
	if content != "" {
		t.Error("expected empty content when no files exist")
	}
}

func TestQuickDevClarifyPrompt(t *testing.T) {
	p := QuickDevClarifyPrompt("add auth to the API")
	if !hasSubstr(p, "add auth to the API") {
		t.Error("expected user input in prompt")
	}
	if !hasSubstr(p, "ONE-SHOT") {
		t.Error("expected routing options in prompt")
	}
}

func TestCorrectCoursePrompt(t *testing.T) {
	p := CorrectCoursePrompt("add pagination", "half-implemented", "wrong offset calculation")
	if !hasSubstr(p, "pagination") {
		t.Error("expected intent in prompt")
	}
	if !hasSubstr(p, "Layer") {
		t.Error("expected layer diagnosis in prompt")
	}
}

func TestInvestigatePrompt(t *testing.T) {
	p := InvestigatePrompt(InvestigateReproduce, "test fails with nil pointer")
	if !hasSubstr(p, "Reproduce") {
		t.Error("expected phase name")
	}
	if !hasSubstr(p, "nil pointer") {
		t.Error("expected context in prompt")
	}
}

func TestCheckpointPrompts(t *testing.T) {
	p := CheckpointPrompts(CheckpointOrientation, []string{"main.go"})
	if !hasSubstr(p, "goal") {
		t.Error("expected orientation content")
	}
	p = CheckpointPrompts(CheckpointWrapup, nil)
	if !hasSubstr(p, "READY TO COMMIT") {
		t.Error("expected wrapup verdict options")
	}
}
