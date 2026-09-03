package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/schedule"
)

// ScheduleCreateTool creates an in-conversation scheduled reminder or task.
type ScheduleCreateTool struct {
	Manager *schedule.Manager
}

func (ScheduleCreateTool) Name() string { return "ScheduleCreate" }
func (ScheduleCreateTool) Aliases() []string {
	return []string{"schedule_create", "schedule-create", "remind"}
}

func (ScheduleCreateTool) Description() string {
	return "Create an in-conversation scheduled reminder or recurring task whose state lives in the session log."
}

func (ScheduleCreateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Reminder or instruction prompt to deliver into the conversation when due",
			},
			"duration": map[string]interface{}{
				"type":        "string",
				"description": "Relative duration from now until due (e.g. '10m', '1h', '30s')",
			},
			"at": map[string]interface{}{
				"type":        "string",
				"description": "Optional absolute due time in RFC3339 format (e.g. '2026-08-20T10:00:00Z')",
			},
			"recurring": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether the reminder should repeat periodically",
			},
			"interval": map[string]interface{}{
				"type":        "string",
				"description": "Repeat interval if recurring (e.g. '15m', '1h', '24h')",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t ScheduleCreateTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Prompt    string `json:"prompt"`
		Duration  string `json:"duration"`
		At        string `json:"at"`
		Recurring bool   `json:"recurring"`
		Interval  string `json:"interval"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &p); err != nil {
			return "", fmt.Errorf("invalid parameters: %w", err)
		}
	}

	if strings.TrimSpace(p.Prompt) == "" {
		return "", fmt.Errorf("prompt is required")
	}

	mgr := t.Manager
	if mgr == nil {
		mgr = schedule.DefaultManager()
	}

	now := time.Now()
	var dueAt time.Time

	if p.At != "" {
		parsed, err := time.Parse(time.RFC3339, p.At)
		if err != nil {
			return "", fmt.Errorf("invalid 'at' timestamp format (expected RFC3339): %w", err)
		}
		dueAt = parsed
	} else if p.Duration != "" {
		dur, err := time.ParseDuration(p.Duration)
		if err != nil {
			return "", fmt.Errorf("invalid duration (e.g. '10m', '1h'): %w", err)
		}
		dueAt = now.Add(dur)
	} else {
		// Default 5 minutes if neither at nor duration specified
		dueAt = now.Add(5 * time.Minute)
	}

	item, err := mgr.Create(p.Prompt, dueAt, p.Interval, p.Recurring)
	if err != nil {
		return "", err
	}

	recurringNotice := ""
	if item.Recurring {
		recurringNotice = fmt.Sprintf(" (repeating every %s)", item.Interval)
	}

	return fmt.Sprintf("Scheduled in-conversation reminder `%s` due at %s%s.\nPrompt: %s",
		item.ID, item.DueAt.Format(time.RFC3339), recurringNotice, item.Prompt), nil
}

// ScheduleListTool lists active in-conversation reminders.
type ScheduleListTool struct {
	Manager *schedule.Manager
}

func (ScheduleListTool) Name() string      { return "ScheduleList" }
func (ScheduleListTool) Aliases() []string { return []string{"schedule_list", "schedule-list"} }
func (ScheduleListTool) Description() string {
	return "List all active in-conversation scheduled reminders and tasks for this session."
}

func (ScheduleListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t ScheduleListTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	mgr := t.Manager
	if mgr == nil {
		mgr = schedule.DefaultManager()
	}

	items := mgr.List()
	if len(items) == 0 {
		return "No active in-conversation schedules found for this session.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Active schedules (%d):\n\n", len(items)))
	for _, item := range items {
		recurring := ""
		if item.Recurring {
			recurring = fmt.Sprintf(" [Recurring: %s]", item.Interval)
		}
		sb.WriteString(fmt.Sprintf("- **`%s`** due at %s%s\n  Prompt: %s\n",
			item.ID, item.DueAt.Format(time.RFC3339), recurring, item.Prompt))
	}

	return strings.TrimRight(sb.String(), "\n"), nil
}

// ScheduleDeleteTool deletes/cancels an in-conversation reminder.
type ScheduleDeleteTool struct {
	Manager *schedule.Manager
}

func (ScheduleDeleteTool) Name() string      { return "ScheduleDelete" }
func (ScheduleDeleteTool) Aliases() []string { return []string{"schedule_delete", "schedule-delete"} }

func (ScheduleDeleteTool) Description() string {
	return "Cancel/delete an in-conversation scheduled reminder by ID."
}

func (ScheduleDeleteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Schedule ID to cancel (e.g. 'sched-1234abcd')",
			},
			"reason": map[string]interface{}{
				"type":        "string",
				"description": "Optional cancellation reason",
			},
		},
		"required": []string{"id"},
	}
}

func (t ScheduleDeleteTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &p); err != nil {
			return "", fmt.Errorf("invalid parameters: %w", err)
		}
	}

	if strings.TrimSpace(p.ID) == "" {
		return "", fmt.Errorf("id parameter is required")
	}

	mgr := t.Manager
	if mgr == nil {
		mgr = schedule.DefaultManager()
	}

	if err := mgr.Delete(p.ID, p.Reason); err != nil {
		return "", err
	}

	return fmt.Sprintf("Cancelled scheduled reminder `%s`.", p.ID), nil
}
