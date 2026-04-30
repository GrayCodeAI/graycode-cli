package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type FileWriteTool struct{}

func (FileWriteTool) Name() string        { return "file_write" }
func (FileWriteTool) Description() string { return "Create or overwrite a file with the given content." }
func (FileWriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":    map[string]interface{}{"type": "string", "description": "File path to write"},
			"content": map[string]interface{}{"type": "string", "description": "File content"},
		},
		"required": []string{"path", "content"},
	}
}

func (FileWriteTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p.Path), 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(p.Path, []byte(p.Content), 0o644); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(p.Content), p.Path), nil
}
