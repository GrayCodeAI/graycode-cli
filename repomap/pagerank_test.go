package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSymbolGraph(t *testing.T) {
	dir := t.TempDir()

	// Create two Go files that reference each other.
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {
	s := NewServer()
	s.Start()
}
`), 0o644)

	os.WriteFile(filepath.Join(dir, "server.go"), []byte(`package main

type Server struct{}

func NewServer() *Server { return &Server{} }

func (s *Server) Start() {}
`), 0o644)

	CacheClear()
	sg, err := BuildSymbolGraph(dir, Options{MaxFiles: 100, MaxTokens: 5000})
	if err != nil {
		t.Fatalf("BuildSymbolGraph error: %v", err)
	}

	if len(sg.nodes) == 0 {
		t.Fatal("expected non-empty symbol graph")
	}

	// main.go should reference Server and NewServer from server.go
	hasEdges := false
	for _, dsts := range sg.edges {
		if len(dsts) > 0 {
			hasEdges = true
			break
		}
	}
	if !hasEdges {
		t.Error("expected at least some edges in the graph")
	}
}

func TestComputePageRank(t *testing.T) {
	sg := &SymbolGraph{
		nodes: map[string]*SymbolNode{
			"a.go:Foo": {File: "a.go", Symbol: "Foo", Kind: "func", Rank: 1.0},
			"a.go:Bar": {File: "a.go", Symbol: "Bar", Kind: "func", Rank: 1.0},
			"b.go:Baz": {File: "b.go", Symbol: "Baz", Kind: "func", Rank: 1.0},
		},
		edges: map[string][]string{
			"a.go:Foo": {"b.go:Baz"},        // Foo references Baz
			"a.go:Bar": {"b.go:Baz"},        // Bar references Baz
			"b.go:Baz": {"a.go:Foo"},        // Baz references Foo
		},
	}

	sg.ComputePageRank(20, 0.85)

	// Baz should have the highest rank since two nodes point to it.
	baz := sg.nodes["b.go:Baz"]
	foo := sg.nodes["a.go:Foo"]
	bar := sg.nodes["a.go:Bar"]

	if baz.Rank <= foo.Rank && baz.Rank <= bar.Rank {
		t.Errorf("expected Baz to have higher rank than others: Baz=%.4f, Foo=%.4f, Bar=%.4f",
			baz.Rank, foo.Rank, bar.Rank)
	}

	// All ranks should be positive.
	for key, node := range sg.nodes {
		if node.Rank <= 0 {
			t.Errorf("expected positive rank for %s, got %.4f", key, node.Rank)
		}
	}
}

func TestTopSymbols(t *testing.T) {
	sg := &SymbolGraph{
		nodes: map[string]*SymbolNode{
			"a.go:Low":    {File: "a.go", Symbol: "Low", Kind: "func", Rank: 0.1},
			"a.go:Medium": {File: "a.go", Symbol: "Medium", Kind: "func", Rank: 0.5},
			"b.go:High":   {File: "b.go", Symbol: "High", Kind: "func", Rank: 0.9},
		},
		edges: map[string][]string{},
	}

	top := sg.TopSymbols(2)
	if len(top) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(top))
	}
	if top[0].Symbol != "High" {
		t.Errorf("expected High first, got %s", top[0].Symbol)
	}
	if top[1].Symbol != "Medium" {
		t.Errorf("expected Medium second, got %s", top[1].Symbol)
	}

	// Request more than available.
	all := sg.TopSymbols(100)
	if len(all) != 3 {
		t.Errorf("expected 3 symbols for large n, got %d", len(all))
	}
}

func TestFormatMap(t *testing.T) {
	sg := &SymbolGraph{
		nodes: map[string]*SymbolNode{
			"main.go:main":   {File: "main.go", Symbol: "main", Kind: "func", Rank: 0.3},
			"server.go:Start": {File: "server.go", Symbol: "Start", Kind: "func", Rank: 0.7},
		},
		edges: map[string][]string{},
	}

	output := sg.FormatMap(5000)
	if output == "" {
		t.Fatal("expected non-empty formatted map")
	}
	if !strings.Contains(output, "server.go") {
		t.Error("expected server.go in output (higher rank)")
	}
	if !strings.Contains(output, "main.go") {
		t.Error("expected main.go in output")
	}
	if !strings.Contains(output, "rank") {
		t.Error("expected rank values in output")
	}
}
