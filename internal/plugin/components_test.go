package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverComponentsMultiPackage(t *testing.T) {
	dir := t.TempDir()
	// plugin.json with tools
	manifest := `{
  "name": "demo",
  "version": "1.0.0",
  "description": "demo",
  "tools": [{"name": "t1", "description": "d", "command": "echo"}]
}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	// skills
	skillDir := filepath.Join(dir, "skills", "hello")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// hooks
	hooksDir := filepath.Join(dir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "pre.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// mcp
	mcp := `{"servers":[{"name":"mem","command":"npx","args":["-y","x"]}]}`
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(mcp), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := DiscoverComponents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !c.HasTools {
		t.Fatal("expected tools")
	}
	if len(c.Skills) != 1 {
		t.Fatalf("skills=%v", c.Skills)
	}
	if len(c.HookFiles) != 1 {
		t.Fatalf("hooks=%v", c.HookFiles)
	}
	if len(c.MCPServers) != 1 || c.MCPServers[0].Name != "mem" {
		t.Fatalf("mcp=%+v", c.MCPServers)
	}
	if sum := c.ComponentSummary(); sum != "tools+hooks+skills+mcp" {
		t.Fatalf("summary=%q", sum)
	}
}

func TestDiscoverScopeDirsOrder(t *testing.T) {
	// User dir always present
	dirs := DiscoverScopeDirs(t.TempDir())
	if len(dirs) == 0 {
		t.Fatal("expected at least user scope")
	}
	// First non-managed should be user if no managed
	foundUser := false
	for _, d := range dirs {
		if d.Scope == ScopeUser {
			foundUser = true
		}
	}
	if !foundUser {
		t.Fatal("expected user scope")
	}
}
