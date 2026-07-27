package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// TestLSPTools_Metadata verifies that all LSP tool implementations return
// non-empty metadata (Name, Description, RiskLevel, Parameters).
func TestLSPTools_Metadata(t *testing.T) {
	mgr := newTestManager()
	defer mgr.Close()

	tools := []struct {
		name string
		tool interface {
			Name() string
			Description() string
			RiskLevel() string
			Parameters() map[string]interface{}
		}
	}{
		{"GotoDefinition", &LSPGotoDefinitionTool{Manager: mgr}},
		{"FindReferences", &LSPFindReferencesTool{Manager: mgr}},
		{"Symbols", &LSPSymbolsTool{Manager: mgr}},
		{"PrepareRename", &LSPPrepareRenameTool{Manager: mgr}},
		{"Rename", &LSPRenameTool{Manager: mgr}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			if name := tc.tool.Name(); name == "" {
				t.Error("Name() returned empty string")
			}
			if desc := tc.tool.Description(); desc == "" {
				t.Error("Description() returned empty string")
			}
			if risk := tc.tool.RiskLevel(); risk == "" {
				t.Error("RiskLevel() returned empty string")
			}
			params := tc.tool.Parameters()
			if params == nil {
				t.Fatal("Parameters() returned nil")
			}
			if _, ok := params["type"]; !ok {
				t.Error("Parameters() missing 'type' field")
			}
			if _, ok := params["properties"]; !ok {
				t.Error("Parameters() missing 'properties' field")
			}
			if _, ok := params["required"]; !ok {
				t.Error("Parameters() missing 'required' field")
			}
		})
	}
}

// TestLSPTools_Execute_NoServer verifies that tools return a helpful message
// when no LSP server is configured for the file type.
func TestLSPTools_Execute_NoServer(t *testing.T) {
	mgr := newTestManager()
	defer mgr.Close()

	ctx := context.Background()

	// GotoDefinition with unknown file type
	t.Run("GotoDefinition_NoServer", func(t *testing.T) {
		tool := &LSPGotoDefinitionTool{Manager: mgr}
		input, _ := json.Marshal(map[string]interface{}{
			"path":      "test.unknownext",
			"line":      1,
			"character": 0,
		})
		result, err := tool.Execute(ctx, input)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result == "" {
			t.Error("expected non-empty result for no server")
		}
	})

	// FindReferences with unknown file type
	t.Run("FindReferences_NoServer", func(t *testing.T) {
		tool := &LSPFindReferencesTool{Manager: mgr}
		input, _ := json.Marshal(map[string]interface{}{
			"path":      "test.unknownext",
			"line":      1,
			"character": 0,
		})
		result, err := tool.Execute(ctx, input)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result == "" {
			t.Error("expected non-empty result for no server")
		}
	})

	// Symbols with unknown file type
	t.Run("Symbols_NoServer", func(t *testing.T) {
		tool := &LSPSymbolsTool{Manager: mgr}
		input, _ := json.Marshal(map[string]interface{}{
			"path": "test.unknownext",
		})
		result, err := tool.Execute(ctx, input)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result == "" {
			t.Error("expected non-empty result for no server")
		}
	})

	// PrepareRename with unknown file type
	t.Run("PrepareRename_NoServer", func(t *testing.T) {
		tool := &LSPPrepareRenameTool{Manager: mgr}
		input, _ := json.Marshal(map[string]interface{}{
			"path":      "test.unknownext",
			"line":      1,
			"character": 0,
		})
		result, err := tool.Execute(ctx, input)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result == "" {
			t.Error("expected non-empty result for no server")
		}
	})

	// Rename with unknown file type
	t.Run("Rename_NoServer", func(t *testing.T) {
		tool := &LSPRenameTool{Manager: mgr}
		input, _ := json.Marshal(map[string]interface{}{
			"path":      "test.unknownext",
			"line":      1,
			"character": 0,
			"new_name":  "newName",
		})
		result, err := tool.Execute(ctx, input)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result == "" {
			t.Error("expected non-empty result for no server")
		}
	})
}

// TestLSPTools_Execute_InvalidInput verifies that tools handle invalid JSON input gracefully.
func TestLSPTools_Execute_InvalidInput(t *testing.T) {
	mgr := newTestManager()
	defer mgr.Close()

	ctx := context.Background()

	t.Run("GotoDefinition_InvalidJSON", func(t *testing.T) {
		tool := &LSPGotoDefinitionTool{Manager: mgr}
		_, err := tool.Execute(ctx, json.RawMessage("not valid json"))
		if err == nil {
			t.Error("expected error for invalid JSON input")
		}
	})

	t.Run("FindReferences_InvalidJSON", func(t *testing.T) {
		tool := &LSPFindReferencesTool{Manager: mgr}
		_, err := tool.Execute(ctx, json.RawMessage("not valid json"))
		if err == nil {
			t.Error("expected error for invalid JSON input")
		}
	})

	t.Run("Symbols_InvalidJSON", func(t *testing.T) {
		tool := &LSPSymbolsTool{Manager: mgr}
		_, err := tool.Execute(ctx, json.RawMessage("not valid json"))
		if err == nil {
			t.Error("expected error for invalid JSON input")
		}
	})

	t.Run("PrepareRename_InvalidJSON", func(t *testing.T) {
		tool := &LSPPrepareRenameTool{Manager: mgr}
		_, err := tool.Execute(ctx, json.RawMessage("not valid json"))
		if err == nil {
			t.Error("expected error for invalid JSON input")
		}
	})

	t.Run("Rename_InvalidJSON", func(t *testing.T) {
		tool := &LSPRenameTool{Manager: mgr}
		_, err := tool.Execute(ctx, json.RawMessage("not valid json"))
		if err == nil {
			t.Error("expected error for invalid JSON input")
		}
	})
}
