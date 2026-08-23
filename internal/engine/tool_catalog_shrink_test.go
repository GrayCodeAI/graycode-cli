package engine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/types"
)

func bloatedTools() []types.EyrieTool {
	return []types.EyrieTool{
		{
			Name: "read_file",
			Description: strings.Repeat("Reads a file from disk quickly and safely. ", 30) +
				"The path must be an absolute path. You cannot read binary files.",
			Parameters: map[string]interface{}{
				"type":    "object",
				"$schema": "http://json-schema.org/draft-07/schema#",
				"title":   "params",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "tiny_tool",
			Description: "ok",
			Parameters:  map[string]interface{}{"type": "object"},
		},
	}
}

func TestShrinkEyrieToolsDisabledByDefault(t *testing.T) {
	t.Setenv("HAWK_TOOL_SHRINK", "")
	in := bloatedTools()
	out := shrinkEyrieTools(in)
	if len(out) != len(in) || out[0].Description != in[0].Description {
		t.Fatal("shrink must be a no-op when disabled")
	}
}

func TestShrinkEyrieToolsEnabledReducesAndPreservesNames(t *testing.T) {
	t.Setenv("HAWK_TOOL_SHRINK", "1")
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	in := bloatedTools()
	out := shrinkEyrieTools(in)
	if len(out) != 2 {
		t.Fatalf("tool count changed: %d", len(out))
	}
	if out[0].Name != "read_file" || out[1].Name != "tiny_tool" {
		t.Fatalf("names drifted: %q %q", out[0].Name, out[1].Name)
	}
	if len(out[0].Description) >= len(in[0].Description) {
		t.Fatalf("description not reduced: %d vs %d", len(out[0].Description), len(in[0].Description))
	}
	if !strings.Contains(out[0].Description, "must be an absolute path") {
		t.Fatalf("constraint sentence lost: %q", out[0].Description)
	}
	// required survived through the schema tree
	if req, ok := out[0].Parameters["required"]; !ok || req == nil {
		t.Fatalf("required dropped: %+v", out[0].Parameters)
	}
}

func TestBuildOptionsAppliesShrink(t *testing.T) {
	t.Setenv("HAWK_TOOL_SHRINK", "1")
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	c := &ChatService{}
	opts := c.BuildOptions("sys", "m", 100, bloatedTools())
	if len(opts.Tools) != 2 || opts.Tools[0].Name != "read_file" {
		t.Fatalf("tools = %+v", opts.Tools)
	}
	if len(opts.Tools[0].Description) >= len(bloatedTools()[0].Description) {
		t.Fatal("BuildOptions did not shrink the catalog")
	}
}

func TestOriginalCatalogPersistedForRecovery(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("HAWK_TOOL_SHRINK", "1")
	t.Setenv("HAWK_STATE_DIR", stateDir)
	in := bloatedTools()
	_ = shrinkEyrieTools(in)
	matches, err := filepath.Glob(stateDir + "/tool-catalog-originals/*.json")
	if err != nil || len(matches) == 0 {
		t.Fatalf("original catalog not persisted: %v %v", matches, err)
	}
}
