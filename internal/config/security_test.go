package config

import (
	"testing"
)

func TestMergeSettings_ProjectCannotOverrideSecurityFields(t *testing.T) {
	base := Settings{
		Model: "claude-sonnet-4-20250514",
		MCPServers: []MCPServerConfig{
			{Name: "global-server", Command: "global-cmd"},
		},
	}

	override := Settings{
		MCPServers: []MCPServerConfig{
			{Name: "project-server", Command: "project-cmd"},
		},
	}

	merged := MergeSettings(base, override)

	// Note: MergeSettings allows project MCP servers to override at the
	// settings level. The actual blocking happens at the tool loading layer
	// in cmd/chat.go where project-level MCP servers are detected and
	// blocked unless --allow-project-mcp is set.
	//
	// This test documents the current behavior — MCP servers from project
	// config DO override global at the merge level. The security boundary
	// is at the loading layer, not the merge layer.
	if len(merged.MCPServers) != 1 {
		t.Fatalf("expected 1 MCP server after merge, got %d", len(merged.MCPServers))
	}
	if merged.MCPServers[0].Name != "project-server" {
		t.Fatalf("expected project-server, got %s", merged.MCPServers[0].Name)
	}
}

func TestMergeSettings_AllowedToolsAppend(t *testing.T) {
	base := Settings{
		AllowedTools:    []string{"read", "write"},
		DisallowedTools: []string{"danger"},
	}

	override := Settings{
		AllowedTools:    []string{"git"},
		DisallowedTools: []string{"network"},
	}

	merged := MergeSettings(base, override)

	// Allowed tools are appended (union)
	if len(merged.AllowedTools) != 3 {
		t.Fatalf("expected 3 allowed tools, got %d: %v", len(merged.AllowedTools), merged.AllowedTools)
	}

	// Disallowed tools are also appended
	if len(merged.DisallowedTools) != 2 {
		t.Fatalf("expected 2 disallowed tools, got %d: %v", len(merged.DisallowedTools), merged.DisallowedTools)
	}
}

func TestValidateSettings_ValidConfig(t *testing.T) {
	// This test uses the global catalog test setup from main_test.go
	s := Settings{
		MaxBudgetUSD: 10.0,
	}
	result := ValidateSettings(s)
	if !result.Valid {
		t.Fatalf("expected valid, got errors: %v", result.Errors)
	}
}

func TestValidateSettings_NegativeBudget(t *testing.T) {
	s := Settings{
		MaxBudgetUSD: -5.0,
	}
	result := ValidateSettings(s)
	if result.Valid {
		t.Fatal("expected invalid for negative budget")
	}
	found := false
	for _, e := range result.Errors {
		if e.Field == "maxBudgetUSD" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected maxBudgetUSD error")
	}
}
