package swarm

import (
	"os"
	"testing"
	"time"
)

func TestIsCoordinatorMode(t *testing.T) {
	os.Unsetenv("HAWK_CODE_COORDINATOR_MODE")
	if IsCoordinatorMode() {
		t.Error("should be false when env not set")
	}

	os.Setenv("HAWK_CODE_COORDINATOR_MODE", "1")
	defer os.Unsetenv("HAWK_CODE_COORDINATOR_MODE")
	if !IsCoordinatorMode() {
		t.Error("should be true when env set to 1")
	}
}

func TestMatchSessionMode(t *testing.T) {
	os.Unsetenv("HAWK_CODE_COORDINATOR_MODE")

	if warn := MatchSessionMode("standard"); warn != "" {
		t.Errorf("expected no warning for matching mode, got %q", warn)
	}
	if warn := MatchSessionMode(""); warn != "" {
		t.Errorf("expected no warning for empty stored mode, got %q", warn)
	}
	if warn := MatchSessionMode("coordinator"); warn == "" {
		t.Error("expected warning for coordinator mode mismatch")
	}
}

func TestGetWorkerTools(t *testing.T) {
	simple := GetWorkerTools(WorkerToolsSimple, nil, nil)
	if len(simple) != 7 {
		t.Errorf("expected 7 simple tools, got %d", len(simple))
	}

	allTools := []string{"Bash", "Read", "Edit", "Write", "TeamCreate", "CronCreate", "Grep"}
	full := GetWorkerTools(WorkerToolsFull, allTools, []string{"mcp__server__tool"})
	for _, tool := range full {
		if excludedFromWorkers[tool] {
			t.Errorf("excluded tool %s should not be in worker tools", tool)
		}
	}
	hasMCP := false
	for _, tool := range full {
		if tool == "mcp__server__tool" {
			hasMCP = true
		}
	}
	if !hasMCP {
		t.Error("MCP tools should be included in full worker tools")
	}
}

func TestGetCoordinatorSystemPrompt(t *testing.T) {
	prompt := GetCoordinatorSystemPrompt()
	if prompt == "" {
		t.Error("coordinator prompt should not be empty")
	}
	if len(prompt) < 500 {
		t.Error("coordinator prompt seems too short")
	}
}

func TestTaskManager_Lifecycle(t *testing.T) {
	tm := NewTaskManager("test-team")

	task := tm.Create("implement feature X", "worker")
	if task.State != TaskPending {
		t.Errorf("expected pending, got %s", task.State)
	}
	if task.ID == "" {
		t.Error("task ID should not be empty")
	}

	if err := tm.Start(task.ID, "agent-1"); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if task.State != TaskRunning {
		t.Errorf("expected running, got %s", task.State)
	}
	if task.AgentName != "agent-1" {
		t.Errorf("expected agent-1, got %s", task.AgentName)
	}

	usage := TaskUsage{PromptTokens: 1000, CompletionTokens: 500, Duration: 5 * time.Second, ToolCalls: 3}
	if err := tm.Complete(task.ID, "done successfully", usage); err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if task.State != TaskCompleted {
		t.Errorf("expected completed, got %s", task.State)
	}
	if task.Result != "done successfully" {
		t.Errorf("unexpected result: %s", task.Result)
	}
}

func TestTaskManager_Kill(t *testing.T) {
	tm := NewTaskManager("test")
	task := tm.Create("long running", "worker")
	tm.Start(task.ID, "agent-2")

	if err := tm.Kill(task.ID); err != nil {
		t.Fatalf("Kill error: %v", err)
	}
	if task.State != TaskKilled {
		t.Errorf("expected killed, got %s", task.State)
	}
}

func TestTaskManager_Fail(t *testing.T) {
	tm := NewTaskManager("test")
	task := tm.Create("will fail", "worker")
	tm.Start(task.ID, "agent-3")

	if err := tm.Fail(task.ID, "timeout exceeded"); err != nil {
		t.Fatalf("Fail error: %v", err)
	}
	if task.State != TaskFailed {
		t.Errorf("expected failed, got %s", task.State)
	}
	if task.Error != "timeout exceeded" {
		t.Errorf("unexpected error: %s", task.Error)
	}
}

func TestTaskManager_List(t *testing.T) {
	tm := NewTaskManager("test")
	tm.Create("task1", "worker")
	tm.Create("task2", "researcher")
	tm.Create("task3", "worker")

	all := tm.List()
	if len(all) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(all))
	}

	pending := tm.Pending()
	if len(pending) != 3 {
		t.Errorf("expected 3 pending, got %d", len(pending))
	}

	running := tm.Running()
	if len(running) != 0 {
		t.Errorf("expected 0 running, got %d", len(running))
	}
}

func TestFormatNotification(t *testing.T) {
	task := &Task{
		ID:          "task_test_1",
		Description: "fix the bug",
		AgentName:   "worker-1",
		State:       TaskCompleted,
		Result:      "Fixed the null pointer in auth.go",
		Usage:       TaskUsage{Duration: 3 * time.Second, ToolCalls: 5},
	}

	notif := FormatNotification(task)
	if notif == "" {
		t.Error("notification should not be empty")
	}
	if len(notif) < 50 {
		t.Error("notification seems too short")
	}

	failedTask := &Task{
		ID:          "task_test_2",
		Description: "refactor module",
		AgentName:   "worker-2",
		State:       TaskFailed,
		Error:       "compilation error",
	}
	failNotif := FormatNotification(failedTask)
	if failNotif == "" {
		t.Error("failed notification should not be empty")
	}
}
