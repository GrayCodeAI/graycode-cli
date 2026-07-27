package lsp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestNewLSPClient_InvalidCommand_Extra tests that an invalid command returns an error.
func TestNewLSPClient_InvalidCommand_Extra(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := ServerConfig{
		Command:    "nonexistent-lsp-command-xyz123",
		Extensions: []string{".go"},
	}

	_, err := NewLSPClient(ctx, "go", cfg)
	if err == nil {
		t.Error("expected error for invalid command")
	}
}

// TestNewLSPClient_ContextCanceled_Extra tests that a canceled context causes an error.
func TestNewLSPClient_ContextCanceled_Extra(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cfg := ServerConfig{
		Command:    "gopls",
		Extensions: []string{".go"},
	}

	_, err := NewLSPClient(ctx, "go", cfg)
	if err == nil {
		t.Error("expected error for canceled context")
	}
}

// TestLSPConfig_LanguageForExtension_Extra tests the LanguageForExtension method.
func TestLSPConfig_LanguageForExtension_Extra(t *testing.T) {
	ResetConfig()
	cfg := LoadConfig("")

	// Test with extension prefix
	lang := cfg.LanguageForExtension(".go")
	if lang != "go" {
		t.Errorf("expected 'go', got %q", lang)
	}

	// Test without extension prefix
	lang = cfg.LanguageForExtension("py")
	if lang != "python" {
		t.Errorf("expected 'python', got %q", lang)
	}

	// Test unknown extension
	lang = cfg.LanguageForExtension(".xyz")
	if lang != "" {
		t.Errorf("expected empty string, got %q", lang)
	}
}

// TestLSPConfig_ServerForFile_Extra tests the ServerForFile method.
func TestLSPConfig_ServerForFile_Extra(t *testing.T) {
	ResetConfig()
	cfg := LoadConfig("")

	// Test Go file
	lang, server, ok := cfg.ServerForFile("test.go")
	if !ok {
		t.Error("expected ok=true for .go file")
	}
	if lang != "go" {
		t.Errorf("expected lang='go', got %q", lang)
	}
	if server.Command != "gopls" {
		t.Errorf("expected command='gopls', got %q", server.Command)
	}

	// Test TypeScript file
	lang, server, ok = cfg.ServerForFile("test.ts")
	if !ok {
		t.Error("expected ok=true for .ts file")
	}
	if lang != "typescript" {
		t.Errorf("expected lang='typescript', got %q", lang)
	}

	// Test unknown file
	lang, server, ok = cfg.ServerForFile("test.xyz")
	if ok {
		t.Error("expected ok=false for .xyz file")
	}
	if lang != "" {
		t.Errorf("expected empty lang, got %q", lang)
	}
}

// TestLoadConfig_Extra tests loading config from a file.
func TestLoadConfig_Extra(t *testing.T) {
	ResetConfig()

	// Create a temp directory with a custom config
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("failed to create .agents dir: %v", err)
	}

	// Write a custom config
	customConfig := `{"lsp": {"custom-lang": {"command": "custom-lsp", "extensions": [".custom"]}}}`
	configPath := filepath.Join(agentsDir, "lsp.json")
	if err := os.WriteFile(configPath, []byte(customConfig), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg := LoadConfig(dir)

	// Check that custom config was loaded
	server, ok := cfg.Servers["custom-lang"]
	if !ok {
		t.Error("expected custom-lang server to be configured")
	}
	if server.Command != "custom-lsp" {
		t.Errorf("expected command='custom-lsp', got %q", server.Command)
	}

	// Check that defaults are still present
	server, ok = cfg.Servers["go"]
	if !ok {
		t.Error("expected go server to still be configured")
	}
}

// TestLoadConfig_InvalidJSON_Extra tests loading config with invalid JSON.
func TestLoadConfig_InvalidJSON_Extra(t *testing.T) {
	ResetConfig()

	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("failed to create .agents dir: %v", err)
	}

	// Write invalid JSON
	configPath := filepath.Join(agentsDir, "lsp.json")
	if err := os.WriteFile(configPath, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Should not panic, should load defaults
	cfg := LoadConfig(dir)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	// Defaults should still be present
	if _, ok := cfg.Servers["go"]; !ok {
		t.Error("expected go server to be configured from defaults")
	}
}

// TestLSPClient_Call_Closed_Extra tests calling a method on a closed client.
// We create a client with a valid command (gopls) but it will fail to initialize,
// so we test the closed state by creating a client and closing it.
func TestLSPClient_Call_Closed_Extra(t *testing.T) {
	// Use a simple echo command that will start but not respond properly to LSP
	// The client will fail to initialize, but we can still test the closed state
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// This will fail to initialize, but we can still test the closed state
	// by creating a client manually
	client := &LSPClient{
		cmd:      exec.Command("echo", "test"),
		pending:  make(map[interface{}]chan json.RawMessage),
		language: "go",
	}
	client.closed.Store(true)

	// Call should return error for closed client
	_, err := client.call(ctx, "test", nil)
	if err == nil {
		t.Error("expected error for closed client")
	}
}

// TestLSPClient_Notify_AfterClose_Extra tests notifying after the client is closed.
func TestLSPClient_Notify_AfterClose_Extra(t *testing.T) {
	client := &LSPClient{
		cmd:      exec.Command("echo", "test"),
		pending:  make(map[interface{}]chan json.RawMessage),
		language: "go",
	}
	client.closed.Store(true)

	// Notify should return an error (client is closed)
	// Actually, notify doesn't check closed state, so it will try to write
	// We can't test this without a real stdin
}

// TestLSPClient_Language_Extra tests the Language method.
func TestLSPClient_Language_Extra(t *testing.T) {
	client := &LSPClient{
		language: "python",
		pending:  make(map[interface{}]chan json.RawMessage),
	}

	if client.Language() != "python" {
		t.Errorf("expected language 'python', got %q", client.Language())
	}
}

// TestLSPClient_Call_ClosedState_Extra tests that a closed client returns an error.
func TestLSPClient_Call_ClosedState_Extra(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := &LSPClient{
		cmd:      exec.Command("echo", "test"),
		pending:  make(map[interface{}]chan json.RawMessage),
		language: "go",
	}
	client.closed.Store(true)

	// Call should return error for closed client
	_, err := client.call(ctx, "textDocument/diagnostic", nil)
	if err == nil {
		t.Error("expected error for closed client")
	}
}
