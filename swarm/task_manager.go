package swarm

import (
	"fmt"
	"sync"
	"time"
)

// TaskState represents the lifecycle state of a task.
type TaskState string

const (
	TaskPending   TaskState = "pending"
	TaskRunning   TaskState = "running"
	TaskCompleted TaskState = "completed"
	TaskFailed    TaskState = "failed"
	TaskKilled    TaskState = "killed"
)

// Task represents a unit of work assigned to a worker.
type Task struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	AgentType   string    `json:"agent_type"`
	AgentName   string    `json:"agent_name,omitempty"`
	State       TaskState `json:"state"`
	Result      string    `json:"result,omitempty"`
	Error       string    `json:"error,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Usage       TaskUsage `json:"usage,omitempty"`
}

// TaskUsage tracks resource usage for a task.
type TaskUsage struct {
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	Duration         time.Duration `json:"duration_ms"`
	ToolCalls        int           `json:"tool_calls"`
}

// TaskManager manages task lifecycle for a coordinator.
type TaskManager struct {
	mu       sync.RWMutex
	tasks    map[string]*Task
	counter  int
	teamName string
}

// NewTaskManager creates a new task manager.
func NewTaskManager(teamName string) *TaskManager {
	return &TaskManager{
		tasks:    make(map[string]*Task),
		teamName: teamName,
	}
}

// Create creates a new task in pending state.
func (tm *TaskManager) Create(description, agentType string) *Task {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.counter++
	task := &Task{
		ID:          fmt.Sprintf("task_%s_%d", tm.teamName, tm.counter),
		Description: description,
		AgentType:   agentType,
		State:       TaskPending,
	}
	tm.tasks[task.ID] = task
	return task
}

// Start marks a task as running.
func (tm *TaskManager) Start(id, agentName string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, ok := tm.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	if task.State != TaskPending {
		return fmt.Errorf("task %s is in state %s, expected pending", id, task.State)
	}
	task.State = TaskRunning
	task.AgentName = agentName
	task.StartedAt = time.Now()
	return nil
}

// Complete marks a task as completed with a result.
func (tm *TaskManager) Complete(id, result string, usage TaskUsage) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, ok := tm.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	task.State = TaskCompleted
	task.Result = result
	task.CompletedAt = time.Now()
	task.Usage = usage
	return nil
}

// Fail marks a task as failed with an error message.
func (tm *TaskManager) Fail(id, errMsg string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, ok := tm.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	task.State = TaskFailed
	task.Error = errMsg
	task.CompletedAt = time.Now()
	return nil
}

// Kill forcefully stops a task.
func (tm *TaskManager) Kill(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, ok := tm.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	if task.State != TaskRunning && task.State != TaskPending {
		return fmt.Errorf("task %s is in state %s, cannot kill", id, task.State)
	}
	task.State = TaskKilled
	task.CompletedAt = time.Now()
	return nil
}

// Get returns a task by ID.
func (tm *TaskManager) Get(id string) (*Task, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	task, ok := tm.tasks[id]
	return task, ok
}

// List returns all tasks.
func (tm *TaskManager) List() []*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	out := make([]*Task, 0, len(tm.tasks))
	for _, t := range tm.tasks {
		out = append(out, t)
	}
	return out
}

// Running returns all currently running tasks.
func (tm *TaskManager) Running() []*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	var out []*Task
	for _, t := range tm.tasks {
		if t.State == TaskRunning {
			out = append(out, t)
		}
	}
	return out
}

// Pending returns all pending tasks.
func (tm *TaskManager) Pending() []*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	var out []*Task
	for _, t := range tm.tasks {
		if t.State == TaskPending {
			out = append(out, t)
		}
	}
	return out
}

// FormatNotification formats a task completion notification in XML for structured agent communication.
func FormatNotification(task *Task) string {
	switch task.State {
	case TaskCompleted:
		return fmt.Sprintf("<task-notification>\n<task-id>%s</task-id>\n<status>completed</status>\n<summary>Agent %q completed: %s</summary>\n<result>%s</result>\n<usage><duration_ms>%d</duration_ms><tool_uses>%d</tool_uses></usage>\n</task-notification>",
			task.ID, task.AgentName, task.Description, task.Result,
			task.Usage.Duration.Milliseconds(), task.Usage.ToolCalls)
	case TaskFailed:
		return fmt.Sprintf("<task-notification>\n<task-id>%s</task-id>\n<status>failed</status>\n<summary>Agent %q failed: %s</summary>\n<error>%s</error>\n</task-notification>",
			task.ID, task.AgentName, task.Description, task.Error)
	case TaskKilled:
		return fmt.Sprintf("<task-notification>\n<task-id>%s</task-id>\n<status>killed</status>\n<summary>Agent %q was stopped</summary>\n</task-notification>",
			task.ID, task.AgentName)
	default:
		return fmt.Sprintf("<task-notification>\n<task-id>%s</task-id>\n<status>%s</status>\n</task-notification>",
			task.ID, task.State)
	}
}
