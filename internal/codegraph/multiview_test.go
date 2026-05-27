package codegraph

import (
	"testing"
)

func TestBuildMultiViewGraph_SimpleFile(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

func HelloWorld() string {
	return "hello"
}

func Add(a, b int) int {
	return a + b
}
`)

	graph, err := BuildMultiViewGraph("main.go", source)
	if err != nil {
		t.Fatalf("BuildMultiViewGraph failed: %v", err)
	}

	if graph == nil {
		t.Fatal("expected non-nil graph")
	}
	if graph.AST == nil {
		t.Error("AST view should not be nil")
	}
	if graph.DFG == nil {
		t.Error("DFG view should not be nil")
	}
	if graph.CFG == nil {
		t.Error("CFG view should not be nil")
	}
	if graph.Call == nil {
		t.Error("Call view should not be nil")
	}
}

func TestBuildMultiViewGraph_ASTNodes(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

func Hello() {}

type Config struct {
	Name string
}

var Version = "1.0"
`)

	graph, err := BuildMultiViewGraph("main.go", source)
	if err != nil {
		t.Fatalf("BuildMultiViewGraph failed: %v", err)
	}

	// Should have function, type, and var nodes
	funcFound := false
	typeFound := false
	varFound := false

	for _, node := range graph.AST.Nodes {
		switch node.Type {
		case "func":
			if node.Name == "Hello" {
				funcFound = true
			}
		case "type":
			if node.Name == "Config" {
				typeFound = true
			}
		case "var":
			if node.Name == "Version" {
				varFound = true
			}
		}
	}

	if !funcFound {
		t.Error("expected to find Hello function node")
	}
	if !typeFound {
		t.Error("expected to find Config type node")
	}
	if !varFound {
		t.Error("expected to find Version var node")
	}
}

func TestBuildMultiViewGraph_CallGraph(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

func helper() int {
	return 42
}

func main() {
	x := helper()
	_ = x
}
`)

	graph, err := BuildMultiViewGraph("main.go", source)
	if err != nil {
		t.Fatalf("BuildMultiViewGraph failed: %v", err)
	}

	// Should have call edges from main to helper
	hasCallEdge := false
	for _, edge := range graph.Call.Edges {
		if edge.Callee == "main.go:helper" {
			hasCallEdge = true
		}
	}

	if !hasCallEdge {
		t.Error("expected call edge from main to helper")
	}
}

func TestBuildMultiViewGraph_CFGBranches(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

func Check(x int) string {
	if x > 0 {
		return "positive"
	}
	return "non-positive"
}
`)

	graph, err := BuildMultiViewGraph("main.go", source)
	if err != nil {
		t.Fatalf("BuildMultiViewGraph failed: %v", err)
	}

	// Should have entry node
	hasEntry := false
	for _, node := range graph.CFG.Nodes {
		if node.Type == "entry" {
			hasEntry = true
		}
	}

	if !hasEntry {
		t.Error("expected entry node in CFG")
	}
}

func TestBuildMultiViewGraph_DFGVariables(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

func Process() {
	x := 10
	y := x + 5
	_ = y
}
`)

	graph, err := BuildMultiViewGraph("main.go", source)
	if err != nil {
		t.Fatalf("BuildMultiViewGraph failed: %v", err)
	}

	// Should have at least some DFG nodes
	if len(graph.DFG.Nodes) == 0 {
		t.Error("expected DFG nodes for variable assignments")
	}
}

func TestBuildMultiViewGraph_InvalidSyntax(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

func broken( {
`)

	_, err := BuildMultiViewGraph("broken.go", source)
	if err == nil {
		t.Error("expected error for invalid syntax")
	}
}

func TestBuildMultiViewGraph_EmptyFile(t *testing.T) {
	t.Parallel()
	source := []byte(`package main
`)

	graph, err := BuildMultiViewGraph("empty.go", source)
	if err != nil {
		t.Fatalf("BuildMultiViewGraph failed: %v", err)
	}

	// Empty file should have empty views but not nil
	if graph.AST == nil || graph.DFG == nil || graph.CFG == nil || graph.Call == nil {
		t.Error("views should not be nil for empty file")
	}
}
