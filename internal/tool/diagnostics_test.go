package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticsToolMetadata(t *testing.T) {
	d := DiagnosticsTool{}
	if d.Name() != "Diagnostics" {
		t.Fatalf("expected name 'Diagnostics', got %q", d.Name())
	}
	if d.Description() == "" {
		t.Fatal("expected non-empty description")
	}
	aliases := d.Aliases()
	if len(aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(aliases))
	}
	params := d.Parameters()
	if params == nil {
		t.Fatal("expected non-nil parameters")
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}
	if _, ok := props["path"]; !ok {
		t.Fatal("expected 'path' parameter")
	}
}

func TestDiagnosticsToolMissingPath(t *testing.T) {
	d := DiagnosticsTool{}
	input, _ := json.Marshal(map[string]string{})
	_, err := d.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestDiagnosticsToolUnsupportedExt(t *testing.T) {
	d := DiagnosticsTool{}
	input, _ := json.Marshal(map[string]string{"path": "/tmp/test.xyz"})
	_, err := d.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected 'unsupported' in error, got: %s", err.Error())
	}
}
