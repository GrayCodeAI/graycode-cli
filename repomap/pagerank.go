package repomap

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SymbolGraph is a directed graph of symbol references used for PageRank
// computation over a codebase.
type SymbolGraph struct {
	nodes map[string]*SymbolNode // "file:symbol" -> node
	edges map[string][]string    // "file:symbol" -> list of referenced "file:symbol"
}

// SymbolNode is a single node in the symbol graph.
type SymbolNode struct {
	File   string
	Symbol string
	Kind   string
	Rank   float64
}

// BuildSymbolGraph scans the directory, extracts symbols using the existing
// repomap parsers, then builds a directed graph by grepping for references.
func BuildSymbolGraph(dir string, opts Options) (*SymbolGraph, error) {
	rm, err := Generate(dir, opts)
	if err != nil {
		return nil, fmt.Errorf("pagerank: generate repo map: %w", err)
	}

	sg := &SymbolGraph{
		nodes: make(map[string]*SymbolNode),
		edges: make(map[string][]string),
	}

	// Collect all symbols.
	type symInfo struct {
		key    string
		name   string
		file   string
		kind   string
	}
	var allSyms []symInfo
	for _, fm := range rm.Files {
		for _, sym := range fm.Symbols {
			key := fm.Path + ":" + sym.Name
			sg.nodes[key] = &SymbolNode{
				File:   fm.Path,
				Symbol: sym.Name,
				Kind:   sym.Kind,
				Rank:   1.0,
			}
			allSyms = append(allSyms, symInfo{
				key:  key,
				name: sym.Name,
				file: fm.Path,
				kind: sym.Kind,
			})
		}
	}

	// Build edges: for each file, check which symbols from other files are
	// referenced in its source code.
	fileContents := make(map[string]string)
	for _, fm := range rm.Files {
		absPath := filepath.Join(dir, fm.Path)
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		fileContents[fm.Path] = string(data)
	}

	for _, fm := range rm.Files {
		content, ok := fileContents[fm.Path]
		if !ok {
			continue
		}
		for _, sym := range allSyms {
			if sym.file == fm.Path {
				continue // skip self-references
			}
			if strings.Contains(content, sym.name) {
				// fm.Path references sym => edge from fm.Path symbols to sym.key
				for _, localSym := range fm.Symbols {
					localKey := fm.Path + ":" + localSym.Name
					sg.edges[localKey] = appendUnique(sg.edges[localKey], sym.key)
				}
			}
		}
	}

	return sg, nil
}

// appendUnique appends val to s if it is not already present.
func appendUnique(s []string, val string) []string {
	for _, v := range s {
		if v == val {
			return s
		}
	}
	return append(s, val)
}

// ComputePageRank runs the standard PageRank algorithm on the symbol graph.
//
//	rank[i] = (1-d) + d * sum(rank[j]/outlinks[j]) for all j->i
//
// Default: iterations=20, damping=0.85.
func (sg *SymbolGraph) ComputePageRank(iterations int, damping float64) {
	if iterations <= 0 {
		iterations = 20
	}
	if damping <= 0 || damping >= 1 {
		damping = 0.85
	}

	n := float64(len(sg.nodes))
	if n == 0 {
		return
	}

	// Initialize ranks.
	for _, node := range sg.nodes {
		node.Rank = 1.0 / n
	}

	// Build inbound edges map for efficient lookup.
	inbound := make(map[string][]string) // key -> list of keys that reference it
	for src, dsts := range sg.edges {
		for _, dst := range dsts {
			inbound[dst] = append(inbound[dst], src)
		}
	}

	for iter := 0; iter < iterations; iter++ {
		newRanks := make(map[string]float64, len(sg.nodes))

		for key := range sg.nodes {
			sum := 0.0
			for _, src := range inbound[key] {
				srcNode := sg.nodes[src]
				if srcNode == nil {
					continue
				}
				outlinks := len(sg.edges[src])
				if outlinks > 0 {
					sum += srcNode.Rank / float64(outlinks)
				}
			}
			newRanks[key] = (1.0-damping)/n + damping*sum
		}

		for key, rank := range newRanks {
			sg.nodes[key].Rank = rank
		}
	}
}

// TopSymbols returns the top-N symbols ordered by rank (descending).
func (sg *SymbolGraph) TopSymbols(n int) []SymbolNode {
	all := make([]SymbolNode, 0, len(sg.nodes))
	for _, node := range sg.nodes {
		all = append(all, *node)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Rank > all[j].Rank
	})
	if n > len(all) {
		n = len(all)
	}
	return all[:n]
}

// FormatMap renders the ranked symbols as a repo map string, highest-rank
// first, stopping when the estimated token budget is reached.
func (sg *SymbolGraph) FormatMap(maxTokens int) string {
	if maxTokens <= 0 {
		maxTokens = 2000
	}

	top := sg.TopSymbols(len(sg.nodes))

	var b strings.Builder
	tokenCount := 0

	// Group by file for readability.
	type fileEntry struct {
		path    string
		symbols []SymbolNode
	}
	fileOrder := make(map[string]*fileEntry)
	var orderedFiles []string

	for _, sym := range top {
		fe, ok := fileOrder[sym.File]
		if !ok {
			fe = &fileEntry{path: sym.File}
			fileOrder[sym.File] = fe
			orderedFiles = append(orderedFiles, sym.File)
		}
		fe.symbols = append(fe.symbols, sym)
	}

	for _, path := range orderedFiles {
		fe := fileOrder[path]
		lineEst := 1 + len(fe.symbols)
		tokEst := lineEst * 6
		if tokenCount+tokEst > maxTokens {
			remaining := len(orderedFiles) - countLines(&b)
			if remaining > 0 {
				b.WriteString(fmt.Sprintf("\n... and %d more files\n", remaining))
			}
			break
		}

		b.WriteString(fe.path + "\n")
		for _, sym := range fe.symbols {
			b.WriteString(fmt.Sprintf("  %s %s (rank %.4f)\n", sym.Kind, sym.Symbol, sym.Rank))
		}
		tokenCount += tokEst
	}

	return b.String()
}

// countLines counts non-empty, non-indented lines (approximation of file headers).
func countLines(b *strings.Builder) int {
	count := 0
	for _, line := range strings.Split(b.String(), "\n") {
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "...") {
			count++
		}
	}
	return count
}
