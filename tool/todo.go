package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type todoItem struct {
	ID       int    `json:"id"`
	Task     string `json:"task"`
	Status   string `json:"status,omitempty"`
	Priority string `json:"priority,omitempty"`
	Done     bool   `json:"done"`
}

var (
	todoMu    sync.Mutex
	todoItems []todoItem
	todoNext  = 1
)

type TodoWriteTool struct{}

func (TodoWriteTool) Name() string      { return "TodoWrite" }
func (TodoWriteTool) Aliases() []string { return []string{"todo"} }
func (TodoWriteTool) Description() string {
	return "Manage a task list. Actions: add, complete, list, remove."
}
func (TodoWriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{"type": "string", "enum": []string{"add", "complete", "list", "remove"}, "description": "Action to perform"},
			"task":   map[string]interface{}{"type": "string", "description": "Task description (for add)"},
			"id":     map[string]interface{}{"type": "integer", "description": "Task ID (for complete/remove)"},
			"todos": map[string]interface{}{
				"type":        "array",
				"description": "Archive-compatible full todo list replacement",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"content":  map[string]interface{}{"type": "string"},
						"task":     map[string]interface{}{"type": "string"},
						"status":   map[string]interface{}{"type": "string"},
						"priority": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
	}
}

func (TodoWriteTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action string      `json:"action"`
		Task   string      `json:"task"`
		ID     int         `json:"id"`
		Todos  []todoInput `json:"todos"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	todoMu.Lock()
	defer todoMu.Unlock()

	if p.Todos != nil {
		todoItems = todoItems[:0]
		todoNext = 1
		for _, in := range p.Todos {
			task := strings.TrimSpace(in.Content)
			if task == "" {
				task = strings.TrimSpace(in.Task)
			}
			if task == "" {
				continue
			}
			status := strings.TrimSpace(in.Status)
			done := status == "completed" || status == "done"
			todoItems = append(todoItems, todoItem{
				ID:       todoNext,
				Task:     task,
				Status:   status,
				Priority: strings.TrimSpace(in.Priority),
				Done:     done,
			})
			todoNext++
		}
		return fmt.Sprintf("Updated todo list (%d items):\n%s", len(todoItems), formatTodoItems()), nil
	}

	switch p.Action {
	case "add":
		if p.Task == "" {
			return "", fmt.Errorf("task is required for add")
		}
		todoItems = append(todoItems, todoItem{ID: todoNext, Task: p.Task})
		todoNext++
		return fmt.Sprintf("Added task #%d: %s", todoNext-1, p.Task), nil
	case "complete":
		for i := range todoItems {
			if todoItems[i].ID == p.ID {
				todoItems[i].Done = true
				return fmt.Sprintf("Completed task #%d", p.ID), nil
			}
		}
		return "", fmt.Errorf("task #%d not found", p.ID)
	case "remove":
		for i := range todoItems {
			if todoItems[i].ID == p.ID {
				todoItems = append(todoItems[:i], todoItems[i+1:]...)
				return fmt.Sprintf("Removed task #%d", p.ID), nil
			}
		}
		return "", fmt.Errorf("task #%d not found", p.ID)
	case "list":
		if len(todoItems) == 0 {
			return "No tasks.", nil
		}
		return formatTodoItems(), nil
	default:
		return "", fmt.Errorf("unknown action: %s", p.Action)
	}
}

type todoInput struct {
	Content  string `json:"content"`
	Task     string `json:"task"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

func formatTodoItems() string {
	var b strings.Builder
	for _, t := range todoItems {
		mark := "[ ]"
		if t.Done {
			mark = "[x]"
		}
		extra := ""
		if t.Status != "" || t.Priority != "" {
			var parts []string
			if t.Status != "" {
				parts = append(parts, "status="+t.Status)
			}
			if t.Priority != "" {
				parts = append(parts, "priority="+t.Priority)
			}
			extra = " (" + strings.Join(parts, ", ") + ")"
		}
		fmt.Fprintf(&b, "%s #%d: %s%s\n", mark, t.ID, t.Task, extra)
	}
	return strings.TrimRight(b.String(), "\n")
}
