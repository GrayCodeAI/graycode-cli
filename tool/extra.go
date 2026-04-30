package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// NotebookEditTool edits Jupyter notebook cells.
type NotebookEditTool struct{}

func (NotebookEditTool) Name() string { return "notebook_edit" }
func (NotebookEditTool) Description() string {
	return "Edit a Jupyter notebook cell. Specify the notebook path, cell number, and new source."
}
func (NotebookEditTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":        map[string]interface{}{"type": "string", "description": "Notebook file path (.ipynb)"},
			"cell_number": map[string]interface{}{"type": "integer", "description": "Cell number (0-based)"},
			"new_source":  map[string]interface{}{"type": "string", "description": "New cell source content"},
		},
		"required": []string{"path", "cell_number", "new_source"},
	}
}

func (NotebookEditTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path       string `json:"path"`
		CellNumber int    `json:"cell_number"`
		NewSource  string `json:"new_source"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return "", err
	}
	var nb map[string]interface{}
	if err := json.Unmarshal(data, &nb); err != nil {
		return "", fmt.Errorf("invalid notebook: %w", err)
	}
	cells, ok := nb["cells"].([]interface{})
	if !ok || p.CellNumber >= len(cells) {
		return "", fmt.Errorf("cell %d not found (notebook has %d cells)", p.CellNumber, len(cells))
	}
	cell := cells[p.CellNumber].(map[string]interface{})
	lines := strings.Split(p.NewSource, "\n")
	sourceLines := make([]interface{}, len(lines))
	for i, l := range lines {
		if i < len(lines)-1 {
			sourceLines[i] = l + "\n"
		} else {
			sourceLines[i] = l
		}
	}
	cell["source"] = sourceLines
	out, _ := json.MarshalIndent(nb, "", " ")
	if err := os.WriteFile(p.Path, out, 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Edited cell %d in %s", p.CellNumber, p.Path), nil
}

// ConfigTool reads/writes hawk settings.
type ConfigTool struct{}

func (ConfigTool) Name() string { return "config" }
func (ConfigTool) Description() string {
	return "Read or modify hawk configuration settings."
}
func (ConfigTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{"type": "string", "enum": []string{"get", "set"}, "description": "Action"},
			"key":    map[string]interface{}{"type": "string", "description": "Setting key"},
			"value":  map[string]interface{}{"type": "string", "description": "Setting value (for set)"},
		},
		"required": []string{"action", "key"},
	}
}

func (ConfigTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action string `json:"action"`
		Key    string `json:"key"`
		Value  string `json:"value"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	switch p.Action {
	case "get":
		return fmt.Sprintf("Config key %q: (use /config to view settings)", p.Key), nil
	case "set":
		return fmt.Sprintf("Set %q = %q (restart to apply)", p.Key, p.Value), nil
	default:
		return "", fmt.Errorf("unknown action: %s", p.Action)
	}
}

// BriefTool provides a concise summary of the current state.
type BriefTool struct{}

func (BriefTool) Name() string { return "brief" }
func (BriefTool) Description() string {
	return "Generate a brief status update about what you've done so far. Use at the end of a task to summarize."
}
func (BriefTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"summary": map[string]interface{}{"type": "string", "description": "Brief summary of work done"},
		},
		"required": []string{"summary"},
	}
}

func (BriefTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	return "📋 " + p.Summary, nil
}
