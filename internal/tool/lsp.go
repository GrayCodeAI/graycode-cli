package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/codegraph"
)

type LSPTool struct{}

func (LSPTool) Name() string      { return "LSP" }
func (LSPTool) Aliases() []string { return []string{"lsp"} }
func (LSPTool) Description() string {
	return "Get code intelligence: diagnostics, definitions, references. Uses codegraph for go-to-definition and find-references."
}

func (LSPTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{"type": "string", "enum": []string{"diagnostics", "definition", "references", "implementations"}, "description": "LSP action"},
			"path":   map[string]interface{}{"type": "string", "description": "File path"},
			"line":   map[string]interface{}{"type": "integer", "description": "Line number (1-based)"},
			"column": map[string]interface{}{"type": "integer", "description": "Column number (1-based)"},
			"symbol": map[string]interface{}{"type": "string", "description": "Symbol name to look up (alternative to line/column)"},
			"root":   map[string]interface{}{"type": "string", "description": "Project root directory (default: current dir)"},
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
		Symbol string `json:"symbol"`
		Root   string `json:"root"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	root := p.Root
	if root == "" {
		root = "."
	}

	switch p.Action {
	case "diagnostics":
		return lspDiagnostics(ctx, p.Path)
	case "definition":
		return lspDefinition(root, p.Path, p.Line, p.Symbol)
	case "references":
		return lspReferences(root, p.Path, p.Line, p.Symbol)
	case "implementations":
		return lspImplementations(root, p.Symbol)
	default:
		return "", fmt.Errorf("unknown LSP action: %s", p.Action)
	}
}

// lspDefinition finds where a symbol is defined using codegraph.
func lspDefinition(root, filePath string, line int, symbol string) (string, error) {
	cg, err := codegraph.Open(root)
	if err != nil {
		return "", fmt.Errorf("codegraph not available: %w", err)
	}
	defer cg.Close()

	// If symbol is provided directly, search for it
	if symbol != "" {
		nodes, err := cg.Search(symbol, 10)
		if err != nil || len(nodes) == 0 {
			return fmt.Sprintf("No definition found for %q", symbol), nil
		}

		var result strings.Builder
		result.WriteString(fmt.Sprintf("## Definition: %s\n\n", symbol))
		for _, n := range nodes {
			sig := n.QualifiedName
			if n.Signature != "" {
				sig = n.Signature
			}
			result.WriteString(fmt.Sprintf("- **%s** `%s`\n  in %s:%d\n", n.Kind, sig, n.FilePath, n.StartLine))
		}
		return result.String(), nil
	}

	// If line is provided, try to extract symbol from that line
	if line > 0 {
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			absPath = filePath
		}

		source, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Sprintf("Cannot read file: %s", filePath), nil
		}

		lines := strings.Split(string(source), "\n")
		if line <= len(lines) {
			symbolFromLine := extractSymbolFromLine(lines[line-1])
			if symbolFromLine != "" {
				nodes, err := cg.Search(symbolFromLine, 10)
				if err != nil || len(nodes) == 0 {
					return fmt.Sprintf("No definition found for %q (from line %d)", symbolFromLine, line), nil
				}

				var result strings.Builder
				result.WriteString(fmt.Sprintf("## Definition: %s (from %s:%d)\n\n", symbolFromLine, filePath, line))
				for _, n := range nodes {
					sig := n.QualifiedName
					if n.Signature != "" {
						sig = n.Signature
					}
					result.WriteString(fmt.Sprintf("- **%s** `%s`\n  in %s:%d\n", n.Kind, sig, n.FilePath, n.StartLine))
				}
				return result.String(), nil
			}
		}
	}

	return "Provide 'symbol' or 'line' to find definition", nil
}

// lspReferences finds all references to a symbol using codegraph.
func lspReferences(root, filePath string, line int, symbol string) (string, error) {
	cg, err := codegraph.Open(root)
	if err != nil {
		return "", fmt.Errorf("codegraph not available: %w", err)
	}
	defer cg.Close()

	// Get symbol name
	sym := symbol
	if sym == "" && line > 0 {
		absPath, absErr := filepath.Abs(filePath)
		if absErr != nil {
			absPath = filePath
		}
		source, readErr := os.ReadFile(absPath)
		if readErr == nil {
			lines := strings.Split(string(source), "\n")
			if line <= len(lines) {
				sym = extractSymbolFromLine(lines[line-1])
			}
		}
	}

	if sym == "" {
		return "Provide 'symbol' or 'line' to find references", nil
	}

	// Search for the symbol
	nodes, err := cg.Search(sym, 5)
	if err != nil || len(nodes) == 0 {
		return fmt.Sprintf("No references found for %q", sym), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("## References: %s\n\n", sym))

	totalRefs := 0
	for _, n := range nodes {
		// Get callers of this symbol
		callers, err := cg.GetCallers(n.ID, 1)
		if err != nil {
			continue
		}

		if len(callers) > 0 {
			result.WriteString(fmt.Sprintf("### %s `%s` in %s:%d\n", n.Kind, n.Name, n.FilePath, n.StartLine))
			result.WriteString(fmt.Sprintf("Called by %d:\n", len(callers)))
			for _, c := range callers {
				result.WriteString(fmt.Sprintf("- %s `%s` in %s:%d\n", c.Kind, c.Name, c.FilePath, c.StartLine))
				totalRefs++
			}
			result.WriteString("\n")
		}
	}

	if totalRefs == 0 {
		return fmt.Sprintf("No references found for %q", sym), nil
	}

	return result.String(), nil
}

// lspImplementations finds types that implement an interface using codegraph.
func lspImplementations(root, symbol string) (string, error) {
	if symbol == "" {
		return "Provide 'symbol' (interface name) to find implementations", nil
	}

	cg, err := codegraph.Open(root)
	if err != nil {
		return "", fmt.Errorf("codegraph not available: %w", err)
	}
	defer cg.Close()

	// Search for the interface
	nodes, err := cg.Search(symbol, 5)
	if err != nil || len(nodes) == 0 {
		return fmt.Sprintf("No interface found for %q", symbol), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("## Implementations: %s\n\n", symbol))

	for _, n := range nodes {
		if n.Kind != "interface" && n.Kind != "trait" {
			continue
		}

		result.WriteString(fmt.Sprintf("### Interface: `%s` in %s:%d\n\n", n.QualifiedName, n.FilePath, n.StartLine))

		// Find nodes that implement this interface (via 'implements' edges)
		impact, err := cg.GetImpactRadius(n.ID, 2)
		if err != nil {
			continue
		}

		implementors := 0
		for _, imp := range impact {
			if imp.Kind == "struct" || imp.Kind == "class" {
				result.WriteString(fmt.Sprintf("- **%s** `%s` in %s:%d\n", imp.Kind, imp.Name, imp.FilePath, imp.StartLine))
				implementors++
			}
		}

		if implementors == 0 {
			result.WriteString("No implementations found.\n")
		}
	}

	return result.String(), nil
}

// extractSymbolFromLine extracts a likely symbol name from a line of code.
func extractSymbolFromLine(line string) string {
	line = strings.TrimSpace(line)

	// Common patterns to extract symbol names
	patterns := []struct {
		prefix string
		suffix string
	}{
		{"func ", "("},
		{"def ", "("},
		{"function ", "("},
		{"class ", "{"},
		{"class ", ":"},
		{"interface ", "{"},
		{"type ", " struct"},
		{"type ", " interface"},
		{"import {", "}"},
		{"from ", " import"},
		{"new ", "("},
		{"", "("}, // generic function call
	}

	for _, p := range patterns {
		if p.prefix != "" && !strings.HasPrefix(line, p.prefix) {
			continue
		}

		text := line
		if p.prefix != "" {
			text = strings.TrimPrefix(text, p.prefix)
		}

		if idx := strings.Index(text, p.suffix); idx > 0 {
			symbol := strings.TrimSpace(text[:idx])
			// Clean up common noise
			symbol = strings.TrimLeft(symbol, "*&") // pointer/receiver
			if strings.Contains(symbol, ".") {
				parts := strings.Split(symbol, ".")
				symbol = parts[len(parts)-1] // last part
			}
			if len(symbol) > 2 && len(symbol) < 100 {
				return symbol
			}
		}
	}

	// Fallback: look for camelCase or snake_case identifiers
	words := strings.FieldsFunc(line, func(r rune) bool {
		return !isAlphaNum(r) && r != '_' && r != '.'
	})
	for _, w := range words {
		if len(w) > 2 && (isCamelCase(w) || isSnakeCase(w)) {
			// Skip common keywords
			lower := strings.ToLower(w)
			if lower != "func" && lower != "return" && lower != "if" && lower != "else" &&
				lower != "for" && lower != "range" && lower != "var" && lower != "const" &&
				lower != "type" && lower != "struct" && lower != "interface" && lower != "package" {
				return w
			}
		}
	}

	return ""
}

func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func isCamelCase(s string) bool {
	hasLower := false
	hasUpper := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			hasLower = true
		}
		if r >= 'A' && r <= 'Z' {
			hasUpper = true
		}
	}
	return hasLower && hasUpper
}

func isSnakeCase(s string) bool {
	return strings.Contains(s, "_") && len(s) > 3
}

func lspDiagnostics(ctx context.Context, path string) (string, error) {
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
