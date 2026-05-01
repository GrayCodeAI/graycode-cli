package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
)

// NotebookEditTool edits Jupyter notebook cells.
type NotebookEditTool struct{}

func (NotebookEditTool) Name() string      { return "NotebookEdit" }
func (NotebookEditTool) Aliases() []string { return []string{"notebook_edit"} }
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

func (ConfigTool) Name() string      { return "Config" }
func (ConfigTool) Aliases() []string { return []string{"config"} }
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
		value, ok := hawkconfig.SettingValue(hawkconfig.LoadSettings(), p.Key)
		if !ok {
			return "", fmt.Errorf("unsupported setting key: %s", p.Key)
		}
		return fmt.Sprintf("%s=%s", p.Key, value), nil
	case "set":
		if err := hawkconfig.SetGlobalSetting(p.Key, p.Value); err != nil {
			return "", err
		}
		return fmt.Sprintf("Set %q in global settings", p.Key), nil
	default:
		return "", fmt.Errorf("unknown action: %s", p.Action)
	}
}

// BriefTool (SendUserMessage) sends a message the user will read.
// Text outside this tool is visible in the detail view; the answer lives here.
type BriefTool struct{}

func (BriefTool) Name() string      { return "SendUserMessage" }
func (BriefTool) Aliases() []string { return []string{"brief", "Brief"} }
func (BriefTool) Description() string {
	return "Send a message to the user. Supports markdown. Use 'proactive' status when surfacing something the user hasn't asked for."
}
func (BriefTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "The message for the user. Supports markdown formatting.",
			},
			"attachments": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Optional file paths to attach (images, diffs, logs)",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"normal", "proactive"},
				"description": "Use 'proactive' when surfacing something the user hasn't asked for",
			},
		},
		"required": []string{"message"},
	}
}

func (BriefTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Message     string   `json:"message"`
		Attachments []string `json:"attachments"`
		Status      string   `json:"status"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Message == "" {
		return "", fmt.Errorf("message is required")
	}
	return p.Message, nil
}
