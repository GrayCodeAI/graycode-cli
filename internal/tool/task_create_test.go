package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskCreateTool(t *testing.T) {
	globalTaskStore.Reset()

	input, _ := json.Marshal(map[string]string{
		"subject":     "Fix auth bug",
		"description": "The login flow breaks when session expires",
	})

	result, err := (TaskCreateTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Task struct {
			ID      string `json:"id"`
			Subject string `json:"subject"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp.Task.ID == "" {
		t.Fatal("expected task ID")
	}
	if resp.Task.Subject != "Fix auth bug" {
		t.Fatalf("expected subject 'Fix auth bug', got %q", resp.Task.Subject)
	}
}

func TestTaskGetTool(t *testing.T) {
	globalTaskStore.Reset()
	task := globalTaskStore.Create("Test task", "description", "", nil)

	input, _ := json.Marshal(map[string]string{"taskId": task.ID})
	result, err := (TaskGetTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Task struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Task.ID != task.ID {
		t.Fatalf("expected ID %q, got %q", task.ID, resp.Task.ID)
	}
	if resp.Task.Status != "pending" {
		t.Fatalf("expected status 'pending', got %q", resp.Task.Status)
	}
}

func TestTaskGetTool_NotFound(t *testing.T) {
	globalTaskStore.Reset()
	input, _ := json.Marshal(map[string]string{"taskId": "nonexistent"})
	result, err := (TaskGetTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Task *struct{} `json:"task"`
	}
	json.Unmarshal([]byte(result), &resp)
	if resp.Task != nil {
		t.Fatal("expected null task")
	}
}

func TestTaskListTool(t *testing.T) {
	globalTaskStore.Reset()
	globalTaskStore.Create("Task 1", "desc 1", "", nil)
	globalTaskStore.Create("Task 2", "desc 2", "", nil)

	result, err := (TaskListTool{}).Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	}
	json.Unmarshal([]byte(result), &resp)
	if len(resp.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(resp.Tasks))
	}
}

func TestTaskUpdateTool(t *testing.T) {
	globalTaskStore.Reset()
	task := globalTaskStore.Create("Update me", "test", "", nil)

	input, _ := json.Marshal(map[string]string{
		"taskId": task.ID,
		"status": "in_progress",
		"owner":  "agent-1",
	})
	result, err := (TaskUpdateTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Task struct {
			Status string `json:"status"`
			Owner  string `json:"owner"`
		} `json:"task"`
	}
	json.Unmarshal([]byte(result), &resp)
	if resp.Task.Status != "in_progress" {
		t.Fatalf("expected 'in_progress', got %q", resp.Task.Status)
	}
	if resp.Task.Owner != "agent-1" {
		t.Fatalf("expected 'agent-1', got %q", resp.Task.Owner)
	}
}

func TestTaskUpdateTool_NotFound(t *testing.T) {
	globalTaskStore.Reset()
	input, _ := json.Marshal(map[string]string{"taskId": "bad_id", "status": "completed"})
	_, err := (TaskUpdateTool{}).Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

// --- Beads pattern tests ---

func TestHierarchicalTaskIDs(t *testing.T) {
	globalTaskStore.Reset()
	parent := globalTaskStore.Create("Parent", "parent task", "", nil)
	child1 := globalTaskStore.CreateWithParent("Child 1", "first child", "", nil, parent.ID)
	child2 := globalTaskStore.CreateWithParent("Child 2", "second child", "", nil, parent.ID)

	if child1.ID != parent.ID+".1" {
		t.Fatalf("expected %q, got %q", parent.ID+".1", child1.ID)
	}
	if child2.ID != parent.ID+".2" {
		t.Fatalf("expected %q, got %q", parent.ID+".2", child2.ID)
	}
	if child1.ParentID != parent.ID {
		t.Fatalf("expected parentId %q, got %q", parent.ID, child1.ParentID)
	}
	// Child should have parent-child dependency
	found := false
	for _, dep := range child1.Dependencies {
		if dep.Type == "parent-child" && dep.TargetID == parent.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("expected parent-child dependency on child task")
	}
}

func TestTypedDependencies(t *testing.T) {
	globalTaskStore.Reset()
	t1 := globalTaskStore.Create("Task A", "a", "", nil)
	t2 := globalTaskStore.Create("Task B", "b", "", nil)

	globalTaskStore.Update(t2.ID, func(task *Task) {
		task.Dependencies = []TaskDependency{
			{TargetID: t1.ID, Type: "blocks"},
			{TargetID: t1.ID, Type: "related"},
		}
	})

	updated, _ := globalTaskStore.Get(t2.ID)
	if len(updated.Dependencies) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(updated.Dependencies))
	}
	if updated.Dependencies[0].Type != "blocks" {
		t.Fatalf("expected type 'blocks', got %q", updated.Dependencies[0].Type)
	}
}

func TestGetReadyWork(t *testing.T) {
	globalTaskStore.Reset()
	t1 := globalTaskStore.Create("Blocker", "blocks others", "", nil)
	t2 := globalTaskStore.Create("Blocked", "blocked by t1", "", nil)
	globalTaskStore.Create("Free", "no blockers", "", nil)

	globalTaskStore.Update(t2.ID, func(task *Task) {
		task.Dependencies = []TaskDependency{{TargetID: t1.ID, Type: "blocks"}}
	})

	ready := globalTaskStore.GetReadyWork()
	// t1 (pending, no blockers) and "Free" (pending, no blockers) should be ready
	// t2 is blocked by t1 which is not completed
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready tasks, got %d", len(ready))
	}

	// Complete t1, now t2 should also be ready
	globalTaskStore.Update(t1.ID, func(task *Task) {
		task.Status = TaskStatusCompleted
	})
	ready = globalTaskStore.GetReadyWork()
	// t1 is completed (not pending), t2 and Free are ready
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready tasks after completing blocker, got %d", len(ready))
	}
}

func TestGetReadyWorkAction(t *testing.T) {
	globalTaskStore.Reset()
	globalTaskStore.Create("Ready task", "ready", "", nil)

	input, _ := json.Marshal(map[string]string{"action": "ready"})
	result, err := (TaskListTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	}
	json.Unmarshal([]byte(result), &resp)
	if len(resp.Tasks) != 1 {
		t.Fatalf("expected 1 ready task, got %d", len(resp.Tasks))
	}
}

func TestCompactCompleted(t *testing.T) {
	globalTaskStore.Reset()
	t1 := globalTaskStore.Create("Done", "completed", "", nil)
	globalTaskStore.Create("Pending", "still pending", "", nil)

	globalTaskStore.Update(t1.ID, func(task *Task) {
		task.Status = TaskStatusCompleted
	})

	summary := globalTaskStore.CompactCompleted()
	if !strings.Contains(summary, "Compacted 1") {
		t.Fatalf("expected compaction summary, got %q", summary)
	}

	tasks := globalTaskStore.List()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 remaining task, got %d", len(tasks))
	}
}

func TestCompactAction(t *testing.T) {
	globalTaskStore.Reset()
	t1 := globalTaskStore.Create("Done", "done", "", nil)
	globalTaskStore.Update(t1.ID, func(task *Task) {
		task.Status = TaskStatusCompleted
	})

	input, _ := json.Marshal(map[string]string{"action": "compact"})
	result, err := (TaskListTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Result string `json:"result"`
	}
	json.Unmarshal([]byte(result), &resp)
	if !strings.Contains(resp.Result, "Compacted 1") {
		t.Fatalf("expected compaction result, got %q", resp.Result)
	}
}

func TestTaskCreateWithParentViaTool(t *testing.T) {
	globalTaskStore.Reset()
	parent := globalTaskStore.Create("Parent", "parent", "", nil)

	input, _ := json.Marshal(map[string]any{
		"subject":     "Child task",
		"description": "child of parent",
		"parentId":    parent.ID,
	})
	result, err := (TaskCreateTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Task struct {
			ID       string `json:"id"`
			ParentID string `json:"parentId"`
		} `json:"task"`
	}
	json.Unmarshal([]byte(result), &resp)
	if resp.Task.ParentID != parent.ID {
		t.Fatalf("expected parentId %q, got %q", parent.ID, resp.Task.ParentID)
	}
	if resp.Task.ID != parent.ID+".1" {
		t.Fatalf("expected child ID %q, got %q", parent.ID+".1", resp.Task.ID)
	}
}
