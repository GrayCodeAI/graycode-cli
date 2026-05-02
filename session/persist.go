package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SaveMessages serializes a slice of conversation messages to a JSON file
// atomically. This is a lightweight alternative to full Session persistence
// for callers that only need message-level save/restore.
func SaveMessages(path string, messages []Message) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}

	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}

	// Atomic write: temp file + rename to avoid partial writes.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write session temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename session file: %w", err)
	}
	return nil
}

// LoadMessages deserializes conversation messages from a JSON file.
func LoadMessages(path string) ([]Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no file, empty conversation
		}
		return nil, fmt.Errorf("read session file: %w", err)
	}

	var messages []Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}
	return messages, nil
}

// SessionPath returns the file path for a session within a project directory.
// Sessions are stored under .hawk/sessions/{id}.json.
func SessionPath(projectDir, sessionID string) string {
	return filepath.Join(projectDir, ".hawk", "sessions", sessionID+".json")
}
