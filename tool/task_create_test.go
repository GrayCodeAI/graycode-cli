package tool

import (
	"context"
	"encoding/json"
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
