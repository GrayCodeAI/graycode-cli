package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type todoItem struct {
	ID      int    `json:"id"`
	Task    string `json:"task"`
	Done    bool   `json:"done"`
}

var (
	todoMu    sync.Mutex
	todoItems []todoItem
	todoNext  = 1
)

type TodoWriteTool struct{}

func (TodoWriteTool) Name() string { return "todo" }
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
		},
		"required": []string{"action"},
	}
}

func (TodoWriteTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action string `json:"action"`
		Task   string `json:"task"`
		ID     int    `json:"id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	todoMu.Lock()
	defer todoMu.Unlock()

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
		var b strings.Builder
		for _, t := range todoItems {
			mark := "[ ]"
			if t.Done {
				mark = "[x]"
			}
			fmt.Fprintf(&b, "%s #%d: %s\n", mark, t.ID, t.Task)
		}
		return b.String(), nil
	default:
		return "", fmt.Errorf("unknown action: %s", p.Action)
	}
}
