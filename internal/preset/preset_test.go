package preset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreset_Builtins(t *testing.T) {
	reg := Default()

	p, ok := reg.Get("code-reviewer")
	if !ok {
		t.Fatal("expected code-reviewer builtin preset")
	}
	if p.SubagentType != "explore" {
		t.Errorf("SubagentType = %q, want explore", p.SubagentType)
	}
	if p.CapabilityMode != "read-only" {
		t.Errorf("CapabilityMode = %q, want read-only", p.CapabilityMode)
	}
	if p.SandboxMode != "strict" {
		t.Errorf("SandboxMode = %q, want strict", p.SandboxMode)
	}

	p2, ok := reg.Get("architect")
	if !ok {
		t.Fatal("expected architect builtin preset")
	}
	if p2.SubagentType != "plan" {
		t.Errorf("SubagentType = %q, want plan", p2.SubagentType)
	}

	list := reg.List()
	if len(list) < 5 {
		t.Errorf("expected at least 5 builtin presets, got %d", len(list))
	}
}

func TestPreset_PrecedenceOverride(t *testing.T) {
	reg := NewRegistry()

	// 1. Builtin
	_ = reg.Register(Preset{
		Name:        "custom-agent",
		Description: "Builtin version",
		Source:      SourceBuiltin,
	})

	p, _ := reg.Get("custom-agent")
	if p.Description != "Builtin version" {
		t.Errorf("got %q, want Builtin version", p.Description)
	}

	// 2. User overrides Builtin
	_ = reg.Register(Preset{
		Name:        "custom-agent",
		Description: "User version",
		Source:      SourceUser,
	})
	p, _ = reg.Get("custom-agent")
	if p.Description != "User version" {
		t.Errorf("got %q, want User version", p.Description)
	}

	// 3. Project overrides User
	_ = reg.Register(Preset{
		Name:        "custom-agent",
		Description: "Project version",
		Source:      SourceProject,
	})
	p, _ = reg.Get("custom-agent")
	if p.Description != "Project version" {
		t.Errorf("got %q, want Project version", p.Description)
	}

	// 4. Builtin cannot downgrade Project
	_ = reg.Register(Preset{
		Name:        "custom-agent",
		Description: "Builtin attempt",
		Source:      SourceBuiltin,
	})
	p, _ = reg.Get("custom-agent")
	if p.Description != "Project version" {
		t.Errorf("got %q, want Project version", p.Description)
	}
}

func TestPreset_LoadFromDir(t *testing.T) {
	tempDir := t.TempDir()

	// Write a JSON preset
	jsonContent := `{
		"name": "json-specialist",
		"description": "JSON specialist description",
		"subagent_type": "explore",
		"capability_mode": "read-only"
	}`
	if err := os.WriteFile(filepath.Join(tempDir, "specialist.json"), []byte(jsonContent), 0o600); err != nil {
		t.Fatal(err)
	}

	// Write a YAML preset
	yamlContent := `name: yaml-specialist
description: YAML specialist description
subagent_type: plan
capability_mode: read-only
`
	if err := os.WriteFile(filepath.Join(tempDir, "specialist.yaml"), []byte(yamlContent), 0o600); err != nil {
		t.Fatal(err)
	}

	reg := NewRegistry()
	if err := reg.LoadFromDir(tempDir, SourceProject); err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}

	pJSON, ok := reg.Get("json-specialist")
	if !ok || pJSON.Description != "JSON specialist description" {
		t.Errorf("json-specialist = %+v", pJSON)
	}

	pYaml, ok := reg.Get("yaml-specialist")
	if !ok || pYaml.Description != "YAML specialist description" {
		t.Errorf("yaml-specialist = %+v", pYaml)
	}
}
