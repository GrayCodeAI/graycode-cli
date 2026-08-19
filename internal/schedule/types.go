package schedule

import (
	"time"
)

// Item represents an in-conversation scheduled reminder or task.
type Item struct {
	ID        string     `json:"id"`
	Prompt    string     `json:"prompt"`
	DueAt     time.Time  `json:"due_at"`
	Interval  string     `json:"interval,omitempty"`
	Recurring bool       `json:"recurring"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	LastDueAt *time.Time `json:"last_due_at,omitempty"`
	Deleted   bool       `json:"deleted"`
}
