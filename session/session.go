package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
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

// Save persists a session to disk atomically.
// Writes to a temp file first, then renames — a crash at any point
// leaves either the old valid file or the new valid file, never a partial write.
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

	dir := sessionsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create sessions directory: %w", err)
	}

	// Write to temp file, then atomic rename
	target := jsonlPathFor(s.ID)
	tmp := target + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create temp session file: %w", err)
	}

	w := bufio.NewWriter(f)

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
	metaData, _ := json.Marshal(meta)
	w.Write(metaData)
	w.WriteByte('\n')

	// Write each message as a JSON line
	for _, msg := range s.Messages {
		msgData, _ := json.Marshal(msg)
		w.Write(msgData)
		w.WriteByte('\n')
	}

	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("flush session: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("sync session: %w", err)
	}
	f.Close()

	// Atomic rename: either old file or new file, never partial
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}

// WAL (Write-Ahead Log) appends messages incrementally for crash recovery.
// Each message is appended immediately — if hawk crashes, the WAL has everything.
type WAL struct {
	mu   sync.Mutex
	f    *os.File
	path string
	id   string
}

// NewWAL creates or opens a write-ahead log for a session.
func NewWAL(sessionID string) (*WAL, error) {
	dir := sessionsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	path := filepath.Join(dir, sessionID+".wal")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening WAL: %w", err)
	}

	return &WAL{f: f, path: path, id: sessionID}, nil
}

// Append writes a message to the WAL immediately. This is crash-safe:
// even if the process dies right after, the message is on disk.
func (w *WAL) Append(msg Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if _, err := w.f.Write(data); err != nil {
		return err
	}
	return w.f.Sync()
}

// AppendMeta writes session metadata to the WAL.
func (w *WAL) AppendMeta(model, provider, cwd string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	meta := map[string]interface{}{
		"type":       "session_meta",
		"id":         w.id,
		"model":      model,
		"provider":   provider,
		"cwd":        cwd,
		"created_at": time.Now().Format(time.RFC3339),
	}
	data, _ := json.Marshal(meta)
	data = append(data, '\n')

	if _, err := w.f.Write(data); err != nil {
		return err
	}
	return w.f.Sync()
}

// Close closes the WAL file.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}

// Remove deletes the WAL file (called after successful Save).
func (w *WAL) Remove() error {
	w.Close()
	return os.Remove(w.path)
}

// RecoverFromWAL rebuilds a session from a WAL file if one exists.
// Returns nil if no WAL exists.
func RecoverFromWAL(sessionID string) (*Session, error) {
	path := filepath.Join(sessionsDir(), sessionID+".wal")
	f, err := os.Open(path)
	if err != nil {
		return nil, nil // no WAL, nothing to recover
	}
	defer f.Close()

	var s Session
	s.ID = sessionID
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Check if it's metadata
		var raw map[string]interface{}
		if json.Unmarshal(line, &raw) == nil {
			if raw["type"] == "session_meta" {
				if v, ok := raw["model"].(string); ok {
					s.Model = v
				}
				if v, ok := raw["provider"].(string); ok {
					s.Provider = v
				}
				if v, ok := raw["cwd"].(string); ok {
					s.CWD = v
				}
				if v, ok := raw["created_at"].(string); ok {
					s.CreatedAt, _ = time.Parse(time.RFC3339, v)
				}
				continue
			}
		}

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue // skip corrupted lines
		}
		s.Messages = append(s.Messages, msg)
	}

	if len(s.Messages) == 0 {
		return nil, nil
	}

	s.UpdatedAt = time.Now()
	return &s, nil
}

// CheckForRecovery looks for any WAL files and offers recovery.
// Returns session IDs that have WAL files.
func CheckForRecovery() []string {
	dir := sessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var ids []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".wal" {
			id := e.Name()[:len(e.Name())-4]
			ids = append(ids, id)
		}
	}
	return ids
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
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer
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
			continue // skip corrupted lines instead of failing
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
// Uses file modification time for sorting to avoid loading all sessions.
func List() ([]Entry, error) {
	dir := sessionsDir()
	entries, err := os.ReadDir(dir)
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

		// Use file info for timestamp (fast, no parsing needed)
		info, err := e.Info()
		if err != nil {
			continue
		}

		// Only load the first user message for preview (don't parse full file)
		preview := loadPreview(filepath.Join(dir, e.Name()))
		out = append(out, Entry{
			ID:        id,
			Preview:   preview,
			UpdatedAt: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// loadPreview reads only enough of a session file to extract the first user message.
func loadPreview(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 4096) // small buffer, just need first few lines
	linesRead := 0

	for scanner.Scan() && linesRead < 10 {
		linesRead++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg Message
		if json.Unmarshal(line, &msg) == nil && msg.Role == "user" && msg.Content != "" {
			preview := msg.Content
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			return preview
		}
	}
	return ""
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

	// Scan files by modification time without loading all sessions
	dir := sessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	type candidate struct {
		id   string
		time time.Time
	}
	var candidates []candidate

	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if ext != ".jsonl" && ext != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		id := e.Name()[:len(e.Name())-len(ext)]
		candidates = append(candidates, candidate{id: id, time: info.ModTime()})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].time.After(candidates[j].time)
	})

	// Check most recent sessions until we find one matching CWD
	for _, c := range candidates {
		s, err := Load(c.id)
		if err != nil {
			continue
		}
		if s.CWD == cwd || s.CWD == "" {
			return s, nil
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
