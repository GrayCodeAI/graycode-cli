package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CoreMemoryAppendTool appends content to a memory block identified by label.
type CoreMemoryAppendTool struct{}

func (CoreMemoryAppendTool) Name() string        { return "CoreMemoryAppend" }
func (CoreMemoryAppendTool) Description() string {
	return "Append content to a core memory block. Creates the block if it doesn't exist."
}
func (CoreMemoryAppendTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"label":   map[string]interface{}{"type": "string", "description": "Memory block label (e.g. convention, preference, decision)"},
			"content": map[string]interface{}{"type": "string", "description": "Content to append"},
		},
		"required": []string{"label", "content"},
	}
}

func (CoreMemoryAppendTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Label   string `json:"label"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	tc := GetToolContext(ctx)
	if tc == nil || tc.YaadBridge == nil {
		return "", fmt.Errorf("memory not available")
	}
	if err := tc.YaadBridge.Remember(p.Content, p.Label); err != nil {
		return "", err
	}
	return fmt.Sprintf("Appended to [%s] memory block.", p.Label), nil
}

// CoreMemoryReplaceTool finds and replaces content within a memory block.
type CoreMemoryReplaceTool struct{}

func (CoreMemoryReplaceTool) Name() string        { return "CoreMemoryReplace" }
func (CoreMemoryReplaceTool) Description() string {
	return "Find and replace content within a core memory block identified by label."
}
func (CoreMemoryReplaceTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"label":       map[string]interface{}{"type": "string", "description": "Memory block label"},
			"old_content": map[string]interface{}{"type": "string", "description": "Text to find"},
			"new_content": map[string]interface{}{"type": "string", "description": "Replacement text"},
		},
		"required": []string{"label", "old_content", "new_content"},
	}
}

func (CoreMemoryReplaceTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Label      string `json:"label"`
		OldContent string `json:"old_content"`
		NewContent string `json:"new_content"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	tc := GetToolContext(ctx)
	if tc == nil || tc.YaadBridge == nil {
		return "", fmt.Errorf("memory not available")
	}
	ids, contents, err := tc.YaadBridge.SearchByType(p.Label, 100)
	if err != nil {
		return "", err
	}
	replaced := 0
	for i, content := range contents {
		if strings.Contains(content, p.OldContent) {
			updated := strings.Replace(content, p.OldContent, p.NewContent, 1)
			if err := tc.YaadBridge.UpdateNodeContent(ids[i], updated); err != nil {
				return "", err
			}
			replaced++
		}
	}
	if replaced == 0 {
		return fmt.Sprintf("No [%s] memory block contains %q.", p.Label, p.OldContent), nil
	}
	return fmt.Sprintf("Replaced in %d [%s] memory node(s).", replaced, p.Label), nil
}

// CoreMemoryRethinkTool completely rewrites a memory block.
type CoreMemoryRethinkTool struct{}

func (CoreMemoryRethinkTool) Name() string        { return "CoreMemoryRethink" }
func (CoreMemoryRethinkTool) Description() string {
	return "Completely rewrite a core memory block identified by label. Overwrites the first matching node or creates a new one."
}
func (CoreMemoryRethinkTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"label":     map[string]interface{}{"type": "string", "description": "Memory block label"},
			"new_value": map[string]interface{}{"type": "string", "description": "New content for the memory block"},
		},
		"required": []string{"label", "new_value"},
	}
}

func (CoreMemoryRethinkTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Label    string `json:"label"`
		NewValue string `json:"new_value"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	tc := GetToolContext(ctx)
	if tc == nil || tc.YaadBridge == nil {
		return "", fmt.Errorf("memory not available")
	}
	ids, _, err := tc.YaadBridge.SearchByType(p.Label, 1)
	if err != nil {
		return "", err
	}
	if len(ids) > 0 {
		if err := tc.YaadBridge.UpdateNodeContent(ids[0], p.NewValue); err != nil {
			return "", err
		}
		return fmt.Sprintf("Rewrote [%s] memory block.", p.Label), nil
	}
	if err := tc.YaadBridge.Remember(p.NewValue, p.Label); err != nil {
		return "", err
	}
	return fmt.Sprintf("Created new [%s] memory block.", p.Label), nil
}
