package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type FileWriteTool struct{}

func (FileWriteTool) Name() string      { return "Write" }
func (FileWriteTool) Aliases() []string { return []string{"file_write"} }
func (FileWriteTool) Description() string {
	return "Create or overwrite a file with the given content."
}
func (FileWriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":      map[string]interface{}{"type": "string", "description": "File path to write"},
			"file_path": map[string]interface{}{"type": "string", "description": "Archive-compatible alias for path"},
			"content":   map[string]interface{}{"type": "string", "description": "File content"},
		},
	}
}

func (FileWriteTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	path := p.Path
	if path == "" {
		path = p.FilePath
	}
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if err := validatePathAllowed(ctx, path); err != nil {
		return "", err
	}
	if tc := GetToolContext(ctx); tc != nil && tc.Protected != nil && tc.Protected.IsProtected(path) {
		return "", fmt.Errorf("path %s is protected (read-only)", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(path, []byte(p.Content), 0o644); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	if autoCommitEnabled(ctx) {
		_ = AutoCommit(path, "Write", "wrote file")
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(p.Content), path), nil
}
