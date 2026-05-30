package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettingsPartial_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	data := `{
		"model": "claude-sonnet-4-20250514",
		"theme": "dark",
		"max_budget_usd": 10.5,
		"auto_allow": ["read", "write"],
		"mcp_servers": [{"name": "test", "command": "echo"}]
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	ps := LoadSettingsPartial(path)
	if len(ps.Errors) > 0 {
		t.Fatalf("expected no errors, got %v", ps.Errors)
	}
	if ps.Settings.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("expected model 'claude-sonnet-4-20250514', got %q", ps.Settings.Model)
	}
	if ps.Settings.Theme != "dark" {
		t.Fatalf("expected theme 'dark', got %q", ps.Settings.Theme)
	}
	if ps.Settings.MaxBudgetUSD != 10.5 {
		t.Fatalf("expected budget 10.5, got %f", ps.Settings.MaxBudgetUSD)
	}
	if len(ps.Settings.AutoAllow) != 2 {
		t.Fatalf("expected 2 auto_allow entries, got %d", len(ps.Settings.AutoAllow))
	}
	if len(ps.Settings.MCPServers) != 1 {
		t.Fatalf("expected 1 MCP server, got %d", len(ps.Settings.MCPServers))
	}
}

func TestLoadSettingsPartial_MalformedMCPServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	// mcp_servers is a string instead of array — should fail that section but load the rest
	data := `{
		"model": "claude-sonnet-4-20250514",
		"theme": "dark",
		"mcp_servers": "not-an-array"
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	ps := LoadSettingsPartial(path)

	// Should have an error for mcp_servers
	hasMCPError := false
	for _, e := range ps.Errors {
		if e.Section == "mcp_servers" {
			hasMCPError = true
		}
	}
	if !hasMCPError {
		t.Fatal("expected error for mcp_servers section")
	}

	// Model and theme should still be loaded
	if ps.Settings.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("expected model to load, got %q", ps.Settings.Model)
	}
	if ps.Settings.Theme != "dark" {
		t.Fatalf("expected theme to load, got %q", ps.Settings.Theme)
	}
}

func TestLoadSettingsPartial_MalformedModelRoles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	data := `{
		"model": "claude-sonnet-4-20250514",
		"model_roles": "not-an-object"
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	ps := LoadSettingsPartial(path)

	hasModelError := false
	for _, e := range ps.Errors {
		if e.Section == "model_roles" {
			hasModelError = true
		}
	}
	if !hasModelError {
		t.Fatal("expected error for model_roles section")
	}

	if ps.Settings.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("expected model to load, got %q", ps.Settings.Model)
	}
}

func TestLoadSettingsPartial_CompletelyInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := os.WriteFile(path, []byte("not json at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	ps := LoadSettingsPartial(path)
	if len(ps.Errors) == 0 {
		t.Fatal("expected errors for completely invalid JSON")
	}
	if ps.Settings.Model != "" {
		t.Fatal("expected zero settings for invalid JSON")
	}
}

func TestLoadSettingsPartial_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	ps := LoadSettingsPartial(path)
	if len(ps.Errors) > 0 {
		t.Fatalf("expected no errors for empty file, got %v", ps.Errors)
	}
}

func TestLoadSettingsPartial_NonexistentFile(t *testing.T) {
	ps := LoadSettingsPartial("/nonexistent/path/settings.json")
	if len(ps.Errors) > 0 {
		t.Fatalf("expected no errors for missing file, got %v", ps.Errors)
	}
}
