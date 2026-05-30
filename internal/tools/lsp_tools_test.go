package tools

import (
	"testing"

	"github.com/GrayCodeAI/hawk/internal/lsp"
)

func TestLSPToolNames(t *testing.T) {
	cfg := &lsp.LSPConfig{Servers: map[string]lsp.ServerConfig{
		"go": {Command: "gopls", Extensions: []string{".go"}},
	}}
	m := lsp.NewManager(cfg)
	defer m.Close()

	tools := RegisterLSPTools(nil, m)
	expected := []string{
		"lsp_status", "lsp_diagnostics", "lsp_goto_definition",
		"lsp_find_references", "lsp_symbols", "lsp_prepare_rename", "lsp_rename",
	}
	if len(tools) != len(expected) {
		t.Fatalf("expected %d tools, got %d", len(expected), len(tools))
	}
	for i, name := range expected {
		if tools[i].Name() != name {
			t.Errorf("tool[%d]: expected %q, got %q", i, name, tools[i].Name())
		}
	}
}

func TestSeverityName(t *testing.T) {
	tests := []struct {
		sev  int
		want string
	}{
		{1, "error"},
		{2, "warning"},
		{3, "info"},
		{4, "hint"},
		{99, "unknown"},
	}
	for _, tt := range tests {
		if got := severityName(tt.sev); got != tt.want {
			t.Errorf("severityName(%d) = %q, want %q", tt.sev, got, tt.want)
		}
	}
}

func TestSymbolKindName(t *testing.T) {
	tests := []struct {
		kind int
		want string
	}{
		{6, "Method"},
		{12, "Function"},
		{13, "Variable"},
		{5, "Class"},
		{23, "Struct"},
		{999, "Symbol"},
	}
	for _, tt := range tests {
		if got := symbolKindName(tt.kind); got != tt.want {
			t.Errorf("symbolKindName(%d) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}
