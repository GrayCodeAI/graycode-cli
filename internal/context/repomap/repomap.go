// Package repomap builds an AST-ranked overview of a codebase within a token
// budget, in the spirit of Aider's repository map.
//
// It constructs a graph where files are nodes and edges are import/reference
// dependencies, ranks the nodes with a PageRank-like pass, and emits a compact
// textual map (file -> key symbols) constrained to a configurable token budget.
//
// The package is intentionally dependency-light: Go files are parsed with
// go/parser + go/ast; other languages fall back to a small regex extractor.
// Only the standard library is used.
package repomap

import (
	"sort"
	"strings"
)

// Symbol is a top-level declaration extracted from a source file.
type Symbol struct {
	Name     string // declared identifier
	Kind     string // "func", "type", "const", "var", "method", etc.
	Exported bool   // true if the symbol is part of the file's public surface
}

// FileNode is a node in the repository graph.
type FileNode struct {
	Path    string   // path relative to the scanned root, slash-separated
	Lang    string   // detected language ("go", "python", ...)
	Symbols []Symbol // extracted symbols, in source order
	Imports []string // raw import/reference targets (module paths, file refs)
	Rank    float64  // PageRank score (filled in by Rank)
}

// Graph is the in-memory representation of a scanned repository.
type Graph struct {
	// Nodes is keyed by relative path.
	Nodes map[string]*FileNode
	// edges[a][b] is the weight of references from file a to file b.
	edges map[string]map[string]float64
}

// Options configures a RepoMap build.
type Options struct {
	// Budget is the approximate maximum number of tokens the emitted map may
	// occupy. Values <= 0 fall back to DefaultBudget.
	Budget int
	// MaxSymbolsPerFile caps how many symbols are listed per file. Values <= 0
	// fall back to DefaultMaxSymbolsPerFile.
	MaxSymbolsPerFile int
	// ExportedOnly, when true, lists only exported/public symbols.
	ExportedOnly bool
}

const (
	// DefaultBudget is the token budget used when Options.Budget <= 0.
	DefaultBudget = 1024
	// DefaultMaxSymbolsPerFile caps per-file symbol output by default.
	DefaultMaxSymbolsPerFile = 12
)

// RepoMap scans root, ranks files, and returns a compact map string within the
// given token budget. A budget <= 0 uses DefaultBudget.
//
// This is the primary entry point requested by the context layer.
func RepoMap(root string, budget int) (string, error) {
	g, err := Build(root)
	if err != nil {
		return "", err
	}
	g.Rank()
	return g.Render(Options{Budget: budget}), nil
}

// Build scans root and returns the populated graph (without ranking).
func Build(root string) (*Graph, error) {
	g := &Graph{
		Nodes: make(map[string]*FileNode),
		edges: make(map[string]map[string]float64),
	}
	if err := g.scan(root); err != nil {
		return nil, err
	}
	g.linkEdges()
	return g, nil
}

// addEdge records a reference of the given weight from src to dst.
func (g *Graph) addEdge(src, dst string, weight float64) {
	if src == dst {
		return
	}
	m := g.edges[src]
	if m == nil {
		m = make(map[string]float64)
		g.edges[src] = m
	}
	m[dst] += weight
}

// rankedFiles returns nodes sorted by descending rank, then ascending path for
// deterministic ordering.
func (g *Graph) rankedFiles() []*FileNode {
	nodes := make([]*FileNode, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Rank != nodes[j].Rank {
			return nodes[i].Rank > nodes[j].Rank
		}
		return nodes[i].Path < nodes[j].Path
	})
	return nodes
}

// dedupeStrings returns xs with duplicates removed, preserving first-seen order.
func dedupeStrings(xs []string) []string {
	seen := make(map[string]struct{}, len(xs))
	out := xs[:0]
	for _, x := range xs {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

// normalizePath converts an OS path to a slash-separated relative path.
func normalizePath(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
