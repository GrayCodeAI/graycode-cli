package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Memory stores extracted memories from sessions.
type Memory struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source,omitempty"` // session ID or file
}

func memoryDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "memories")
}

// Save persists a memory.
func Save(m *Memory) error {
	if m.ID == "" {
		m.ID = fmt.Sprintf("mem_%d", time.Now().Unix())
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if err := os.MkdirAll(memoryDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(memoryDir(), m.ID+".json"), data, 0o644)
}

// Load reads a memory by ID.
func Load(id string) (*Memory, error) {
	data, err := os.ReadFile(filepath.Join(memoryDir(), id+".json"))
	if err != nil {
		return nil, err
	}
	var m Memory
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// List returns all memories.
func List() ([]*Memory, error) {
	entries, err := os.ReadDir(memoryDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Memory
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5]
		m, err := Load(id)
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// Search finds memories matching a query.
func Search(query string) ([]*Memory, error) {
	memories, err := List()
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(query)
	var out []*Memory
	for _, m := range memories {
		if strings.Contains(strings.ToLower(m.Content), query) {
			out = append(out, m)
			continue
		}
		for _, tag := range m.Tags {
			if strings.Contains(strings.ToLower(tag), query) {
				out = append(out, m)
				break
			}
		}
	}
	return out, nil
}

// ExtractFromSession extracts memories from a session transcript.
func ExtractFromSession(sessionID string, messages []string) []*Memory {
	var memories []*Memory
	for _, msg := range messages {
		if isMemoryWorthy(msg) {
			memories = append(memories, &Memory{
				Content:   msg,
				Source:    sessionID,
				CreatedAt: time.Now(),
			})
		}
	}
	return memories
}

// isMemoryWorthy checks if a message is worth remembering.
func isMemoryWorthy(msg string) bool {
	// Simple heuristics for memory extraction
	indicators := []string{
		"important", "remember", "note", "key", "critical",
		"decision", "agreed", "conclusion", "summary",
	}
	lower := strings.ToLower(msg)
	for _, ind := range indicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	return false
}

// Consolidate merges similar memories.
func Consolidate(memories []*Memory) []*Memory {
	// Simple deduplication by content similarity
	seen := make(map[string]bool)
	var out []*Memory
	for _, m := range memories {
		key := strings.ToLower(strings.TrimSpace(m.Content))
		if len(key) > 50 {
			key = key[:50]
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, m)
		}
	}
	return out
}
