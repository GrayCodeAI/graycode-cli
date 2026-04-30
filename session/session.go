package session

import (
	"bufio"
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
	CWD       string    `json:"cwd,omitempty"`
	Name      string    `json:"name,omitempty"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func sessionsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "sessions")
}

func legacyPathFor(id string) string {
	return filepath.Join(sessionsDir(), id+".json")
}

func jsonlPathFor(id string) string {
	return filepath.Join(sessionsDir(), id+".jsonl")
}

// Save persists a session to disk in JSONL format for archive compatibility.
func Save(s *Session) error {
	if s.CWD == "" {
		if cwd, err := os.Getwd(); err == nil {
			if abs, err := filepath.Abs(cwd); err == nil {
				s.CWD = abs
			} else {
				s.CWD = cwd
			}
		}
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	s.UpdatedAt = time.Now()
	if err := os.MkdirAll(sessionsDir(), 0o755); err != nil {
		return err
	}

	// Write in JSONL format: first line is session metadata, subsequent lines are messages
	path := jsonlPathFor(s.ID)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	// Write session metadata as first line
	meta := map[string]interface{}{
		"type":       "session_meta",
		"id":         s.ID,
		"model":      s.Model,
		"provider":   s.Provider,
		"cwd":        s.CWD,
		"name":       s.Name,
		"created_at": s.CreatedAt.Format(time.RFC3339),
		"updated_at": s.UpdatedAt.Format(time.RFC3339),
	}
	metaData, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if _, err := w.Write(metaData); err != nil {
		return err
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}

	// Write each message as a JSON line
	for _, msg := range s.Messages {
		msgData, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		if _, err := w.Write(msgData); err != nil {
			return err
		}
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
	}

	return nil
}

// Load reads a session from disk, supporting both JSONL and legacy JSON formats.
func Load(id string) (*Session, error) {
	// Try JSONL first
	if s, err := loadJSONL(id); err == nil {
		return s, nil
	}
	// Fall back to legacy JSON
	return loadLegacyJSON(id)
}

func loadJSONL(id string) (*Session, error) {
	path := jsonlPathFor(id)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var s Session
	s.ID = id
	scanner := bufio.NewScanner(f)
	firstLine := true

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if firstLine {
			firstLine = false
			var meta map[string]interface{}
			if err := json.Unmarshal(line, &meta); err != nil {
				return nil, err
			}
			if v, ok := meta["model"].(string); ok {
				s.Model = v
			}
			if v, ok := meta["provider"].(string); ok {
				s.Provider = v
			}
			if v, ok := meta["cwd"].(string); ok {
				s.CWD = v
			}
			if v, ok := meta["name"].(string); ok {
				s.Name = v
			}
			if v, ok := meta["created_at"].(string); ok {
				s.CreatedAt, _ = time.Parse(time.RFC3339, v)
			}
			if v, ok := meta["updated_at"].(string); ok {
				s.UpdatedAt, _ = time.Parse(time.RFC3339, v)
			}
			continue
		}

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, err
		}
		s.Messages = append(s.Messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return &s, nil
}

func loadLegacyJSON(id string) (*Session, error) {
	path := legacyPathFor(id)
	data, err := os.ReadFile(path)
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
	CWD       string
	UpdatedAt time.Time
}

// List returns all saved sessions, newest first.
func List() ([]Entry, error) {
	entries, err := os.ReadDir(sessionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if ext != ".json" && ext != ".jsonl" {
			continue
		}
		id := e.Name()[:len(e.Name())-len(ext)]
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
		out = append(out, Entry{ID: id, Preview: preview, CWD: s.CWD, UpdatedAt: s.UpdatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// LoadLatestForCWD returns the newest saved session for cwd.
func LoadLatestForCWD(cwd string) (*Session, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	entries, err := List()
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.CWD == cwd || e.CWD == "" {
			return Load(e.ID)
		}
	}
	return nil, fmt.Errorf("no saved session for %s", cwd)
}

// LoadLatest returns the newest saved session regardless of CWD.
func LoadLatest() (*Session, error) {
	entries, err := List()
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no saved sessions")
	}
	return Load(entries[0].ID)
}

// MigrateToJSONL converts a legacy JSON session to JSONL format.
func MigrateToJSONL(id string) error {
	s, err := loadLegacyJSON(id)
	if err != nil {
		return err
	}
	return Save(s)
}
