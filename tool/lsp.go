package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type LSPTool struct{}

func (LSPTool) Name() string      { return "LSP" }
func (LSPTool) Aliases() []string { return []string{"lsp"} }
func (LSPTool) Description() string {
	return "Get code intelligence: diagnostics, definitions, references. Uses the project's language server."
}
func (LSPTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{"type": "string", "enum": []string{"diagnostics", "definition", "references"}, "description": "LSP action"},
			"path":   map[string]interface{}{"type": "string", "description": "File path"},
			"line":   map[string]interface{}{"type": "integer", "description": "Line number (1-based)"},
			"column": map[string]interface{}{"type": "integer", "description": "Column number (1-based)"},
		},
		"required": []string{"action", "path"},
	}
}

func (LSPTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action string `json:"action"`
		Path   string `json:"path"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	switch p.Action {
	case "diagnostics":
		return lspDiagnostics(ctx, p.Path)
	case "definition", "references":
		return fmt.Sprintf("LSP %s at %s:%d:%d — not yet implemented (use grep as fallback)", p.Action, p.Path, p.Line, p.Column), nil
	default:
		return "", fmt.Errorf("unknown LSP action: %s", p.Action)
	}
}

func lspDiagnostics(ctx context.Context, path string) (string, error) {
	// Try language-specific linters
	ext := ""
	if i := strings.LastIndex(path, "."); i >= 0 {
		ext = path[i:]
	}

	var cmd *exec.Cmd
	switch ext {
	case ".go":
		cmd = exec.CommandContext(ctx, "go", "vet", "./...")
	case ".ts", ".tsx", ".js", ".jsx":
		cmd = exec.CommandContext(ctx, "npx", "tsc", "--noEmit", "--pretty")
	case ".py":
		cmd = exec.CommandContext(ctx, "python3", "-m", "py_compile", path)
	case ".rs":
		cmd = exec.CommandContext(ctx, "cargo", "check", "--message-format=short")
	default:
		return fmt.Sprintf("No linter configured for %s files", ext), nil
	}

	out, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil && result == "" {
		result = err.Error()
	}
	if result == "" {
		return "No diagnostics found.", nil
	}
	if len(result) > 10000 {
		result = result[:10000] + "\n... (truncated)"
	}
	return result, nil
}
