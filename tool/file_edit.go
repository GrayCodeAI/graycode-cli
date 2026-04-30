package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type FileEditTool struct{}

func (FileEditTool) Name() string        { return "file_edit" }
func (FileEditTool) Description() string { return "Edit a file by replacing an exact string match with new content." }
func (FileEditTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":    map[string]interface{}{"type": "string", "description": "File path to edit"},
			"old_str": map[string]interface{}{"type": "string", "description": "Exact string to find and replace"},
			"new_str": map[string]interface{}{"type": "string", "description": "Replacement string"},
		},
		"required": []string{"path", "old_str", "new_str"},
	}
}

func (FileEditTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path   string `json:"path"`
		OldStr string `json:"old_str"`
		NewStr string `json:"new_str"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", p.Path, err)
	}
	content := string(data)
	count := strings.Count(content, p.OldStr)
	if count == 0 {
		return "", fmt.Errorf("old_str not found in %s", p.Path)
	}
	if count > 1 {
		return "", fmt.Errorf("old_str found %d times in %s — must be unique", count, p.Path)
	}
	result := strings.Replace(content, p.OldStr, p.NewStr, 1)
	if err := os.WriteFile(p.Path, []byte(result), 0o644); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	return fmt.Sprintf("Edited %s", p.Path), nil
}
