package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/toolset"
)

// ToolsetTool lists and resolves named, composable tool groups (research,
// dev, ops, full_stack). It lets a user/agent scope the tool surface instead
// of always advertising every tool — adopted from Hermes Agent's toolset
// system.
type ToolsetTool struct{}

func (ToolsetTool) Name() string      { return "Toolset" }
func (ToolsetTool) RiskLevel() string { return "low" }
func (ToolsetTool) Aliases() []string { return []string{"toolset"} }
func (ToolsetTool) Description() string {
	return "List available toolsets or resolve one to its concrete tool list. Toolsets are named, composable groups (research, dev, ops, full_stack); resolving expands required toolsets transitively."
}

func (ToolsetTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"list", "resolve"},
				"description": "list: show available toolsets; resolve: expand a toolset to its tools.",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Toolset name to resolve (action=resolve).",
			},
		},
		"required": []string{"action"},
	}
}

func (ToolsetTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	reg, err := toolset.NewRegistry(toolset.Defaults())
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(p.Action)) {
	case "list":
		return "Available toolsets: " + strings.Join(reg.Names(), ", "), nil
	case "resolve":
		tools, err := reg.Resolve(strings.TrimSpace(p.Name))
		if err != nil {
			return "", err
		}
		payload := map[string]interface{}{
			"toolset": strings.TrimSpace(p.Name),
			"tools":   tools,
			"count":   len(tools),
		}
		out, _ := json.MarshalIndent(payload, "", "  ")
		return string(out), nil
	default:
		return "", fmt.Errorf("unsupported action %q (use list or resolve)", p.Action)
	}
}
