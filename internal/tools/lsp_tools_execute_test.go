package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/lsp"
)

// helper to create a manager with a Go server configured
func newTestManager() *lsp.LSPManager {
	cfg := &lsp.LSPConfig{
		Servers: map[string]lsp.ServerConfig{
			"go": {Command: "gopls", Extensions: []string{".go"}},
		},
	}
	return lsp.NewManager(cfg)
}

// helper to create a manager with a non-existent server command
func newFailingManager() *lsp.LSPManager {
	cfg := &lsp.LSPConfig{
		Servers: map[string]lsp.ServerConfig{
			"go": {Command: "nonexistent-lsp-server-xyz", Extensions: []string{".go"}},
		},
	}
	return lsp.NewManager(cfg)
}

// --- Metadata tests ---

func TestLSPStatusTool_Metadata(t *testing.T) {
	m := newTestManager()
	defer m.Close()
	tool := &LSPStatusTool{Manager: m}

	if tool.Name() != "lsp_status" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "lsp_status")
	}
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
	if tool.RiskLevel() != "low" {
		t.Errorf("RiskLevel() = %q, want %q", tool.RiskLevel(), "low")
	}
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("Parameters() type = %v, want %q", params["type"], "object")
	}
}

func TestLSPDiagnosticsTool_Metadata(t *testing.T) {
	m := newTestManager()
	defer m.Close()
	tool := &LSPDiagnosticsTool{Manager: m}

	if tool.Name() != "lsp_diagnostics" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "lsp_diagnostics")
	}
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
	if tool.RiskLevel() != "low" {
		t.Errorf("RiskLevel() = %q, want %q", tool.RiskLevel(), "low")
	}
	params := tool.Parameters()
	if _, ok := params["properties"]; !ok {
		t.Error("Parameters() should have properties")
	}
}

func TestLSPGotoDefinitionTool_Metadata(t *testing.T) {
	m := newTestManager()
	defer m.Close()
	tool := &LSPGotoDefinitionTool{Manager: m}

	if tool.Name() != "lsp_goto_definition" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "lsp_goto_definition")
	}
	if tool.RiskLevel() != "low" {
		t.Errorf("RiskLevel() = %q, want %q", tool.RiskLevel(), "low")
	}
}

func TestLSPFindReferencesTool_Metadata(t *testing.T) {
	m := newTestManager()
	defer m.Close()
	tool := &LSPFindReferencesTool{Manager: m}

	if tool.Name() != "lsp_find_references" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "lsp_find_references")
	}
	if tool.RiskLevel() != "low" {
		t.Errorf("RiskLevel() = %q, want %q", tool.RiskLevel(), "low")
	}
}

func TestLSPSymbolsTool_Metadata(t *testing.T) {
	m := newTestManager()
	defer m.Close()
	tool := &LSPSymbolsTool{Manager: m}

	if tool.Name() != "lsp_symbols" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "lsp_symbols")
	}
	if tool.RiskLevel() != "low" {
		t.Errorf("RiskLevel() = %q, want %q", tool.RiskLevel(), "low")
	}
}

func TestLSPPrepareRenameTool_Metadata(t *testing.T) {
	m := newTestManager()
	defer m.Close()
	tool := &LSPPrepareRenameTool{Manager: m}

	if tool.Name() != "lsp_prepare_rename" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "lsp_prepare_rename")
	}
	if tool.RiskLevel() != "low" {
		t.Errorf("RiskLevel() = %q, want %q", tool.RiskLevel(), "low")
	}
}

func TestLSPRenameTool_Metadata(t *testing.T) {
	m := newTestManager()
	defer m.Close()
	tool := &LSPRenameTool{Manager: m}

	if tool.Name() != "lsp_rename" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "lsp_rename")
	}
	if tool.RiskLevel() != "medium" {
		t.Errorf("RiskLevel() = %q, want %q", tool.RiskLevel(), "medium")
	}
}

// --- LSPStatusTool Execute tests ---

func TestLSPStatusTool_Execute_EmptyConfig(t *testing.T) {
	cfg := &lsp.LSPConfig{Servers: map[string]lsp.ServerConfig{}}
	m := lsp.NewManager(cfg)
	defer m.Close()

	tool := &LSPStatusTool{Manager: m}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "No LSP servers configured." {
		t.Errorf("Execute() = %q, want %q", result, "No LSP servers configured.")
	}
}

func TestLSPStatusTool_Execute_WithServers(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPStatusTool{Manager: m}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "LSP Servers:") {
		t.Errorf("Execute() should contain 'LSP Servers:', got %q", result)
	}
	if !strings.Contains(result, "go:") {
		t.Errorf("Execute() should contain 'go:', got %q", result)
	}
	if !strings.Contains(result, "available") {
		t.Errorf("Execute() should contain 'available', got %q", result)
	}
}

// --- LSPDiagnosticsTool Execute tests ---

func TestLSPDiagnosticsTool_Execute_InvalidJSON(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPDiagnosticsTool{Manager: m}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLSPDiagnosticsTool_Execute_NoServerForFile(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPDiagnosticsTool{Manager: m}
	input, _ := json.Marshal(map[string]string{"path": "test.py"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No LSP server configured for .py") {
		t.Errorf("Execute() should mention no LSP server for .py, got %q", result)
	}
}

func TestLSPDiagnosticsTool_Execute_ServerError(t *testing.T) {
	m := newFailingManager()
	defer m.Close()

	tool := &LSPDiagnosticsTool{Manager: m}
	input, _ := json.Marshal(map[string]string{"path": "main.go"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("expected error when server fails to start")
	}
}

// --- LSPGotoDefinitionTool Execute tests ---

func TestLSPGotoDefinitionTool_Execute_InvalidJSON(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPGotoDefinitionTool{Manager: m}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLSPGotoDefinitionTool_Execute_NoServerForFile(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPGotoDefinitionTool{Manager: m}
	input, _ := json.Marshal(map[string]interface{}{
		"path":      "test.py",
		"line":      10,
		"character": 5,
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No LSP server configured for .py") {
		t.Errorf("Execute() should mention no LSP server for .py, got %q", result)
	}
}

func TestLSPGotoDefinitionTool_Execute_ServerError(t *testing.T) {
	m := newFailingManager()
	defer m.Close()

	tool := &LSPGotoDefinitionTool{Manager: m}
	input, _ := json.Marshal(map[string]interface{}{
		"path":      "main.go",
		"line":      10,
		"character": 5,
	})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("expected error when server fails to start")
	}
}

// --- LSPFindReferencesTool Execute tests ---

func TestLSPFindReferencesTool_Execute_InvalidJSON(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPFindReferencesTool{Manager: m}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLSPFindReferencesTool_Execute_NoServerForFile(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPFindReferencesTool{Manager: m}
	input, _ := json.Marshal(map[string]interface{}{
		"path":      "test.py",
		"line":      10,
		"character": 5,
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No LSP server configured for .py") {
		t.Errorf("Execute() should mention no LSP server for .py, got %q", result)
	}
}

func TestLSPFindReferencesTool_Execute_ServerError(t *testing.T) {
	m := newFailingManager()
	defer m.Close()

	tool := &LSPFindReferencesTool{Manager: m}
	input, _ := json.Marshal(map[string]interface{}{
		"path":      "main.go",
		"line":      10,
		"character": 5,
	})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("expected error when server fails to start")
	}
}

// --- LSPSymbolsTool Execute tests ---

func TestLSPSymbolsTool_Execute_InvalidJSON(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPSymbolsTool{Manager: m}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLSPSymbolsTool_Execute_NoServerForFile(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPSymbolsTool{Manager: m}
	input, _ := json.Marshal(map[string]string{"path": "test.py"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No LSP server configured for .py") {
		t.Errorf("Execute() should mention no LSP server for .py, got %q", result)
	}
}

func TestLSPSymbolsTool_Execute_ServerError(t *testing.T) {
	m := newFailingManager()
	defer m.Close()

	tool := &LSPSymbolsTool{Manager: m}
	input, _ := json.Marshal(map[string]string{"path": "main.go"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("expected error when server fails to start")
	}
}

// --- LSPPrepareRenameTool Execute tests ---

func TestLSPPrepareRenameTool_Execute_InvalidJSON(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPPrepareRenameTool{Manager: m}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLSPPrepareRenameTool_Execute_NoServerForFile(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPPrepareRenameTool{Manager: m}
	input, _ := json.Marshal(map[string]interface{}{
		"path":      "test.py",
		"line":      10,
		"character": 5,
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No LSP server configured for .py") {
		t.Errorf("Execute() should mention no LSP server for .py, got %q", result)
	}
}

func TestLSPPrepareRenameTool_Execute_ServerError(t *testing.T) {
	m := newFailingManager()
	defer m.Close()

	tool := &LSPPrepareRenameTool{Manager: m}
	input, _ := json.Marshal(map[string]interface{}{
		"path":      "main.go",
		"line":      10,
		"character": 5,
	})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("expected error when server fails to start")
	}
}

// --- LSPRenameTool Execute tests ---

func TestLSPRenameTool_Execute_InvalidJSON(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPRenameTool{Manager: m}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLSPRenameTool_Execute_NoServerForFile(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	tool := &LSPRenameTool{Manager: m}
	input, _ := json.Marshal(map[string]interface{}{
		"path":      "test.py",
		"line":      10,
		"character": 5,
		"new_name":  "newName",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No LSP server configured for .py") {
		t.Errorf("Execute() should mention no LSP server for .py, got %q", result)
	}
}

func TestLSPRenameTool_Execute_ServerError(t *testing.T) {
	m := newFailingManager()
	defer m.Close()

	tool := &LSPRenameTool{Manager: m}
	input, _ := json.Marshal(map[string]interface{}{
		"path":      "main.go",
		"line":      10,
		"character": 5,
		"new_name":  "newName",
	})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("expected error when server fails to start")
	}
}

// --- RegisterLSPTools tests ---

func TestRegisterLSPTools_WithExisting(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	existing := []interface{ Name() string }{
		&mockTool{name: "existing_tool"},
	}
	result := RegisterLSPTools(existing, m)
	if len(result) != 8 {
		t.Errorf("expected 8 tools (1 existing + 7 new), got %d", len(result))
	}
	if result[0].Name() != "existing_tool" {
		t.Errorf("first tool should be existing, got %q", result[0].Name())
	}
	if result[1].Name() != "lsp_status" {
		t.Errorf("second tool should be lsp_status, got %q", result[1].Name())
	}
}

func TestRegisterLSPTools_NilSlice(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	result := RegisterLSPTools(nil, m)
	if len(result) != 7 {
		t.Errorf("expected 7 tools, got %d", len(result))
	}
}

// --- mock tool for testing ---

type mockTool struct {
	name string
}

func (m *mockTool) Name() string { return m.name }

// --- Parameters tests ---

func TestLSPDiagnosticsTool_Parameters(t *testing.T) {
	m := newTestManager()
	defer m.Close()
	tool := &LSPDiagnosticsTool{Manager: m}

	params := tool.Parameters()
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be a map")
	}
	pathProp, ok := props["path"].(map[string]interface{})
	if !ok {
		t.Fatal("expected path property")
	}
	if pathProp["type"] != "string" {
		t.Errorf("path type = %v, want %q", pathProp["type"], "string")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required to be a []string")
	}
	if len(required) != 1 || required[0] != "path" {
		t.Errorf("required = %v, want [path]", required)
	}
}

func TestLSPGotoDefinitionTool_Parameters(t *testing.T) {
	m := newTestManager()
	defer m.Close()
	tool := &LSPGotoDefinitionTool{Manager: m}

	params := tool.Parameters()
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be a map")
	}
	for _, field := range []string{"path", "line", "character"} {
		if _, exists := props[field]; !exists {
			t.Errorf("expected %q in properties", field)
		}
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required to be a []string")
	}
	if len(required) != 3 {
		t.Errorf("expected 3 required fields, got %d", len(required))
	}
}

func TestLSPRenameTool_Parameters(t *testing.T) {
	m := newTestManager()
	defer m.Close()
	tool := &LSPRenameTool{Manager: m}

	params := tool.Parameters()
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be a map")
	}
	for _, field := range []string{"path", "line", "character", "new_name"} {
		if _, exists := props[field]; !exists {
			t.Errorf("expected %q in properties", field)
		}
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required to be a []string")
	}
	if len(required) != 4 {
		t.Errorf("expected 4 required fields, got %d", len(required))
	}
}
