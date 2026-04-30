package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// TaskStatus represents the state of a task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
)

// Task represents a structured task in the task list.
type Task struct {
	ID          string            `json:"id"`
	Subject     string            `json:"subject"`
	Description string            `json:"description"`
	ActiveForm  string            `json:"activeForm,omitempty"`
	Status      TaskStatus        `json:"status"`
	Owner       string            `json:"owner,omitempty"`
	Blocks      []string          `json:"blocks"`
	BlockedBy   []string          `json:"blockedBy"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// TaskStore is a thread-safe in-memory store for tasks.
type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*Task
	next  int
}

// Global task store.
var globalTaskStore = &TaskStore{tasks: make(map[string]*Task)}

// GetTaskStore returns the global task store.
func GetTaskStore() *TaskStore { return globalTaskStore }

func (s *TaskStore) Create(subject, description, activeForm string, metadata map[string]any) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	id := fmt.Sprintf("task_%d", s.next)
	now := time.Now()
	t := &Task{
		ID:          id,
		Subject:     subject,
		Description: description,
		ActiveForm:  activeForm,
		Status:      TaskStatusPending,
		Blocks:      []string{},
		BlockedBy:   []string{},
		Metadata:    metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.tasks[id] = t
	return t
}

func (s *TaskStore) Get(id string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	return t, ok
}

func (s *TaskStore) List() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		out = append(out, t)
	}
	return out
}

func (s *TaskStore) Update(id string, fn func(*Task)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return false
	}
	fn(t)
	t.UpdatedAt = time.Now()
	return true
}

func (s *TaskStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.tasks[id]
	if ok {
		delete(s.tasks, id)
	}
	return ok
}

func (s *TaskStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = make(map[string]*Task)
	s.next = 0
}

// TaskCreateTool creates a new task in the task list.
type TaskCreateTool struct{}

func (TaskCreateTool) Name() string        { return "TaskCreate" }
func (TaskCreateTool) Aliases() []string   { return []string{"task_create"} }
func (TaskCreateTool) Description() string { return "Create a new task in the task list" }
func (TaskCreateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"subject":     map[string]interface{}{"type": "string", "description": "A brief title for the task"},
			"description": map[string]interface{}{"type": "string", "description": "What needs to be done"},
			"activeForm":  map[string]interface{}{"type": "string", "description": "Present continuous form shown in spinner when in_progress (e.g., \"Running tests\")"},
			"metadata":    map[string]interface{}{"type": "object", "description": "Arbitrary metadata to attach to the task"},
		},
		"required": []string{"subject", "description"},
	}
}

func (TaskCreateTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Subject     string         `json:"subject"`
		Description string         `json:"description"`
		ActiveForm  string         `json:"activeForm"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Subject == "" {
		return "", fmt.Errorf("subject is required")
	}
	if p.Description == "" {
		return "", fmt.Errorf("description is required")
	}
	task := globalTaskStore.Create(p.Subject, p.Description, p.ActiveForm, p.Metadata)
	out, _ := json.Marshal(map[string]any{
		"task": map[string]any{"id": task.ID, "subject": task.Subject},
	})
	return string(out), nil
}

// TaskGetTool retrieves a task by ID.
type TaskGetTool struct{}

func (TaskGetTool) Name() string        { return "TaskGet" }
func (TaskGetTool) Aliases() []string   { return []string{"task_get"} }
func (TaskGetTool) Description() string { return "Get a task by ID from the task list" }
func (TaskGetTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"taskId": map[string]interface{}{"type": "string", "description": "The ID of the task to retrieve"},
		},
		"required": []string{"taskId"},
	}
}

func (TaskGetTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	task, ok := globalTaskStore.Get(p.TaskID)
	if !ok {
		out, _ := json.Marshal(map[string]any{"task": nil})
		return string(out), nil
	}
	out, _ := json.Marshal(map[string]any{
		"task": map[string]any{
			"id":          task.ID,
			"subject":     task.Subject,
			"description": task.Description,
			"status":      task.Status,
			"blocks":      task.Blocks,
			"blockedBy":   task.BlockedBy,
		},
	})
	return string(out), nil
}

// TaskListTool lists all tasks.
type TaskListTool struct{}

func (TaskListTool) Name() string        { return "TaskList" }
func (TaskListTool) Aliases() []string   { return []string{"task_list"} }
func (TaskListTool) Description() string { return "List all tasks in the task list" }
func (TaskListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (TaskListTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	tasks := globalTaskStore.List()
	summaries := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		summaries = append(summaries, map[string]any{
			"id":        t.ID,
			"subject":   t.Subject,
			"status":    t.Status,
			"owner":     t.Owner,
			"blockedBy": t.BlockedBy,
		})
	}
	out, _ := json.Marshal(map[string]any{"tasks": summaries})
	return string(out), nil
}

// TaskUpdateTool updates task fields.
type TaskUpdateTool struct{}

func (TaskUpdateTool) Name() string        { return "TaskUpdate" }
func (TaskUpdateTool) Aliases() []string   { return []string{"task_update"} }
func (TaskUpdateTool) Description() string { return "Update a task's status, owner, or dependencies" }
func (TaskUpdateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"taskId":    map[string]interface{}{"type": "string", "description": "The ID of the task to update"},
			"status":    map[string]interface{}{"type": "string", "enum": []string{"pending", "in_progress", "completed"}, "description": "New task status"},
			"owner":     map[string]interface{}{"type": "string", "description": "Agent name to assign"},
			"blocks":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Task IDs this task blocks"},
			"blockedBy": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Task IDs blocking this task"},
		},
		"required": []string{"taskId"},
	}
}

func (TaskUpdateTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		TaskID    string   `json:"taskId"`
		Status    string   `json:"status"`
		Owner     string   `json:"owner"`
		Blocks    []string `json:"blocks"`
		BlockedBy []string `json:"blockedBy"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.TaskID == "" {
		return "", fmt.Errorf("taskId is required")
	}
	ok := globalTaskStore.Update(p.TaskID, func(t *Task) {
		if p.Status != "" {
			t.Status = TaskStatus(p.Status)
		}
		if p.Owner != "" {
			t.Owner = p.Owner
		}
		if p.Blocks != nil {
			t.Blocks = p.Blocks
		}
		if p.BlockedBy != nil {
			t.BlockedBy = p.BlockedBy
		}
	})
	if !ok {
		return "", fmt.Errorf("task %q not found", p.TaskID)
	}
	task, _ := globalTaskStore.Get(p.TaskID)
	out, _ := json.Marshal(map[string]any{
		"task": map[string]any{
			"id":      task.ID,
			"subject": task.Subject,
			"status":  task.Status,
			"owner":   task.Owner,
		},
	})
	return string(out), nil
}
