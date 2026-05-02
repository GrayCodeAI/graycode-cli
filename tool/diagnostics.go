package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// DiagnosticsTool runs lint/type/compile diagnostics for a file or project.
type DiagnosticsTool struct{}

func (DiagnosticsTool) Name() string        { return "Diagnostics" }
func (DiagnosticsTool) Aliases() []string    { return []string{"diagnostics", "lint"} }
func (DiagnosticsTool) Description() string {
	return "Get lint/type/compile errors for a file or project"
}

func (DiagnosticsTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File or directory path to diagnose",
			},
			"scope": map[string]interface{}{
				"type":        "string",
				"description": "Scope of diagnostics: 'file' or 'project' (default: file)",
				"enum":        []string{"file", "project"},
			},
		},
		"required": []string{"path"},
	}
}

func (DiagnosticsTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path  string `json:"path"`
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if args.Scope == "" {
		args.Scope = "file"
	}

	ext := filepath.Ext(args.Path)
	switch ext {
	case ".go":
		return runGoDiagnostics(ctx, args.Path, args.Scope)
	case ".py":
		return runPythonDiagnostics(ctx, args.Path)
	case ".js", ".ts", ".jsx", ".tsx":
		return runJSTSDiagnostics(ctx, args.Path, ext)
	default:
		if args.Scope == "project" {
			return runGoDiagnostics(ctx, args.Path, "project")
		}
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}
}

func runGoDiagnostics(ctx context.Context, path, scope string) (string, error) {
	var cmd *exec.Cmd
	if scope == "project" {
		dir := path
		cmd = exec.CommandContext(ctx, "go", "vet", "./...")
		cmd.Dir = dir
	} else {
		dir := filepath.Dir(path)
		cmd = exec.CommandContext(ctx, "go", "vet", "./...")
		cmd.Dir = dir
	}
	output, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(output))

	// Also try go build for compile errors
	var buildCmd *exec.Cmd
	if scope == "project" {
		buildCmd = exec.CommandContext(ctx, "go", "build", "./...")
		buildCmd.Dir = path
	} else {
		buildCmd = exec.CommandContext(ctx, "go", "build", path)
		buildCmd.Dir = filepath.Dir(path)
	}
	buildOutput, buildErr := buildCmd.CombinedOutput()
	buildResult := strings.TrimSpace(string(buildOutput))

	var parts []string
	if result != "" {
		parts = append(parts, result)
	}
	if buildResult != "" && buildResult != result {
		parts = append(parts, buildResult)
	}

	if len(parts) == 0 {
		if err != nil || buildErr != nil {
			return "Diagnostics completed with warnings (no output captured).", nil
		}
		return "No issues found.", nil
	}
	return strings.Join(parts, "\n"), nil
}

func runPythonDiagnostics(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "python3", "-m", "py_compile", path)
	output, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(output))
	if err != nil && result == "" {
		result = err.Error()
	}
	if result == "" {
		return "No issues found.", nil
	}
	return result, nil
}

func runJSTSDiagnostics(ctx context.Context, path, ext string) (string, error) {
	// Try eslint first
	cmd := exec.CommandContext(ctx, "npx", "eslint", path, "--format", "compact")
	output, _ := cmd.CombinedOutput()
	result := strings.TrimSpace(string(output))

	// For TypeScript, also try tsc
	if ext == ".ts" || ext == ".tsx" {
		tscCmd := exec.CommandContext(ctx, "npx", "tsc", "--noEmit", path)
		tscOutput, _ := tscCmd.CombinedOutput()
		tscResult := strings.TrimSpace(string(tscOutput))
		if tscResult != "" {
			if result != "" {
				result += "\n" + tscResult
			} else {
				result = tscResult
			}
		}
	}

	if result == "" {
		return "No issues found.", nil
	}
	return result, nil
}
