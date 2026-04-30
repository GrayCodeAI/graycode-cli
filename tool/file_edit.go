package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type FileEditTool struct{}

func (FileEditTool) Name() string      { return "Edit" }
func (FileEditTool) Aliases() []string { return []string{"file_edit"} }
func (FileEditTool) Description() string {
	return "Edit a file by replacing an exact string match with new content."
}
func (FileEditTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":       map[string]interface{}{"type": "string", "description": "File path to edit"},
			"file_path":  map[string]interface{}{"type": "string", "description": "Archive-compatible alias for path"},
			"old_str":    map[string]interface{}{"type": "string", "description": "Exact string to find and replace"},
			"old_string": map[string]interface{}{"type": "string", "description": "Archive-compatible alias for old_str"},
			"new_str":    map[string]interface{}{"type": "string", "description": "Replacement string"},
			"new_string": map[string]interface{}{"type": "string", "description": "Archive-compatible alias for new_str"},
		},
	}
}

func (FileEditTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path      string  `json:"path"`
		FilePath  string  `json:"file_path"`
		OldStr    string  `json:"old_str"`
		OldString string  `json:"old_string"`
		NewStr    *string `json:"new_str"`
		NewString *string `json:"new_string"`
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
	oldStr := p.OldStr
	if oldStr == "" {
		oldStr = p.OldString
	}
	newStr := ""
	if p.NewStr != nil {
		newStr = *p.NewStr
	} else if p.NewString != nil {
		newStr = *p.NewString
	}

	info, err := os.Stat(path)
	if err != nil {
		suggestion := suggestSimilar(path)
		if suggestion != "" {
			return "", fmt.Errorf("file not found: %s\nDid you mean: %s", path, suggestion)
		}
		return "", fmt.Errorf("file not found: %s", path)
	}
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file too large: %d bytes", info.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	content := string(data)

	// Detect line endings
	lineEnding := "\n"
	if strings.Contains(content, "\r\n") {
		lineEnding = "\r\n"
	}

	count := strings.Count(content, oldStr)
	if count == 0 {
		return "", fmt.Errorf("old_str not found in %s", path)
	}
	if count > 1 {
		return "", fmt.Errorf("old_str found %d times in %s — must be unique", count, path)
	}
	result := strings.Replace(content, oldStr, newStr, 1)

	// Preserve line endings
	if lineEnding == "\r\n" {
		result = strings.ReplaceAll(result, "\r\n", "\n")
		result = strings.ReplaceAll(result, "\n", "\r\n")
	}

	if err := os.WriteFile(path, []byte(result), info.Mode()); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	return fmt.Sprintf("Edited %s (replaced 1 occurrence)", path), nil
}
