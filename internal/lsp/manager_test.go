package lsp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	cfg := &LSPConfig{
		Servers: map[string]ServerConfig{
			"go": {Command: "gopls", Extensions: []string{".go"}},
		},
	}
	m := NewManager(cfg)
	defer m.Close()

	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	status := m.Status()
	if status["go"] != "available" {
		t.Errorf("expected 'available', got %q", status["go"])
	}
}

func TestManagerStatus(t *testing.T) {
	cfg := &LSPConfig{
		Servers: map[string]ServerConfig{
			"go":         {Command: "gopls", Extensions: []string{".go"}},
			"typescript": {Command: "typescript-language-server", Args: []string{"--stdio"}, Extensions: []string{".ts"}},
		},
	}
	m := NewManager(cfg)
	defer m.Close()

	status := m.Status()
	if len(status) != 2 {
		t.Errorf("expected 2 languages, got %d", len(status))
	}
	for lang := range cfg.Servers {
		if s, ok := status[lang]; !ok || s != "available" {
			t.Errorf("expected %q to be 'available', got %q", lang, s)
		}
	}
}

func TestManagerExecute_LanguageNotConfigured(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{}}
	m := NewManager(cfg)
	defer m.Close()

	err := m.Execute(context.TODO(), "haskell", true, func(c *LSPClient) error { return nil })
	if err == nil {
		t.Error("expected error for unconfigured language")
	}
	var lspErr *LSPError
	if errors.As(err, &lspErr) {
		if lspErr.Code != "LANG_NOT_CONFIGURED" {
			t.Errorf("expected LANG_NOT_CONFIGURED, got %s", lspErr.Code)
		}
	}
}

func TestManagerExecute_ClosedManager(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{
		"go": {Command: "gopls", Extensions: []string{".go"}},
	}}
	m := NewManager(cfg)
	m.Close()

	err := m.Execute(context.TODO(), "go", true, func(c *LSPClient) error { return nil })
	if err == nil {
		t.Error("expected error from closed manager")
	}
}

func TestManagerClose_Idempotent(t *testing.T) {
	cfg := &LSPConfig{Servers: map[string]ServerConfig{}}
	m := NewManager(cfg)
	if err := m.Close(); err != nil {
		t.Errorf("first close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("second close: %v", err)
	}
}

func TestReadOnlyRetryTools(t *testing.T) {
	// Verify retry-safe tools
	safe := []string{"diagnostics", "goto_definition", "find_references", "symbols", "prepare_rename", "status"}
	for _, name := range safe {
		if !ReadOnlyRetryTools[name] {
			t.Errorf("expected %q to be retry-safe", name)
		}
	}
	// Verify mutating tools are NOT retry-safe
	unsafe := []string{"rename", "apply_edit", "format"}
	for _, name := range unsafe {
		if ReadOnlyRetryTools[name] {
			t.Errorf("expected %q to NOT be retry-safe", name)
		}
	}
}

func TestManagedClient_RefCount(t *testing.T) {
	mc := &ManagedClient{language: "go"}
	if mc.refCount != 0 {
		t.Errorf("expected initial refcount 0, got %d", mc.refCount)
	}
	// Simulate acquire
	mc.refCount++
	if mc.refCount != 1 {
		t.Errorf("expected refcount 1, got %d", mc.refCount)
	}
	// Simulate release
	mc.release()
	if mc.refCount != 0 {
		t.Errorf("expected refcount 0 after release, got %d", mc.refCount)
	}
}

func TestConfigForFile(t *testing.T) {
	cfg := &LSPConfig{
		Servers: map[string]ServerConfig{
			"go":         {Command: "gopls", Extensions: []string{".go"}},
			"typescript": {Command: "typescript-language-server", Args: []string{"--stdio"}, Extensions: []string{".ts", ".tsx"}},
		},
	}

	lang, server, ok := cfg.ServerForFile("main.go")
	if !ok || lang != "go" || server.Command != "gopls" {
		t.Errorf("expected go/gopls, got %v/%v/%v", lang, server.Command, ok)
	}

	lang, _, ok = cfg.ServerForFile("app.tsx")
	if !ok || lang != "typescript" {
		t.Errorf("expected typescript, got %v/%v", lang, ok)
	}

	_, _, ok = cfg.ServerForFile("readme.md")
	if ok {
		t.Error("expected no server for .md files")
	}
}

func TestReaper_IdleTimeout(t *testing.T) {
	// Verify reaper constants
	if defaultIdleTimeout != 5*time.Minute {
		t.Errorf("expected 5m idle timeout, got %v", defaultIdleTimeout)
	}
	if defaultInitTimeout != 30*time.Second {
		t.Errorf("expected 30s init timeout, got %v", defaultInitTimeout)
	}
	if reaperInterval != 30*time.Second {
		t.Errorf("expected 30s reaper interval, got %v", reaperInterval)
	}
}
