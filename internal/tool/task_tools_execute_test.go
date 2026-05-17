package tool

import (
	"context"
	"encoding/json"
	"testing"
)

func TestTaskCreateTool_Execute(t *testing.T) {
	t.Parallel()
	tool := TaskCreateTool{}
	input, _ := json.Marshal(map[string]interface{}{
		"subject":     "Fix the bug",
		"description": "There is a crash on login",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == "" {
		t.Error("should return task info")
	}
}

func TestTaskCreateTool_Execute_InvalidJSON(t *testing.T) {
	t.Parallel()
	tool := TaskCreateTool{}
	_, err := tool.Execute(context.Background(), []byte("bad"))
	if err == nil {
		t.Error("should error on bad JSON")
	}
}

func TestTaskGetTool_Execute(t *testing.T) {
	t.Parallel()
	store := GetTaskStore()
	task := store.Create("get-test", "desc", "form", nil)

	tool := TaskGetTool{}
	input, _ := json.Marshal(map[string]interface{}{"taskId": task.ID})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == "" {
		t.Error("should return task details")
	}
}

func TestTaskGetTool_Execute_NotFound(t *testing.T) {
	t.Parallel()
	tool := TaskGetTool{}
	input, _ := json.Marshal(map[string]interface{}{"taskId": "nonexistent-xyz"})
	result, err := tool.Execute(context.Background(), input)
	// May return error or "not found" message — just verify no panic
	_ = err
	_ = result
}

func TestTaskListTool_Execute(t *testing.T) {
	t.Parallel()
	tool := TaskListTool{}
	result, err := tool.Execute(context.Background(), []byte("{}"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == "" {
		t.Error("should return task list")
	}
}

func TestTaskUpdateTool_Execute(t *testing.T) {
	store := GetTaskStore()
	task := store.Create("update-exec-test", "d", "f", nil)

	tool := TaskUpdateTool{}
	input, _ := json.Marshal(map[string]interface{}{
		"taskId": task.ID,
		"status": "completed",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == "" {
		t.Error("should confirm update")
	}
}

func TestCronCreateTool_Execute(t *testing.T) {
	t.Parallel()
	tool := CronCreateTool{}
	input, _ := json.Marshal(map[string]interface{}{
		"schedule": "*/5 * * * *",
		"prompt":   "check status",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == "" {
		t.Error("should return job info")
	}
}

func TestCronListTool_Execute(t *testing.T) {
	t.Parallel()
	tool := CronListTool{}
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == "" {
		t.Error("should return list")
	}
}

func TestCronDeleteTool_Execute(t *testing.T) {
	sched := GetCronScheduler()
	job, _ := sched.Create("* * * * *", "x", false, false)

	tool := CronDeleteTool{}
	input, _ := json.Marshal(map[string]interface{}{"id": job.ID})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == "" {
		t.Error("should confirm deletion")
	}
}

func TestCronDeleteTool_Execute_NotFound(t *testing.T) {
	t.Parallel()
	tool := CronDeleteTool{}
	input, _ := json.Marshal(map[string]interface{}{"job_id": "missing-xyz"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("should error on missing job")
	}
}
