package tool

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTaskRunToolExecutesReadyTasks(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	s.Create("a", "do a", "", nil)
	s.Create("b", "do b", "", nil)
	// Replace the global store reference used by the tool for the test.
	prev := globalTaskStore
	globalTaskStore = s
	defer func() { globalTaskStore = prev }()

	exec := newFakeExecutor(nil)
	ctx := WithToolContext(context.Background(), &ToolContext{TaskExecutor: exec.execute})
	out, err := TaskRunTool{}.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "2 completed") {
		t.Fatalf("expected 2 completed in summary, got: %s", out)
	}
	got, _ := s.Get("task_1")
	if got.Status != TaskStatusCompleted {
		t.Fatalf("task_1 status = %q, want completed", got.Status)
	}
}

func TestTaskRunToolRequiresExecutor(t *testing.T) {
	ctx := WithToolContext(context.Background(), &ToolContext{})
	_, err := TaskRunTool{}.Execute(ctx, nil)
	if err == nil {
		t.Fatal("expected error when no executor configured")
	}
	if !strings.Contains(err.Error(), "task executor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskRunToolCancelsOnContextTimeout(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	s.Create("slow", "slow task", "", nil)
	prev := globalTaskStore
	globalTaskStore = s
	defer func() { globalTaskStore = prev }()

	exec := func(ctx context.Context, t *Task) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}

	runCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	runCtx = WithToolContext(runCtx, &ToolContext{TaskExecutor: exec})
	_, err := TaskRunTool{}.Execute(runCtx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := s.Get("task_1")
	if got.Status != TaskStatusCancelled {
		t.Fatalf("expected task cancelled on ctx timeout, got %q", got.Status)
	}
}
