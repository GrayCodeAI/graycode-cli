package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// MultiEditTool applies multiple edits to a single file in one call.
type MultiEditTool struct{}

func (MultiEditTool) Name() string      { return "MultiEdit" }
func (MultiEditTool) RiskLevel() string  { return "medium" }
func (MultiEditTool) Aliases() []string  { return []string{"multi_edit", "multi_file_edit"} }
func (MultiEditTool) Description() string {
	return "Apply multiple edits to a single file in one call. Each edit replaces an exact string match. Edits are applied sequentially."
}
func (MultiEditTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_path": map[string]interface{}{"type": "string", "description": "File path to edit"},
			"edits": map[string]interface{}{
				"type": "array",
				"description": "Array of edit operations",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"old_string":  map[string]interface{}{"type": "string", "description": "Exact string to find"},
						"new_string":  map[string]interface{}{"type": "string", "description": "Replacement string"},
						"replace_all": map[string]interface{}{"type": "boolean", "description": "Replace all occurrences (default: first only)"},
					},
				},
			},
		},
	}
}

type multiEditParams struct {
	FilePath string `json:"file_path"`
	Edits    []struct {
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all,omitempty"`
	} `json:"edits"`
}

func (MultiEditTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p multiEditParams
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}
	if len(p.Edits) == 0 {
		return "", fmt.Errorf("at least one edit is required")
	}
	if err := validatePathAllowed(ctx, p.FilePath); err != nil {
		return "", err
	}
	if reason := IsSensitivePath(p.FilePath); reason != "" {
		return "", fmt.Errorf("blocked: %s", reason)
	}
	if tc := GetToolContext(ctx); tc != nil && tc.Protected != nil && tc.Protected.IsProtected(p.FilePath) {
		return "", fmt.Errorf("path %s is protected (read-only)", p.FilePath)
	}

	data, err := os.ReadFile(p.FilePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	BackupFile(p.FilePath)
	content := string(data)

	applied, failed := 0, 0
	for i, edit := range p.Edits {
		if edit.OldString == "" {
			failed++
			continue
		}
		if !strings.Contains(content, edit.OldString) {
			failed++
			continue
		}
		if edit.ReplaceAll {
			content = strings.ReplaceAll(content, edit.OldString, edit.NewString)
		} else {
			content = strings.Replace(content, edit.OldString, edit.NewString, 1)
		}
		applied++
		_ = i
	}

	if applied == 0 {
		return fmt.Sprintf("No edits applied (%d failed — old_string not found in file).", failed), nil
	}

	if err := os.WriteFile(p.FilePath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fmt.Sprintf("Applied %d/%d edit(s) to %s.", applied, applied+failed, p.FilePath), nil
}
