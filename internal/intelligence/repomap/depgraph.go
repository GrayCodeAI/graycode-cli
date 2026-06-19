// depgraph.go constructs a package-level dependency graph
// for Go (via go.mod + go/parser ImportsOnly) and JavaScript/TypeScript
// (via package.json + import/require regexes). It computes topological
// order, layers, cycles, hot paths, and renders the result as DOT,
// Mermaid, or ASCII art for use in summaries and dashboards.
//
// This file holds the graph type, node mutators, renderers, and stats. The
// builders live in depgraph_build.go; the traversal algorithms live in
// depgraph_analysis.go.
package repomap

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// DepGraph represents a directed dependency graph of packages/modules.
type DepGraph struct {
	Nodes map[string]*DepNode
	Edges []DepEdge
	Root  string
	mu    sync.RWMutex
}

// DepNode represents a single package or module in the dependency graph.
type DepNode struct {
	ID         string // package/module path
	Name       string // short name
	Type       string // "internal", "external", "stdlib"
	FileCount  int
	LOC        int
	ImportedBy []string
	Imports    []string
}

// DepEdge represents a directed edge from one package to another.
type DepEdge struct {
	From   string
	To     string
	Weight int // number of imports between these packages
}

// GraphStats holds summary statistics about the dependency graph.
type GraphStats struct {
	TotalNodes    int
	InternalNodes int
	ExternalNodes int
	StdlibNodes   int
	TotalEdges    int
	MaxDepth      int
	Cycles        int
	MostImported  string
	MostImporting string
}

// NewDepGraph creates and returns a new empty DepGraph.
func NewDepGraph() *DepGraph {
	return &DepGraph{
		Nodes: make(map[string]*DepNode),
		Edges: []DepEdge{},
	}
}

// AddNode adds a node to the graph. If a node with the same ID already exists,
// it is overwritten.
func (dg *DepGraph) AddNode(node DepNode) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	dg.Nodes[node.ID] = &node
}

// AddEdge adds a directed edge to the graph. If an edge between From and To
// already exists, its weight is incremented.
func (dg *DepGraph) AddEdge(edge DepEdge) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	for i, e := range dg.Edges {
		if e.From == edge.From && e.To == edge.To {
			dg.Edges[i].Weight += edge.Weight
			return
		}
	}
	if edge.Weight == 0 {
		edge.Weight = 1
	}
	dg.Edges = append(dg.Edges, edge)
}

// RenderDOT generates a Graphviz DOT format representation of the graph.
func (dg *DepGraph) RenderDOT() string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	var b strings.Builder
	b.WriteString("digraph deps {\n")
	b.WriteString("  rankdir=LR;\n")

	// Sort edges for determinism.
	edges := make([]DepEdge, len(dg.Edges))
	copy(edges, dg.Edges)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})

	for _, edge := range edges {
		fromName := dg.shortName(edge.From)
		toName := dg.shortName(edge.To)
		b.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", fromName, toName))
	}

	b.WriteString("}\n")
	return b.String()
}

// RenderASCII generates an ASCII art visualization of the dependency graph.
func (dg *DepGraph) RenderASCII(maxWidth int) string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	if maxWidth <= 0 {
		maxWidth = 80
	}

	if len(dg.Nodes) == 0 {
		return "(empty graph)\n"
	}

	layers := dg.layersUnlocked()
	if len(layers) == 0 {
		return "(empty graph)\n"
	}

	var b strings.Builder

	// Build adjacency for forward edges.
	adj := make(map[string][]string)
	for _, edge := range dg.Edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
	}

	// Render each layer.
	for layerIdx, layer := range layers {
		if len(layer) == 0 {
			continue
		}

		// Determine box widths.
		type box struct {
			name  string
			width int
		}
		var boxes []box
		for _, id := range layer {
			name := dg.shortName(id)
			w := len(name) + 4 // padding
			if w < 7 {
				w = 7
			}
			boxes = append(boxes, box{name: name, width: w})
		}

		// Top borders.
		var topLine, midLine, botLine strings.Builder
		for i, bx := range boxes {
			if i > 0 {
				topLine.WriteString("     ")
				midLine.WriteString("────▶")
				botLine.WriteString("     ")
			}
			topLine.WriteString("┌" + strings.Repeat("─", bx.width) + "┐")
			padded := fmt.Sprintf("│ %-*s │", bx.width-2, bx.name)
			midLine.WriteString(padded)
			botLine.WriteString("└" + strings.Repeat("─", bx.width) + "┘")
		}

		b.WriteString(topLine.String() + "\n")
		b.WriteString(midLine.String() + "\n")
		b.WriteString(botLine.String() + "\n")

		// Draw vertical connectors to next layer if applicable.
		if layerIdx < len(layers)-1 {
			// Check if any node in this layer connects to the next.
			hasConnection := false
			for _, id := range layer {
				for _, dep := range adj[id] {
					for _, nextID := range layers[layerIdx+1] {
						if dep == nextID {
							hasConnection = true
							break
						}
					}
					if hasConnection {
						break
					}
				}
				if hasConnection {
					break
				}
			}
			if hasConnection {
				b.WriteString("     │\n")
				b.WriteString("     ▼\n")
			}
		}
	}

	result := b.String()
	// Truncate lines to maxWidth.
	var truncated strings.Builder
	for _, line := range strings.Split(result, "\n") {
		if len(line) > maxWidth {
			line = line[:maxWidth]
		}
		truncated.WriteString(line + "\n")
	}

	return truncated.String()
}

// RenderMermaid generates a Mermaid.js format representation for markdown rendering.
func (dg *DepGraph) RenderMermaid() string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	var b strings.Builder
	b.WriteString("graph LR\n")

	// Sort edges for determinism.
	edges := make([]DepEdge, len(dg.Edges))
	copy(edges, dg.Edges)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})

	for _, edge := range edges {
		fromName := dg.shortName(edge.From)
		toName := dg.shortName(edge.To)
		b.WriteString(fmt.Sprintf("  %s --> %s\n", fromName, toName))
	}

	return b.String()
}

// Stats returns summary statistics about the dependency graph.
func (dg *DepGraph) Stats() GraphStats {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	stats := GraphStats{
		TotalNodes: len(dg.Nodes),
		TotalEdges: len(dg.Edges),
	}

	// Count node types.
	importedByCount := make(map[string]int)
	importsCount := make(map[string]int)

	for id, node := range dg.Nodes {
		switch node.Type {
		case "internal":
			stats.InternalNodes++
		case "external":
			stats.ExternalNodes++
		case "stdlib":
			stats.StdlibNodes++
		}
		importedByCount[id] = len(node.ImportedBy)
		importsCount[id] = len(node.Imports)
	}

	// Find most imported and most importing.
	maxImportedBy := 0
	maxImports := 0
	for id, count := range importedByCount {
		if count > maxImportedBy {
			maxImportedBy = count
			stats.MostImported = id
		}
	}
	for id, count := range importsCount {
		if count > maxImports {
			maxImports = count
			stats.MostImporting = id
		}
	}

	// Count cycles.
	cycles := dg.findCyclesUnlocked()
	stats.Cycles = len(cycles)

	// Compute max depth from layers.
	layers := dg.layersUnlocked()
	stats.MaxDepth = len(layers) - 1
	if stats.MaxDepth < 0 {
		stats.MaxDepth = 0
	}

	return stats
}

// shortName returns the short display name for a node.
func (dg *DepGraph) shortName(id string) string {
	if node, ok := dg.Nodes[id]; ok && node.Name != "" {
		return node.Name
	}
	return filepath.Base(id)
}
