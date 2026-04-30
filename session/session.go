package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Message is a persisted conversation message.
type Message struct {
	Role       string      `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolUse    []ToolCall  `json:"tool_use,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
}

// ToolCall mirrors client.ToolCall for persistence.
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolResult mirrors client.ToolResult for persistence.
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Session is a persisted conversation.
type Session struct {
	ID        string    `json:"id"`
	Model     string    `json:"model"`
	Provider  string    `json:"provider"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "sessions")
}

func pathFor(id string) string {
	return filepath.Join(dir(), id+".json")
}

// Save persists a session to disk.
func Save(s *Session) error {
	s.UpdatedAt = time.Now()
	if err := os.MkdirAll(dir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pathFor(s.ID), data, 0o644)
}

// Load reads a session from disk.
func Load(id string) (*Session, error) {
	data, err := os.ReadFile(pathFor(id))
	if err != nil {
		return nil, fmt.Errorf("session %s not found", id)
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Entry is a summary of a saved session for listing.
type Entry struct {
	ID        string
	Preview   string
	UpdatedAt time.Time
}

// List returns all saved sessions, newest first.
func List() ([]Entry, error) {
	entries, err := os.ReadDir(dir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5]
		s, err := Load(id)
		if err != nil {
			continue
		}
		preview := ""
		for _, m := range s.Messages {
			if m.Role == "user" {
				preview = m.Content
				if len(preview) > 80 {
					preview = preview[:80] + "..."
				}
				break
			}
		}
		out = append(out, Entry{ID: id, Preview: preview, UpdatedAt: s.UpdatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
