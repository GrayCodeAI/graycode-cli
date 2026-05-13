package repomap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDepGraph(t *testing.T) {
	dg := NewDepGraph()
	if dg == nil {
		t.Fatal("NewDepGraph returned nil")
	}
	if dg.Nodes == nil {
		t.Error("Nodes map should be initialized")
	}
	if dg.Edges == nil {
		t.Error("Edges slice should be initialized")
	}
	if len(dg.Nodes) != 0 {
		t.Error("Nodes should be empty initially")
	}
	if len(dg.Edges) != 0 {
		t.Error("Edges should be empty initially")
	}
}

func TestAddNode(t *testing.T) {
	dg := NewDepGraph()
	dg.AddNode(DepNode{
		ID:   "pkg/foo",
		Name: "foo",
		Type: "internal",
	})

	if len(dg.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(dg.Nodes))
	}
	node, ok := dg.Nodes["pkg/foo"]
	if !ok {
		t.Fatal("node not found")
	}
	if node.Name != "foo" {
		t.Errorf("expected name 'foo', got %q", node.Name)
	}
	if node.Type != "internal" {
		t.Errorf("expected type 'internal', got %q", node.Type)
	}
}

func TestAddEdge(t *testing.T) {
	dg := NewDepGraph()
	dg.AddEdge(DepEdge{From: "a", To: "b", Weight: 1})
	if len(dg.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(dg.Edges))
	}

	// Adding same edge again should increment weight.
	dg.AddEdge(DepEdge{From: "a", To: "b", Weight: 1})
	if len(dg.Edges) != 1 {
		t.Fatalf("expected 1 edge after duplicate, got %d", len(dg.Edges))
	}
	if dg.Edges[0].Weight != 2 {
		t.Errorf("expected weight 2, got %d", dg.Edges[0].Weight)
	}

	// Adding a different edge.
	dg.AddEdge(DepEdge{From: "a", To: "c", Weight: 3})
	if len(dg.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(dg.Edges))
	}
}

func TestAddEdgeDefaultWeight(t *testing.T) {
	dg := NewDepGraph()
	dg.AddEdge(DepEdge{From: "x", To: "y"})
	if dg.Edges[0].Weight != 1 {
		t.Errorf("expected default weight 1, got %d", dg.Edges[0].Weight)
	}
}

func TestEmptyGraphHandling(t *testing.T) {
	dg := NewDepGraph()

	// TopologicalSort on empty graph.
	sorted := dg.TopologicalSort()
	if len(sorted) != 0 {
		t.Errorf("expected empty topo sort, got %v", sorted)
	}

	// FindCycles on empty graph.
	cycles := dg.FindCycles()
	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %v", cycles)
	}

	// Layers on empty graph.
	layers := dg.Layers()
	if len(layers) != 0 {
		t.Errorf("expected no layers, got %v", layers)
	}

	// HotPaths on empty graph.
	paths := dg.HotPaths()
	if paths != nil {
		t.Errorf("expected nil hot paths, got %v", paths)
	}

	// Stats on empty graph.
	stats := dg.Stats()
	if stats.TotalNodes != 0 {
		t.Errorf("expected 0 total nodes, got %d", stats.TotalNodes)
	}

	// RenderDOT on empty graph.
	dot := dg.RenderDOT()
	if !strings.Contains(dot, "digraph deps") {
		t.Error("RenderDOT should produce valid DOT even when empty")
	}

	// RenderASCII on empty graph.
	ascii := dg.RenderASCII(80)
	if !strings.Contains(ascii, "empty graph") {
		t.Error("RenderASCII should indicate empty graph")
	}

	// RenderMermaid on empty graph.
	mermaid := dg.RenderMermaid()
	if !strings.Contains(mermaid, "graph LR") {
		t.Error("RenderMermaid should produce valid mermaid even when empty")
	}
}

func TestTopologicalSort(t *testing.T) {
	dg := NewDepGraph()

	// Build a simple chain: a -> b -> c
	dg.AddNode(DepNode{ID: "a", Name: "a", Type: "internal"})
	dg.AddNode(DepNode{ID: "b", Name: "b", Type: "internal"})
	dg.AddNode(DepNode{ID: "c", Name: "c", Type: "internal"})
	dg.AddEdge(DepEdge{From: "a", To: "b", Weight: 1})
	dg.AddEdge(DepEdge{From: "b", To: "c", Weight: 1})

	sorted := dg.TopologicalSort()
	if len(sorted) != 3 {
		t.Fatalf("expected 3 items, got %d: %v", len(sorted), sorted)
	}

	// "c" should appear before "b", and "b" before "a" (leaves first).
	indexC, indexB, indexA := -1, -1, -1
	for i, s := range sorted {
		switch s {
		case "a":
			indexA = i
		case "b":
			indexB = i
		case "c":
			indexC = i
		}
	}
	if indexC > indexB {
		t.Errorf("c (leaf) should come before b, got c at %d, b at %d", indexC, indexB)
	}
	if indexB > indexA {
		t.Errorf("b should come before a, got b at %d, a at %d", indexB, indexA)
	}
}

func TestTopologicalSortDiamond(t *testing.T) {
	dg := NewDepGraph()

	// Diamond: a -> b, a -> c, b -> d, c -> d
	dg.AddNode(DepNode{ID: "a", Name: "a", Type: "internal"})
	dg.AddNode(DepNode{ID: "b", Name: "b", Type: "internal"})
	dg.AddNode(DepNode{ID: "c", Name: "c", Type: "internal"})
	dg.AddNode(DepNode{ID: "d", Name: "d", Type: "internal"})
	dg.AddEdge(DepEdge{From: "a", To: "b", Weight: 1})
	dg.AddEdge(DepEdge{From: "a", To: "c", Weight: 1})
	dg.AddEdge(DepEdge{From: "b", To: "d", Weight: 1})
	dg.AddEdge(DepEdge{From: "c", To: "d", Weight: 1})

	sorted := dg.TopologicalSort()
	if len(sorted) != 4 {
		t.Fatalf("expected 4 items, got %d: %v", len(sorted), sorted)
	}

	// d must come before b and c; b and c must come before a.
	indexOf := make(map[string]int)
	for i, s := range sorted {
		indexOf[s] = i
	}
	if indexOf["d"] > indexOf["b"] {
		t.Error("d should come before b")
	}
	if indexOf["d"] > indexOf["c"] {
		t.Error("d should come before c")
	}
	if indexOf["b"] > indexOf["a"] {
		t.Error("b should come before a")
	}
	if indexOf["c"] > indexOf["a"] {
		t.Error("c should come before a")
	}
}

func TestFindCycles(t *testing.T) {
	dg := NewDepGraph()

	// Create a cycle: a -> b -> c -> a
	dg.AddNode(DepNode{ID: "a", Name: "a", Type: "internal"})
	dg.AddNode(DepNode{ID: "b", Name: "b", Type: "internal"})
	dg.AddNode(DepNode{ID: "c", Name: "c", Type: "internal"})
	dg.AddEdge(DepEdge{From: "a", To: "b", Weight: 1})
	dg.AddEdge(DepEdge{From: "b", To: "c", Weight: 1})
	dg.AddEdge(DepEdge{From: "c", To: "a", Weight: 1})

	cycles := dg.FindCycles()
	if len(cycles) == 0 {
		t.Fatal("expected at least one cycle")
	}

	// The cycle should contain a, b, c.
	found := false
	for _, cycle := range cycles {
		if len(cycle) == 3 {
			has := make(map[string]bool)
			for _, n := range cycle {
				has[n] = true
			}
			if has["a"] && has["b"] && has["c"] {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected cycle [a, b, c], got %v", cycles)
	}
}

func TestFindCyclesNone(t *testing.T) {
	dg := NewDepGraph()

	// DAG: a -> b -> c (no cycle)
	dg.AddNode(DepNode{ID: "a", Name: "a", Type: "internal"})
	dg.AddNode(DepNode{ID: "b", Name: "b", Type: "internal"})
	dg.AddNode(DepNode{ID: "c", Name: "c", Type: "internal"})
	dg.AddEdge(DepEdge{From: "a", To: "b", Weight: 1})
	dg.AddEdge(DepEdge{From: "b", To: "c", Weight: 1})

	cycles := dg.FindCycles()
	if len(cycles) != 0 {
		t.Errorf("expected no cycles in DAG, got %v", cycles)
	}
}

func TestLayers(t *testing.T) {
	dg := NewDepGraph()

	// Layer 0: d (no deps), Layer 1: b, c (depend on d), Layer 2: a (depends on b, c)
	dg.AddNode(DepNode{ID: "a", Name: "a", Type: "internal"})
	dg.AddNode(DepNode{ID: "b", Name: "b", Type: "internal"})
	dg.AddNode(DepNode{ID: "c", Name: "c", Type: "internal"})
	dg.AddNode(DepNode{ID: "d", Name: "d", Type: "internal"})
	dg.AddEdge(DepEdge{From: "a", To: "b", Weight: 1})
	dg.AddEdge(DepEdge{From: "a", To: "c", Weight: 1})
	dg.AddEdge(DepEdge{From: "b", To: "d", Weight: 1})
	dg.AddEdge(DepEdge{From: "c", To: "d", Weight: 1})

	layers := dg.Layers()
	if len(layers) < 3 {
		t.Fatalf("expected at least 3 layers, got %d: %v", len(layers), layers)
	}

	// Layer 0 should contain "d".
	foundD := false
	for _, id := range layers[0] {
		if id == "d" {
			foundD = true
		}
	}
	if !foundD {
		t.Errorf("expected 'd' in layer 0, got %v", layers[0])
	}

	// Layer 1 should contain "b" and "c".
	layer1Set := make(map[string]bool)
	for _, id := range layers[1] {
		layer1Set[id] = true
	}
	if !layer1Set["b"] || !layer1Set["c"] {
		t.Errorf("expected 'b' and 'c' in layer 1, got %v", layers[1])
	}

	// Layer 2 should contain "a".
	foundA := false
	for _, id := range layers[2] {
		if id == "a" {
			foundA = true
		}
	}
	if !foundA {
		t.Errorf("expected 'a' in layer 2, got %v", layers[2])
	}
}

func TestLayersSingleNode(t *testing.T) {
	dg := NewDepGraph()
	dg.AddNode(DepNode{ID: "x", Name: "x", Type: "internal"})

	layers := dg.Layers()
	if len(layers) != 1 {
		t.Fatalf("expected 1 layer for single node, got %d", len(layers))
	}
	if layers[0][0] != "x" {
		t.Errorf("expected 'x' in layer 0, got %v", layers[0])
	}
}

func TestRenderDOT(t *testing.T) {
	dg := NewDepGraph()
	dg.AddNode(DepNode{ID: "engine", Name: "engine", Type: "internal"})
	dg.AddNode(DepNode{ID: "tool", Name: "tool", Type: "internal"})
	dg.AddNode(DepNode{ID: "config", Name: "config", Type: "internal"})
	dg.AddEdge(DepEdge{From: "engine", To: "tool", Weight: 1})
	dg.AddEdge(DepEdge{From: "engine", To: "config", Weight: 1})

	dot := dg.RenderDOT()

	if !strings.Contains(dot, "digraph deps {") {
		t.Error("DOT output should start with 'digraph deps {'")
	}
	if !strings.Contains(dot, "rankdir=LR") {
		t.Error("DOT output should contain 'rankdir=LR'")
	}
	if !strings.Contains(dot, `"engine" -> "tool"`) {
		t.Error("DOT output should contain edge from engine to tool")
	}
	if !strings.Contains(dot, `"engine" -> "config"`) {
		t.Error("DOT output should contain edge from engine to config")
	}
	if !strings.HasSuffix(strings.TrimSpace(dot), "}") {
		t.Error("DOT output should end with '}'")
	}
}

func TestRenderASCII(t *testing.T) {
	dg := NewDepGraph()
	dg.AddNode(DepNode{ID: "engine", Name: "engine", Type: "internal"})
	dg.AddNode(DepNode{ID: "tool", Name: "tool", Type: "internal"})
	dg.AddEdge(DepEdge{From: "engine", To: "tool", Weight: 1})

	ascii := dg.RenderASCII(120)

	// Should contain box-drawing characters.
	if !strings.Contains(ascii, "┌") {
		t.Error("ASCII output should contain box-drawing characters")
	}
	if !strings.Contains(ascii, "┐") {
		t.Error("ASCII output should contain box-drawing characters")
	}
	if !strings.Contains(ascii, "└") {
		t.Error("ASCII output should contain box-drawing characters")
	}
	if !strings.Contains(ascii, "┘") {
		t.Error("ASCII output should contain box-drawing characters")
	}

	// Should contain node names.
	if !strings.Contains(ascii, "engine") && !strings.Contains(ascii, "tool") {
		t.Error("ASCII output should contain node names")
	}
}

func TestRenderASCIIMaxWidth(t *testing.T) {
	dg := NewDepGraph()
	dg.AddNode(DepNode{ID: "very_long_package_name_here", Name: "very_long_package_name_here", Type: "internal"})
	dg.AddNode(DepNode{ID: "another_very_long_name", Name: "another_very_long_name", Type: "internal"})
	dg.AddEdge(DepEdge{From: "very_long_package_name_here", To: "another_very_long_name", Weight: 1})

	ascii := dg.RenderASCII(40)

	for _, line := range strings.Split(ascii, "\n") {
		if len(line) > 40 {
			t.Errorf("line exceeds maxWidth 40: %q (len=%d)", line, len(line))
		}
	}
}

func TestRenderMermaid(t *testing.T) {
	dg := NewDepGraph()
	dg.AddNode(DepNode{ID: "engine", Name: "engine", Type: "internal"})
	dg.AddNode(DepNode{ID: "tool", Name: "tool", Type: "internal"})
	dg.AddNode(DepNode{ID: "sandbox", Name: "sandbox", Type: "internal"})
	dg.AddEdge(DepEdge{From: "engine", To: "tool", Weight: 1})
	dg.AddEdge(DepEdge{From: "tool", To: "sandbox", Weight: 1})

	mermaid := dg.RenderMermaid()

	if !strings.HasPrefix(mermaid, "graph LR\n") {
		t.Error("Mermaid output should start with 'graph LR'")
	}
	if !strings.Contains(mermaid, "engine --> tool") {
		t.Error("Mermaid output should contain 'engine --> tool'")
	}
	if !strings.Contains(mermaid, "tool --> sandbox") {
		t.Error("Mermaid output should contain 'tool --> sandbox'")
	}
}

func TestStats(t *testing.T) {
	dg := NewDepGraph()
	dg.AddNode(DepNode{ID: "app", Name: "app", Type: "internal", Imports: []string{"fmt", "lib"}, ImportedBy: []string{}})
	dg.AddNode(DepNode{ID: "lib", Name: "lib", Type: "internal", Imports: []string{"fmt"}, ImportedBy: []string{"app"}})
	dg.AddNode(DepNode{ID: "fmt", Name: "fmt", Type: "stdlib", Imports: []string{}, ImportedBy: []string{"app", "lib"}})
	dg.AddNode(DepNode{ID: "github.com/foo/bar", Name: "bar", Type: "external", Imports: []string{}, ImportedBy: []string{}})
	dg.AddEdge(DepEdge{From: "app", To: "lib", Weight: 1})
	dg.AddEdge(DepEdge{From: "app", To: "fmt", Weight: 1})
	dg.AddEdge(DepEdge{From: "lib", To: "fmt", Weight: 1})

	stats := dg.Stats()

	if stats.TotalNodes != 4 {
		t.Errorf("expected 4 total nodes, got %d", stats.TotalNodes)
	}
	if stats.InternalNodes != 2 {
		t.Errorf("expected 2 internal nodes, got %d", stats.InternalNodes)
	}
	if stats.ExternalNodes != 1 {
		t.Errorf("expected 1 external node, got %d", stats.ExternalNodes)
	}
	if stats.StdlibNodes != 1 {
		t.Errorf("expected 1 stdlib node, got %d", stats.StdlibNodes)
	}
	if stats.TotalEdges != 3 {
		t.Errorf("expected 3 total edges, got %d", stats.TotalEdges)
	}
	if stats.MostImported != "fmt" {
		t.Errorf("expected most imported 'fmt', got %q", stats.MostImported)
	}
	if stats.MostImporting != "app" {
		t.Errorf("expected most importing 'app', got %q", stats.MostImporting)
	}
	if stats.Cycles != 0 {
		t.Errorf("expected 0 cycles, got %d", stats.Cycles)
	}
	if stats.MaxDepth < 2 {
		t.Errorf("expected max depth >= 2, got %d", stats.MaxDepth)
	}
}

func TestStatsWithCycles(t *testing.T) {
	dg := NewDepGraph()
	dg.AddNode(DepNode{ID: "a", Name: "a", Type: "internal"})
	dg.AddNode(DepNode{ID: "b", Name: "b", Type: "internal"})
	dg.AddEdge(DepEdge{From: "a", To: "b", Weight: 1})
	dg.AddEdge(DepEdge{From: "b", To: "a", Weight: 1})

	stats := dg.Stats()
	if stats.Cycles == 0 {
		t.Error("expected at least 1 cycle detected")
	}
}

func TestNodeClassification(t *testing.T) {
	tests := []struct {
		importPath string
		moduleName string
		external   []string
		expected   string
	}{
		{"fmt", "github.com/example/app", nil, "stdlib"},
		{"os/exec", "github.com/example/app", nil, "stdlib"},
		{"github.com/example/app/internal/pkg", "github.com/example/app", nil, "internal"},
		{"github.com/other/lib", "github.com/example/app", []string{"github.com/other/lib"}, "external"},
		{"golang.org/x/tools", "github.com/example/app", []string{"golang.org/x/tools"}, "external"},
	}

	for _, tt := range tests {
		result := classifyImport(tt.importPath, tt.moduleName, tt.external)
		if result != tt.expected {
			t.Errorf("classifyImport(%q, %q, %v) = %q, want %q",
				tt.importPath, tt.moduleName, tt.external, result, tt.expected)
		}
	}
}

func TestEdgeWeightCounting(t *testing.T) {
	dg := NewDepGraph()
	dg.AddNode(DepNode{ID: "a", Name: "a", Type: "internal"})
	dg.AddNode(DepNode{ID: "b", Name: "b", Type: "internal"})

	// Simulate multiple imports from a to b.
	dg.AddEdge(DepEdge{From: "a", To: "b", Weight: 1})
	dg.AddEdge(DepEdge{From: "a", To: "b", Weight: 1})
	dg.AddEdge(DepEdge{From: "a", To: "b", Weight: 1})

	if len(dg.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(dg.Edges))
	}
	if dg.Edges[0].Weight != 3 {
		t.Errorf("expected weight 3, got %d", dg.Edges[0].Weight)
	}
}

func TestBuildFromGoMod(t *testing.T) {
	// Create a temporary test project.
	tmpDir := t.TempDir()

	// Create go.mod.
	goMod := `module github.com/test/myapp

go 1.21

require (
	github.com/external/lib v1.0.0
	github.com/another/dep v0.5.0
)
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// Create main.go.
	mainGo := `package main

import (
	"fmt"
	"os"

	"github.com/test/myapp/internal/config"
	"github.com/external/lib"
)

func main() {
	fmt.Println("hello")
	os.Exit(0)
	_ = config.Load()
	_ = lib.New()
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatal(err)
	}

	// Create internal/config package.
	configDir := filepath.Join(tmpDir, "internal", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configGo := `package config

import "fmt"

func Load() error {
	fmt.Println("loading config")
	return nil
}
`
	if err := os.WriteFile(filepath.Join(configDir, "config.go"), []byte(configGo), 0644); err != nil {
		t.Fatal(err)
	}

	dg := NewDepGraph()
	if err := dg.BuildFromGoMod(tmpDir); err != nil {
		t.Fatalf("BuildFromGoMod failed: %v", err)
	}

	// Check root module.
	if dg.Root != "github.com/test/myapp" {
		t.Errorf("expected root 'github.com/test/myapp', got %q", dg.Root)
	}

	// Check that internal packages exist.
	if _, ok := dg.Nodes["github.com/test/myapp"]; !ok {
		t.Error("expected root module node")
	}
	if _, ok := dg.Nodes["github.com/test/myapp/internal/config"]; !ok {
		t.Error("expected internal/config node")
	}

	// Check external deps.
	if _, ok := dg.Nodes["github.com/external/lib"]; !ok {
		t.Error("expected external/lib node")
	}

	// Check stdlib nodes were created.
	if _, ok := dg.Nodes["fmt"]; !ok {
		t.Error("expected 'fmt' stdlib node")
	}

	// Verify node types.
	if node, ok := dg.Nodes["github.com/test/myapp"]; ok {
		if node.Type != "internal" {
			t.Errorf("root module should be internal, got %q", node.Type)
		}
	}
	if node, ok := dg.Nodes["github.com/external/lib"]; ok {
		if node.Type != "external" {
			t.Errorf("external/lib should be external, got %q", node.Type)
		}
	}
	if node, ok := dg.Nodes["fmt"]; ok {
		if node.Type != "stdlib" {
			t.Errorf("fmt should be stdlib, got %q", node.Type)
		}
	}

	// Check file count and LOC.
	if node, ok := dg.Nodes["github.com/test/myapp/internal/config"]; ok {
		if node.FileCount != 1 {
			t.Errorf("expected file count 1, got %d", node.FileCount)
		}
		if node.LOC == 0 {
			t.Error("expected non-zero LOC for config package")
		}
	}

	// Check edges exist.
	if len(dg.Edges) == 0 {
		t.Error("expected edges to be created")
	}

	// Verify a specific edge.
	foundEdge := false
	for _, e := range dg.Edges {
		if e.From == "github.com/test/myapp" && e.To == "fmt" {
			foundEdge = true
			break
		}
	}
	if !foundEdge {
		t.Error("expected edge from root module to fmt")
	}
}

func TestBuildFromGoModMissing(t *testing.T) {
	tmpDir := t.TempDir()
	dg := NewDepGraph()
	err := dg.BuildFromGoMod(tmpDir)
	if err == nil {
		t.Error("expected error for missing go.mod")
	}
}

func TestBuildFromPackageJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create package.json.
	pkgJSON := `{
  "name": "my-app",
  "version": "1.0.0",
  "dependencies": {
    "express": "^4.18.0",
    "lodash": "^4.17.0"
  },
  "devDependencies": {
    "typescript": "^5.0.0"
  }
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Create src directory with files.
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	indexJS := `import express from 'express';
import { merge } from 'lodash';
import { helper } from './utils';

const app = express();
`
	if err := os.WriteFile(filepath.Join(srcDir, "index.js"), []byte(indexJS), 0644); err != nil {
		t.Fatal(err)
	}

	utilsJS := `import path from 'path';

export function helper() {
  return path.join('a', 'b');
}
`
	if err := os.WriteFile(filepath.Join(srcDir, "utils.js"), []byte(utilsJS), 0644); err != nil {
		t.Fatal(err)
	}

	dg := NewDepGraph()
	if err := dg.BuildFromPackageJSON(tmpDir); err != nil {
		t.Fatalf("BuildFromPackageJSON failed: %v", err)
	}

	// Check root.
	if dg.Root != "my-app" {
		t.Errorf("expected root 'my-app', got %q", dg.Root)
	}

	// Check external deps were added.
	if _, ok := dg.Nodes["express"]; !ok {
		t.Error("expected 'express' node")
	}
	if _, ok := dg.Nodes["lodash"]; !ok {
		t.Error("expected 'lodash' node")
	}
	if _, ok := dg.Nodes["typescript"]; !ok {
		t.Error("expected 'typescript' node")
	}

	// Check that edges were created for imports.
	if len(dg.Edges) == 0 {
		t.Error("expected edges to be created from JS imports")
	}

	// path should be classified as stdlib.
	if node, ok := dg.Nodes["path"]; ok {
		if node.Type != "stdlib" {
			t.Errorf("'path' should be stdlib, got %q", node.Type)
		}
	}
}

func TestBuildFromPackageJSONMissing(t *testing.T) {
	tmpDir := t.TempDir()
	dg := NewDepGraph()
	err := dg.BuildFromPackageJSON(tmpDir)
	if err == nil {
		t.Error("expected error for missing package.json")
	}
}

func TestHotPaths(t *testing.T) {
	dg := NewDepGraph()

	// Create a graph where "core" is the most depended-on node.
	dg.AddNode(DepNode{ID: "app", Name: "app", Type: "internal"})
	dg.AddNode(DepNode{ID: "api", Name: "api", Type: "internal"})
	dg.AddNode(DepNode{ID: "core", Name: "core", Type: "internal"})
	dg.AddNode(DepNode{ID: "utils", Name: "utils", Type: "internal"})
	dg.AddNode(DepNode{ID: "db", Name: "db", Type: "internal"})

	dg.AddEdge(DepEdge{From: "app", To: "core", Weight: 5})
	dg.AddEdge(DepEdge{From: "api", To: "core", Weight: 3})
	dg.AddEdge(DepEdge{From: "core", To: "utils", Weight: 2})
	dg.AddEdge(DepEdge{From: "core", To: "db", Weight: 4})
	dg.AddEdge(DepEdge{From: "app", To: "api", Weight: 1})

	paths := dg.HotPaths()
	if len(paths) == 0 {
		t.Fatal("expected at least one hot path")
	}

	// The most critical path should involve "core" since it has the highest PageRank.
	foundCore := false
	for _, path := range paths {
		for _, node := range path {
			if node == "core" {
				foundCore = true
				break
			}
		}
		if foundCore {
			break
		}
	}
	if !foundCore {
		t.Error("expected 'core' to appear in hot paths due to high PageRank")
	}
}

func TestParseModuleName(t *testing.T) {
	tests := []struct {
		content  string
		expected string
	}{
		{"module github.com/foo/bar\n\ngo 1.21\n", "github.com/foo/bar"},
		{"module example.com/mymod\n", "example.com/mymod"},
		{"// comment\nmodule test/mod\n", "test/mod"},
		{"", ""},
	}

	for _, tt := range tests {
		result := parseModuleName(tt.content)
		if result != tt.expected {
			t.Errorf("parseModuleName(%q) = %q, want %q", tt.content, result, tt.expected)
		}
	}
}

func TestParseGoModRequires(t *testing.T) {
	content := `module github.com/test/app

go 1.21

require (
	github.com/foo/bar v1.0.0
	github.com/baz/qux v2.3.4
)

require github.com/single/dep v0.1.0
`
	deps := parseGoModRequires(content)
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d: %v", len(deps), deps)
	}

	expected := map[string]bool{
		"github.com/foo/bar":    true,
		"github.com/baz/qux":   true,
		"github.com/single/dep": true,
	}
	for _, dep := range deps {
		if !expected[dep] {
			t.Errorf("unexpected dep: %q", dep)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	dg := NewDepGraph()

	// Test concurrent AddNode and AddEdge.
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			id := fmt.Sprintf("pkg%d", idx)
			dg.AddNode(DepNode{ID: id, Name: id, Type: "internal"})
			if idx > 0 {
				dg.AddEdge(DepEdge{From: id, To: fmt.Sprintf("pkg%d", idx-1), Weight: 1})
			}
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	if len(dg.Nodes) != 10 {
		t.Errorf("expected 10 nodes after concurrent adds, got %d", len(dg.Nodes))
	}
}

func TestIsNodeBuiltin(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"fs", true},
		{"path", true},
		{"http", true},
		{"node:fs", true},
		{"node:test", true},
		{"express", false},
		{"lodash", false},
		{"@types/node", false},
	}

	for _, tt := range tests {
		result := isNodeBuiltin(tt.name)
		if result != tt.expected {
			t.Errorf("isNodeBuiltin(%q) = %v, want %v", tt.name, result, tt.expected)
		}
	}
}

func TestAppendUniqueStr(t *testing.T) {
	s := []string{"a", "b", "c"}
	s = appendUniqueStr(s, "b")
	if len(s) != 3 {
		t.Errorf("expected 3 elements (no dup), got %d", len(s))
	}
	s = appendUniqueStr(s, "d")
	if len(s) != 4 {
		t.Errorf("expected 4 elements after adding new, got %d", len(s))
	}
}

func TestContainsCycle(t *testing.T) {
	cycles := [][]string{
		{"a", "b", "c"},
		{"x", "y"},
	}

	if !containsCycle(cycles, []string{"a", "b", "c"}) {
		t.Error("should find existing cycle")
	}
	if containsCycle(cycles, []string{"a", "b"}) {
		t.Error("should not find non-existing cycle")
	}
	if containsCycle(cycles, []string{"c", "b", "a"}) {
		t.Error("should not match reversed cycle")
	}
}

func TestCountFileLOC(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")
	content := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	loc := countFileLOC(path)
	// Non-blank lines: package, import, func, fmt.Println, closing brace = at least 5
	if loc < 5 {
		t.Errorf("expected at least 5 LOC, got %d", loc)
	}
}

func TestCountFileLOCMissing(t *testing.T) {
	loc := countFileLOC("/nonexistent/path/file.go")
	if loc != 0 {
		t.Errorf("expected 0 for missing file, got %d", loc)
	}
}
