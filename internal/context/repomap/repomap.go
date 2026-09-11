// Package repomap is the prompt-injection shim that produces a token-budgeted
// repository overview for hawk's context layer. It builds an import/refer
// graph over the source files in root, ranks the nodes with a PageRank
// pass, and renders the highest-ranked files (and their top symbols) as a
// compact text block capped at Options.Budget tokens.
//
// # Relationship to internal/intelligence/repomap
//
// hawk ships a second package, internal/intelligence/repomap, that exposes
// a much larger surface: call graphs, search, quality signals, API
// scanning, incremental indexing, and so on. That package is the deep
// analysis engine. THIS package is intentionally narrow: one entry point
// (RepoMap), a single Graph type, a self-contained scan/rank/render
// pipeline, and a stdlib-only dependency surface. The two packages share
// no code; they merely share a name and a goal (a useful map of a
// repository). The deep package's doc.go spells this out from the other
// side of the boundary.
//
// Callers that need more than a budgeted text block - symbol-level
// navigation, BM25 search, dead-code detection, OpenAPI export, etc. -
// should import internal/intelligence/repomap directly instead of
// extending this one.
//
// # Implementation notes
//
// Go files are parsed with go/parser and go/ast; other languages fall
// back to a small regex extractor. Only the standard library is used.
// Files larger than 1 MiB, hidden directories, and a small set of
// well-known build/vendor trees are skipped during the scan. The
// PageRank pass uses the standard damped iteration with dangling-mass
// redistribution and exits early once the total node-to-node delta
// drops below 1e-9.
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
