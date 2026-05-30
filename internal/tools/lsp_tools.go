package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/lsp"
)

// LSPStatusTool lists configured language servers and their state.
type LSPStatusTool struct {
	Manager *lsp.LSPManager
}

func (t *LSPStatusTool) Name() string        { return "lsp_status" }
func (t *LSPStatusTool) Description() string  { return "List configured LSP servers and their connection state." }
func (t *LSPStatusTool) RiskLevel() string    { return "low" }
func (t *LSPStatusTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}
func (t *LSPStatusTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	status := t.Manager.Status()
	var lines []string
	for lang, state := range status {
		lines = append(lines, fmt.Sprintf("  %s: %s", lang, state))
	}
	if len(lines) == 0 {
		return "No LSP servers configured.", nil
	}
	return "LSP Servers:\n" + strings.Join(lines, "\n"), nil
}

// LSPDiagnosticsTool gets diagnostics for a file.
type LSPDiagnosticsTool struct {
	Manager *lsp.LSPManager
}

func (t *LSPDiagnosticsTool) Name() string        { return "lsp_diagnostics" }
func (t *LSPDiagnosticsTool) Description() string  { return "Get LSP diagnostics (errors, warnings) for a file." }
func (t *LSPDiagnosticsTool) RiskLevel() string    { return "low" }
func (t *LSPDiagnosticsTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path to check diagnostics for",
			},
		},
		"required": []string{"path"},
	}
}
func (t *LSPDiagnosticsTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(args.Path)
	if err != nil {
		return "", err
	}
	uri := "file://" + absPath
	lang, _, ok := t.Manager.Config().ServerForFile(absPath)
	if !ok {
		return fmt.Sprintf("No LSP server configured for %s", filepath.Ext(absPath)), nil
	}
	var diagnostics []lsp.Diagnostic
	err = t.Manager.Execute(ctx, lang, true, func(c *lsp.LSPClient) error {
		var derr error
		diagnostics, derr = c.Diagnostics(ctx, uri)
		return derr
	})
	if err != nil {
		return "", err
	}
	if len(diagnostics) == 0 {
		return "No diagnostics found.", nil
	}
	var lines []string
	for _, d := range diagnostics {
		sev := severityName(d.Severity)
		code := ""
		if d.Code != "" {
			code = " (" + d.Code + ")"
		}
		lines = append(lines, fmt.Sprintf("  Line %d: %s%s — %s", d.Range.Start.Line+1, sev, code, d.Message))
	}
	return fmt.Sprintf("Diagnostics for %s:\n%s", filepath.Base(absPath), strings.Join(lines, "\n")), nil
}

// LSPGotoDefinitionTool navigates to the definition of a symbol.
type LSPGotoDefinitionTool struct {
	Manager *lsp.LSPManager
}

func (t *LSPGotoDefinitionTool) Name() string        { return "lsp_goto_definition" }
func (t *LSPGotoDefinitionTool) Description() string  { return "Go to the definition of a symbol at a position in a file." }
func (t *LSPGotoDefinitionTool) RiskLevel() string    { return "low" }
func (t *LSPGotoDefinitionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{"type": "string", "description": "File path"},
			"line": map[string]interface{}{"type": "integer", "description": "Line number (1-based)"},
			"character": map[string]interface{}{"type": "integer", "description": "Column number (0-based)"},
		},
		"required": []string{"path", "line", "character"},
	}
}
func (t *LSPGotoDefinitionTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	absPath, _ := filepath.Abs(args.Path)
	uri := "file://" + absPath
	lang, _, ok := t.Manager.Config().ServerForFile(absPath)
	if !ok {
		return fmt.Sprintf("No LSP server configured for %s", filepath.Ext(absPath)), nil
	}
	var locations []lsp.Location
	err := t.Manager.Execute(ctx, lang, true, func(c *lsp.LSPClient) error {
		var derr error
		locations, derr = c.GotoDefinition(ctx, uri, args.Line-1, args.Character)
		return derr
	})
	if err != nil {
		return "", err
	}
	if len(locations) == 0 {
		return "No definition found.", nil
	}
	var lines []string
	for _, loc := range locations {
		lines = append(lines, fmt.Sprintf("  %s:%d:%d", strings.TrimPrefix(loc.URI, "file://"), loc.Range.Start.Line+1, loc.Range.Start.Character))
	}
	return "Definitions:\n" + strings.Join(lines, "\n"), nil
}

// LSPFindReferencesTool finds all references to a symbol.
type LSPFindReferencesTool struct {
	Manager *lsp.LSPManager
}

func (t *LSPFindReferencesTool) Name() string        { return "lsp_find_references" }
func (t *LSPFindReferencesTool) Description() string  { return "Find all references to a symbol at a position in a file." }
func (t *LSPFindReferencesTool) RiskLevel() string    { return "low" }
func (t *LSPFindReferencesTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{"type": "string", "description": "File path"},
			"line": map[string]interface{}{"type": "integer", "description": "Line number (1-based)"},
			"character": map[string]interface{}{"type": "integer", "description": "Column number (0-based)"},
		},
		"required": []string{"path", "line", "character"},
	}
}
func (t *LSPFindReferencesTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	absPath, _ := filepath.Abs(args.Path)
	uri := "file://" + absPath
	lang, _, ok := t.Manager.Config().ServerForFile(absPath)
	if !ok {
		return fmt.Sprintf("No LSP server configured for %s", filepath.Ext(absPath)), nil
	}
	var locations []lsp.Location
	err := t.Manager.Execute(ctx, lang, true, func(c *lsp.LSPClient) error {
		var derr error
		locations, derr = c.FindReferences(ctx, uri, args.Line-1, args.Character)
		return derr
	})
	if err != nil {
		return "", err
	}
	if len(locations) == 0 {
		return "No references found.", nil
	}
	var lines []string
	for _, loc := range locations {
		lines = append(lines, fmt.Sprintf("  %s:%d:%d", strings.TrimPrefix(loc.URI, "file://"), loc.Range.Start.Line+1, loc.Range.Start.Character))
	}
	return fmt.Sprintf("References (%d):\n%s", len(locations), strings.Join(lines, "\n")), nil
}

// LSPSymbolsTool lists all symbols in a document.
type LSPSymbolsTool struct {
	Manager *lsp.LSPManager
}

func (t *LSPSymbolsTool) Name() string        { return "lsp_symbols" }
func (t *LSPSymbolsTool) Description() string  { return "List all symbols (functions, types, variables) in a file." }
func (t *LSPSymbolsTool) RiskLevel() string    { return "low" }
func (t *LSPSymbolsTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{"type": "string", "description": "File path"},
		},
		"required": []string{"path"},
	}
}
func (t *LSPSymbolsTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	absPath, _ := filepath.Abs(args.Path)
	uri := "file://" + absPath
	lang, _, ok := t.Manager.Config().ServerForFile(absPath)
	if !ok {
		return fmt.Sprintf("No LSP server configured for %s", filepath.Ext(absPath)), nil
	}
	var symbols []lsp.SymbolInformation
	err := t.Manager.Execute(ctx, lang, true, func(c *lsp.LSPClient) error {
		var derr error
		symbols, derr = c.DocumentSymbol(ctx, uri)
		return derr
	})
	if err != nil {
		return "", err
	}
	if len(symbols) == 0 {
		return "No symbols found.", nil
	}
	var lines []string
	for _, s := range symbols {
		kind := symbolKindName(s.Kind)
		lines = append(lines, fmt.Sprintf("  %s %s (line %d)", kind, s.Name, s.Location.Range.Start.Line+1))
	}
	return fmt.Sprintf("Symbols in %s:\n%s", filepath.Base(absPath), strings.Join(lines, "\n")), nil
}

// LSPPrepareRenameTool checks if a symbol can be renamed.
type LSPPrepareRenameTool struct {
	Manager *lsp.LSPManager
}

func (t *LSPPrepareRenameTool) Name() string        { return "lsp_prepare_rename" }
func (t *LSPPrepareRenameTool) Description() string  { return "Check if a symbol at a position can be renamed." }
func (t *LSPPrepareRenameTool) RiskLevel() string    { return "low" }
func (t *LSPPrepareRenameTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{"type": "string", "description": "File path"},
			"line": map[string]interface{}{"type": "integer", "description": "Line number (1-based)"},
			"character": map[string]interface{}{"type": "integer", "description": "Column number (0-based)"},
		},
		"required": []string{"path", "line", "character"},
	}
}
func (t *LSPPrepareRenameTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	absPath, _ := filepath.Abs(args.Path)
	uri := "file://" + absPath
	lang, _, ok := t.Manager.Config().ServerForFile(absPath)
	if !ok {
		return fmt.Sprintf("No LSP server configured for %s", filepath.Ext(absPath)), nil
	}
	var rng *lsp.Range
	err := t.Manager.Execute(ctx, lang, true, func(c *lsp.LSPClient) error {
		var derr error
		rng, derr = c.PrepareRename(ctx, uri, args.Line-1, args.Character)
		return derr
	})
	if err != nil {
		return "", err
	}
	if rng == nil {
		return "Symbol cannot be renamed at this position.", nil
	}
	return fmt.Sprintf("Symbol is renamable at line %d, columns %d-%d.", rng.Start.Line+1, rng.Start.Character, rng.End.Character), nil
}

// LSPRenameTool renames a symbol across the workspace.
type LSPRenameTool struct {
	Manager *lsp.LSPManager
}

func (t *LSPRenameTool) Name() string        { return "lsp_rename" }
func (t *LSPRenameTool) Description() string  { return "Rename a symbol at a position across the workspace." }
func (t *LSPRenameTool) RiskLevel() string    { return "medium" }
func (t *LSPRenameTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{"type": "string", "description": "File path"},
			"line": map[string]interface{}{"type": "integer", "description": "Line number (1-based)"},
			"character": map[string]interface{}{"type": "integer", "description": "Column number (0-based)"},
			"new_name": map[string]interface{}{"type": "string", "description": "New name for the symbol"},
		},
		"required": []string{"path", "line", "character", "new_name"},
	}
}
func (t *LSPRenameTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
		NewName   string `json:"new_name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	absPath, _ := filepath.Abs(args.Path)
	uri := "file://" + absPath
	lang, _, ok := t.Manager.Config().ServerForFile(absPath)
	if !ok {
		return fmt.Sprintf("No LSP server configured for %s", filepath.Ext(absPath)), nil
	}
	var edit *lsp.WorkspaceEdit
	err := t.Manager.Execute(ctx, lang, false, func(c *lsp.LSPClient) error {
		var derr error
		edit, derr = c.Rename(ctx, uri, args.Line-1, args.Character, args.NewName)
		return derr
	})
	if err != nil {
		return "", err
	}
	if edit == nil || len(edit.Changes) == 0 {
		return "No changes needed.", nil
	}
	totalEdits := 0
	for _, edits := range edit.Changes {
		totalEdits += len(edits)
	}
	return fmt.Sprintf("Rename to %q: %d edits across %d files.", args.NewName, totalEdits, len(edit.Changes)), nil
}

// RegisterLSPTools adds all 7 LSP tools to the provided slice.
func RegisterLSPTools(tools []interface{ Name() string }, manager *lsp.LSPManager) []interface{ Name() string } {
	return append(tools,
		&LSPStatusTool{Manager: manager},
		&LSPDiagnosticsTool{Manager: manager},
		&LSPGotoDefinitionTool{Manager: manager},
		&LSPFindReferencesTool{Manager: manager},
		&LSPSymbolsTool{Manager: manager},
		&LSPPrepareRenameTool{Manager: manager},
		&LSPRenameTool{Manager: manager},
	)
}

// Helpers

func severityName(sev int) string {
	switch lsp.DiagnosticSeverity(sev) {
	case lsp.SeverityError:
		return "error"
	case lsp.SeverityWarning:
		return "warning"
	case lsp.SeverityInformation:
		return "info"
	case lsp.SeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

func symbolKindName(kind int) string {
	kinds := map[int]string{
		1: "File", 2: "Module", 3: "Namespace", 4: "Package", 5: "Class",
		6: "Method", 7: "Property", 8: "Field", 9: "Constructor", 10: "Enum",
		11: "Interface", 12: "Function", 13: "Variable", 14: "Constant",
		15: "String", 16: "Number", 17: "Boolean", 18: "Array", 19: "Object",
		20: "Key", 21: "Null", 22: "EnumMember", 23: "Struct", 24: "Event",
		25: "Operator", 26: "TypeParameter",
	}
	if name, ok := kinds[kind]; ok {
		return name
	}
	return "Symbol"
}
