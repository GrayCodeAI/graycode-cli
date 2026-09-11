package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// SaveMessages serializes a slice of conversation messages to a JSON file
// atomically. This is a lightweight alternative to full Session persistence
// for callers that only need message-level save/restore.
func SaveMessages(path string, messages []Message) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}

	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}

	// Atomic write: temp file → sync → rename to avoid partial writes.
	tmp, err := os.CreateTemp(dir, ".hawk-session-*.tmp")
	if err != nil {
		return fmt.Errorf("create session temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // cleanup if rename fails

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write session temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync session temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close session temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename session file: %w", err)
	}
	return nil
}

// LoadMessages deserializes conversation messages from a JSON file.
func LoadMessages(path string) ([]Message, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path built by SessionPath() from internal project state dir + session ID
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
// It returns an empty path for an invalid session ID. Sessions are stored under
// the user state directory, partitioned by project.
func SessionPath(projectDir, sessionID string) string {
	if !ValidID(sessionID) {
		return ""
	}
	return filepath.Join(storage.ProjectStateDir(projectDir), "sessions", sessionID+".json")
}
