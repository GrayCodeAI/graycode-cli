package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withTempCwd(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

// withSpecSession returns a context carrying a fake, session-scoped
// ToolContext for spec-slug storage — mirrors what stream_tool_exec.go
// wires up per real session, so tests exercise the same contract without
// any package-level state.
func withSpecSession(ctx context.Context) context.Context {
	var slug string
	return WithToolContext(ctx, &ToolContext{
		SpecSlugGet: func() string { return slug },
		SpecSlugSet: func(s string) { slug = s },
	})
}

func TestSpecifyWritesSpecFile(t *testing.T) {
	dir := withTempCwd(t)
	ctx := withSpecSession(context.Background())
	input, _ := json.Marshal(map[string]string{"title": "unify permissions", "spec": "problem statement here"})

	result, err := SpecifyTool{}.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "spec.md") {
		t.Errorf("expected result to mention spec.md, got %q", result)
	}

	entries, err := os.ReadDir(filepath.Join(dir, ".graycode", "specs"))
	if err != nil {
		t.Fatalf("expected .graycode/specs directory to exist: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one spec directory, got %d", len(entries))
	}
	if !strings.HasPrefix(entries[0].Name(), "unify-permissions") {
		t.Errorf("expected slug prefix, got %q", entries[0].Name())
	}

	content, err := os.ReadFile(filepath.Join(dir, ".graycode", "specs", entries[0].Name(), "spec.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "problem statement here" {
		t.Errorf("unexpected spec.md content: %q", content)
	}
}

func TestPlanRequiresActiveSpec(t *testing.T) {
	withTempCwd(t)
	ctx := withSpecSession(context.Background())

	input, _ := json.Marshal(map[string]string{"plan": "some plan"})
	if _, err := (PlanTool{}).Execute(ctx, input); err == nil {
		t.Fatal("expected error when no spec is active")
	}
}

func TestSpecifyPlanTasksSequence(t *testing.T) {
	dir := withTempCwd(t)
	ctx := withSpecSession(context.Background())

	specInput, _ := json.Marshal(map[string]string{"title": "sequence test", "spec": "spec content"})
	if _, err := (SpecifyTool{}).Execute(ctx, specInput); err != nil {
		t.Fatal(err)
	}

	planInput, _ := json.Marshal(map[string]string{"plan": "plan content"})
	if _, err := (PlanTool{}).Execute(ctx, planInput); err != nil {
		t.Fatal(err)
	}

	tasksInput, _ := json.Marshal(map[string]string{"tasks": "task content"})
	if _, err := (TasksTool{}).Execute(ctx, tasksInput); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, ".graycode", "specs"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one spec dir, got entries=%v err=%v", entries, err)
	}
	specPath := filepath.Join(dir, ".graycode", "specs", entries[0].Name())
	for _, f := range []string{"spec.md", "plan.md", "tasks.md"} {
		if _, err := os.Stat(filepath.Join(specPath, f)); err != nil {
			t.Errorf("expected %s to exist: %v", f, err)
		}
	}
}

// TestSpecSlug_IsolatedAcrossSessions is the regression test for the
// concurrency bug this session-scoped design fixes: two concurrent
// "sessions" (two separate ToolContexts) must never see or clobber each
// other's active spec slug.
func TestSpecSlug_IsolatedAcrossSessions(t *testing.T) {
	withTempCwd(t)
	ctxA := withSpecSession(context.Background())
	ctxB := withSpecSession(context.Background())

	inputA, _ := json.Marshal(map[string]string{"title": "session-a-task", "spec": "spec A"})
	if _, err := (SpecifyTool{}).Execute(ctxA, inputA); err != nil {
		t.Fatal(err)
	}
	inputB, _ := json.Marshal(map[string]string{"title": "session-b-task", "spec": "spec B"})
	if _, err := (SpecifyTool{}).Execute(ctxB, inputB); err != nil {
		t.Fatal(err)
	}

	slugA, err := specSlug(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	slugB, err := specSlug(ctxB)
	if err != nil {
		t.Fatal(err)
	}
	if slugA == slugB {
		t.Fatalf("expected distinct slugs per session, both got %q", slugA)
	}
	if !strings.HasPrefix(slugA, "session-a-task") {
		t.Errorf("session A slug = %q, want session-a-task prefix", slugA)
	}
	if !strings.HasPrefix(slugB, "session-b-task") {
		t.Errorf("session B slug = %q, want session-b-task prefix", slugB)
	}
}

func TestApproveImplementation(t *testing.T) {
	result, err := ApproveImplementationTool{}.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("should return confirmation")
	}
}

func TestSpecToolMetadata(t *testing.T) {
	t.Parallel()
	if (SpecifyTool{}).Name() != "Specify" {
		t.Errorf("Specify name mismatch")
	}
	if (PlanTool{}).Name() != "Plan" {
		t.Errorf("Plan name mismatch")
	}
	if (TasksTool{}).Name() != "Tasks" {
		t.Errorf("Tasks name mismatch")
	}
	if (ApproveImplementationTool{}).Name() != "ApproveImplementation" {
		t.Errorf("ApproveImplementation name mismatch")
	}
}
