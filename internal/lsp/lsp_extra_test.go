package lsp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// --- Config tests ---

func TestLoadConfig_WithProjectConfig(t *testing.T) {
	ResetConfig()
	// Create a temp project dir with lsp.json
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configContent := `{
		"lsp": {
			"go": {"command": "custom-gopls", "extensions": [".go"]}
		}
	}`
	if err := os.WriteFile(filepath.Join(agentsDir, "lsp.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := LoadConfig(tmpDir)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	// Custom go config should override default
	if cfg.Servers["go"].Command != "custom-gopls" {
		t.Errorf("go command = %q, want %q", cfg.Servers["go"].Command, "custom-gopls")
	}
	// Defaults should still be present for unconfigured languages
	if cfg.Servers["python"].Command != "pylsp" {
		t.Errorf("python command = %q, want %q", cfg.Servers["python"].Command, "pylsp")
	}
	ResetConfig()
}

func TestLoadConfig_EmptyProjectDir(t *testing.T) {
	ResetConfig()
	cfg := LoadConfig("")
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	// Should have defaults
	if len(cfg.Servers) == 0 {
		t.Error("expected default servers")
	}
	ResetConfig()
}

func TestResetConfig(t *testing.T) {
	ResetConfig()
	// Load config to populate
	LoadConfig("")
	// Reset
	ResetConfig()
	// Load again to verify it works after reset
	cfg := LoadConfig("")
	if cfg == nil {
		t.Error("expected non-nil config after reset")
	}
	ResetConfig()
}

func TestLoadConfigFile_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "lsp.json")
	configContent := `{
		"lsp": {
			"go": {"command": "gopls", "extensions": [".go"]}
		}
	}`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &LSPConfig{Servers: make(map[string]ServerConfig)}
	loadConfigFile(configPath, cfg)

	if cfg.Servers["go"].Command != "gopls" {
		t.Errorf("go command = %q, want %q", cfg.Servers["go"].Command, "gopls")
	}
}

func TestLoadConfigFile_NonExistent(t *testing.T) {
	cfg := &LSPConfig{Servers: make(map[string]ServerConfig)}
	loadConfigFile("/nonexistent/path/lsp.json", cfg)
	// Should be a no-op, no error
	if len(cfg.Servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(cfg.Servers))
	}
}

func TestLoadConfigFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "lsp.json")
	if err := os.WriteFile(configPath, []byte(`{invalid`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &LSPConfig{Servers: make(map[string]ServerConfig)}
	loadConfigFile(configPath, cfg)
	// Should be a no-op on parse error
	if len(cfg.Servers) != 0 {
		t.Errorf("expected 0 servers after parse error, got %d", len(cfg.Servers))
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &LSPConfig{Servers: make(map[string]ServerConfig)}
	applyDefaults(cfg)

	// All default languages should be present
	for _, lang := range []string{"go", "typescript", "python", "rust"} {
		if _, ok := cfg.Servers[lang]; !ok {
			t.Errorf("expected %q in defaults", lang)
		}
	}

	// Verify go defaults
	goCfg := cfg.Servers["go"]
	if goCfg.Command != "gopls" {
		t.Errorf("go command = %q, want %q", goCfg.Command, "gopls")
	}
	if len(goCfg.Extensions) != 1 || goCfg.Extensions[0] != ".go" {
		t.Errorf("go extensions = %v, want [.go]", goCfg.Extensions)
	}

	// Verify typescript defaults
	tsCfg := cfg.Servers["typescript"]
	if tsCfg.Command != "typescript-language-server" {
		t.Errorf("typescript command = %q, want %q", tsCfg.Command, "typescript-language-server")
	}
	if len(tsCfg.Args) != 1 || tsCfg.Args[0] != "--stdio" {
		t.Errorf("typescript args = %v, want [--stdio]", tsCfg.Args)
	}
}

func TestApplyDefaults_PreservesExisting(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go": {Command: "custom-gopls", Extensions: []string{".go"}},
	}}
	applyDefaults(cfg)

	// Custom go config should be preserved
	if cfg.Servers["go"].Command != "custom-gopls" {
		t.Errorf("go command = %q, want %q", cfg.Servers["go"].Command, "custom-gopls")
	}
	// Other defaults should be added
	if _, ok := cfg.Servers["python"]; !ok {
		t.Error("expected python in defaults")
	}
}

func TestLanguageForExtension(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go":         {Command: "gopls", Extensions: []string{".go"}},
		"typescript": {Command: "tsserver", Extensions: []string{".ts", ".tsx", ".js"}},
	}}

	tests := []struct {
		ext  string
		want string
	}{
		{".go", "go"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".js", "typescript"},
		{".py", ""},
		{"go", "go"},     // without dot
		{"GO", "go"},     // case insensitive
		{"", ""},         // empty
		{".unknown", ""}, // unknown
	}

	for _, tt := range tests {
		got := cfg.LanguageForExtension(tt.ext)
		if got != tt.want {
			t.Errorf("LanguageForExtension(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestServerForFile(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go":         {Command: "gopls", Extensions: []string{".go"}},
		"typescript": {Command: "tsserver", Extensions: []string{".ts", ".tsx"}},
	}}

	// Test with .go file
	lang, server, ok := cfg.ServerForFile("main.go")
	if !ok || lang != "go" || server.Command != "gopls" {
		t.Errorf("ServerForFile(main.go) = %q, %+v, %v; want go, gopls, true", lang, server, ok)
	}

	// Test with .ts file
	lang, server, ok = cfg.ServerForFile("app.ts")
	if !ok || lang != "typescript" || server.Command != "tsserver" {
		t.Errorf("ServerForFile(app.ts) = %q, %+v, %v; want typescript, tsserver, true", lang, server, ok)
	}

	// Test with unknown extension
	lang, server, ok = cfg.ServerForFile("readme.md")
	if ok || lang != "" {
		t.Errorf("ServerForFile(readme.md) = %q, %+v, %v; want empty, {}, false", lang, server, ok)
	}
}

func TestServerForFile_UpperCaseExtension(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go": {Command: "gopls", Extensions: []string{".go"}},
	}}

	lang, _, ok := cfg.ServerForFile("main.GO")
	if !ok || lang != "go" {
		t.Errorf("ServerForFile(main.GO) = %q, %v; want go, true", lang, ok)
	}
}

// --- Manager tests ---

func TestManagerConfig(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go": {Command: "gopls", Extensions: []string{".go"}},
	}}
	m := NewManager(cfg)
	defer m.Close()

	got := m.Config()
	if got != cfg {
		t.Error("Config() should return the same config pointer")
	}
}

func TestManagerStatus_AllStates(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go":         {Command: "gopls", Extensions: []string{".go"}},
		"typescript": {Command: "tsserver", Extensions: []string{".ts"}},
	}}
	m := NewManager(cfg)
	defer m.Close()

	// Initially all should be "available"
	status := m.Status()
	if status["go"] != "available" {
		t.Errorf("go status = %q, want %q", status["go"], "available")
	}
	if status["typescript"] != "available" {
		t.Errorf("typescript status = %q, want %q", status["typescript"], "available")
	}
}

func TestManagerStatus_WithClient(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go": {Command: "gopls", Extensions: []string{".go"}},
	}}
	m := NewManager(cfg)
	defer m.Close()

	// Manually add a client to test "connected" status
	mc := &ManagedClient{
		config:   cfg.Servers["go"],
		language: "go",
	}
	m.mu.Lock()
	m.clients["go"] = mc
	m.mu.Unlock()

	status := m.Status()
	if status["go"] != "idle" {
		t.Errorf("go status = %q, want %q", status["go"], "idle")
	}

	// Test "initializing" status
	mc.mu.Lock()
	mc.initializing = true
	mc.initStart = time.Now()
	mc.mu.Unlock()

	status = m.Status()
	if status["go"] != "initializing" {
		t.Errorf("go status = %q, want %q", status["go"], "initializing")
	}
}

func TestManagerClose_StopsReaper(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{}}
	m := NewManager(cfg)

	// Close should stop the reaper goroutine
	if err := m.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
	// Verify closed flag is set
	if !m.closed.Load() {
		t.Error("expected closed flag to be set")
	}
}

func TestLSPError_Error(t *testing.T) {
	err := &LSPError{Code: "TEST_CODE", Message: "test message"}
	expected := "TEST_CODE: test message"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestNewManagerFromProject(t *testing.T) {
	ResetConfig()
	m := NewManagerFromProject("")
	defer m.Close()

	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.Config() == nil {
		t.Error("expected non-nil config")
	}
	ResetConfig()
}

func TestReapIdle_IdleClient(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go": {Command: "gopls", Extensions: []string{".go"}},
	}}
	m := NewManager(cfg)
	defer m.Close()

	// Add a client that is idle and past the timeout
	mc := &ManagedClient{
		config:   cfg.Servers["go"],
		language: "go",
		lastUsed: time.Now().Add(-defaultIdleTimeout - time.Minute),
	}
	m.mu.Lock()
	m.clients["go"] = mc
	m.mu.Unlock()

	m.reapIdle()

	// Client should be nil after reaping
	mc.mu.Lock()
	if mc.client != nil {
		t.Error("expected client to be nil after reaping")
	}
	mc.mu.Unlock()
}

func TestReapIdle_StuckInitClient(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go": {Command: "gopls", Extensions: []string{".go"}},
	}}
	m := NewManager(cfg)
	defer m.Close()

	// Add a client that is stuck initializing
	mc := &ManagedClient{
		config:       cfg.Servers["go"],
		language:     "go",
		initializing: true,
		initStart:    time.Now().Add(-defaultInitTimeout - time.Minute),
	}
	m.mu.Lock()
	m.clients["go"] = mc
	m.mu.Unlock()

	m.reapIdle()

	// Client should be nil and initializing should be false
	mc.mu.Lock()
	if mc.client != nil {
		t.Error("expected client to be nil after reaping stuck-init")
	}
	if mc.initializing {
		t.Error("expected initializing to be false after reaping stuck-init")
	}
	mc.mu.Unlock()
}

func TestReapIdle_RecentClient(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go": {Command: "gopls", Extensions: []string{".go"}},
	}}
	m := NewManager(cfg)
	defer m.Close()

	// Add a client that was recently used (should NOT be reaped)
	mc := &ManagedClient{
		config:   cfg.Servers["go"],
		language: "go",
		lastUsed: time.Now(),
	}
	m.mu.Lock()
	m.clients["go"] = mc
	m.mu.Unlock()

	m.reapIdle()

	// Client should still be nil (we didn't set it), but lastUsed should be unchanged
	// The important thing is no panic and no error
}

func TestReapIdle_EmptyManager(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{}}
	m := NewManager(cfg)
	defer m.Close()

	// Should not panic on empty manager
	m.reapIdle()
}

// --- ServerManager tests ---

func TestNewServerManager(t *testing.T) {
	m := NewServerManager()
	if m == nil {
		t.Fatal("expected non-nil ServerManager")
	}
	if len(m.servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(m.servers))
	}
}

func TestServerManager_List_Empty(t *testing.T) {
	m := NewServerManager()
	servers := m.List()
	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}
}

func TestServerManager_IsRunning_NonExistent(t *testing.T) {
	m := NewServerManager()
	if m.IsRunning("nonexistent") {
		t.Error("expected false for non-existent server")
	}
}

func TestServerManager_Stop_NonExistent(t *testing.T) {
	m := NewServerManager()
	err := m.Stop("nonexistent")
	if err != nil {
		t.Errorf("expected nil error for stopping non-existent server, got %v", err)
	}
}

func TestServerManager_Start_NonExistentCommand(t *testing.T) {
	m := NewServerManager()
	err := m.Start("test", "nonexistent-command-xyz")
	if err == nil {
		t.Error("expected error for non-existent command")
	}
}

func TestServerManager_Start_AlreadyRunning(t *testing.T) {
	m := NewServerManager()
	// Manually add a server to test "already running" path
	m.mu.Lock()
	m.servers["test"] = &Client{}
	m.mu.Unlock()

	err := m.Start("test", "some-command")
	if err != nil {
		t.Errorf("expected nil error for already running server, got %v", err)
	}
}

func TestServerManager_List_WithServers(t *testing.T) {
	m := NewServerManager()
	m.mu.Lock()
	m.servers["server1"] = &Client{}
	m.servers["server2"] = &Client{}
	m.mu.Unlock()

	servers := m.List()
	if len(servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(servers))
	}
}

func TestServerManager_IsRunning_WithServer(t *testing.T) {
	m := NewServerManager()
	m.mu.Lock()
	m.servers["test"] = &Client{}
	m.mu.Unlock()

	if !m.IsRunning("test") {
		t.Error("expected true for running server")
	}
}

// --- Client tests (without real server) ---

func TestLSPClient_Language(t *testing.T) {
	c := &LSPClient{language: "go"}
	if c.Language() != "go" {
		t.Errorf("Language() = %q, want %q", c.Language(), "go")
	}
}

func TestLSPClient_Call_ClosedClient(t *testing.T) {
	c := &LSPClient{}
	c.closed.Store(true)

	_, err := c.call(context.Background(), "test", nil)
	if err == nil {
		t.Error("expected error for closed client")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected 'closed' in error, got %q", err.Error())
	}
}

// --- Diagnostic types tests ---

func TestDiagnosticSeverity_Constants(t *testing.T) {
	if SeverityError != 1 {
		t.Errorf("SeverityError = %d, want 1", SeverityError)
	}
	if SeverityWarning != 2 {
		t.Errorf("SeverityWarning = %d, want 2", SeverityWarning)
	}
	if SeverityInformation != 3 {
		t.Errorf("SeverityInformation = %d, want 3", SeverityInformation)
	}
	if SeverityHint != 4 {
		t.Errorf("SeverityHint = %d, want 4", SeverityHint)
	}
}

// --- Concurrency tests ---

func TestManagedClient_RefCount_Concurrent(t *testing.T) {
	mc := &ManagedClient{language: "go"}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomic.AddInt32(&mc.refCount, 1)
			mc.release()
		}()
	}
	wg.Wait()
	// refCount should be back to 0 after all release() calls
	// Note: release() decrements by 1, but we incremented by 1 in the goroutine
	// So the net should be 0
}

func TestManager_StatusConcurrent(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go": {Command: "gopls", Extensions: []string{".go"}},
	}}
	m := NewManager(cfg)
	defer m.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Status()
		}()
	}
	wg.Wait()
}

// --- Storage integration test ---

func TestLoadConfig_WithGlobalConfig(t *testing.T) {
	ResetConfig()
	// The global config file may or may not exist, but LoadConfig should handle both cases
	cfg := LoadConfig("")
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	// Should have at least the default servers
	if len(cfg.Servers) < 4 {
		t.Errorf("expected at least 4 default servers, got %d", len(cfg.Servers))
	}
	ResetConfig()
}

// --- JSON marshaling tests ---

func TestLSPConfig_JSONRoundTrip(t *testing.T) {
	original := LSPConfig{
		Servers: map[string]ServerConfig{
			"go": {
				Command:    "gopls",
				Args:       []string{"-rpcf"},
				Extensions: []string{".go"},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded LSPConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Servers["go"].Command != "gopls" {
		t.Errorf("command = %q, want %q", decoded.Servers["go"].Command, "gopls")
	}
	if len(decoded.Servers["go"].Args) != 1 || decoded.Servers["go"].Args[0] != "-rpcf" {
		t.Errorf("args = %v, want [-rpcf]", decoded.Servers["go"].Args)
	}
}

// Ensure storage import is used
var _ = storage.StateDir
