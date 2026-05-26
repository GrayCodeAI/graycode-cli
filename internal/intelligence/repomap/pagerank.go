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
//
// If incremental is non-nil, only changed files are re-processed, avoiding a
// full re-scan of the entire codebase on every call.
func BuildSymbolGraph(dir string, opts Options, incremental ...*IncrementalMap) (*SymbolGraph, error) {
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
		key  string
		name string
		file string
		kind string
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

	// Build edges: for each file, find which symbols from other files are
	// referenced in its source code using an inverted index for O(files * symbols_per_file)
	// instead of O(files * total_symbols).
	fileContents := make(map[string]string)
	for _, fm := range rm.Files {
		absPath := filepath.Join(dir, fm.Path)
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		fileContents[fm.Path] = string(data)
	}

	// Build inverted index: symbol name -> list of (file, key) pairs
	type symRef struct {
		file string
		key  string
	}
	symIndex := make(map[string][]symRef)
	for _, sym := range allSyms {
		symIndex[sym.name] = append(symIndex[sym.name], symRef{file: sym.file, key: sym.key})
	}

	for _, fm := range rm.Files {
		content, ok := fileContents[fm.Path]
		if !ok {
			continue
		}
		// Track which remote symbols this file references
		seen := make(map[string]bool)
		for _, sym := range allSyms {
			if sym.file == fm.Path {
				continue // skip self-references
			}
			if seen[sym.name] {
				continue // already processed this symbol name for this file
			}
			if strings.Contains(content, sym.name) {
				seen[sym.name] = true
				// Add edges from all local symbols to all matching remote symbols
				for _, localSym := range fm.Symbols {
					localKey := fm.Path + ":" + localSym.Name
					for _, ref := range symIndex[sym.name] {
						if ref.file != fm.Path {
							sg.edges[localKey] = appendUnique(sg.edges[localKey], ref.key)
						}
					}
				}
			}
		}
	}

	return sg, nil
}

// UpdateGraph incrementally updates the symbol graph by re-processing only
// the changed files. Changed files are re-parsed and their old nodes/edges
// are replaced; referenced symbols from unchanged files remain in place.
func (sg *SymbolGraph) UpdateGraph(dir string, changedFiles []string) {
	if changedFiles == nil {
		return
	}

	// Remove nodes and edges for deleted/changed files
	changedSet := make(map[string]bool, len(changedFiles))
	for _, p := range changedFiles {
		changedSet[p] = true
	}

	// Remove edges pointing to or from changed files
	for srcKey, dsts := range sg.edges {
		srcParts := strings.SplitN(srcKey, ":", 2)
		srcFile := srcParts[0]
		if changedSet[srcFile] {
			delete(sg.edges, srcKey)
			continue
		}
		filtered := dsts[:0]
		for _, dst := range dsts {
			dstParts := strings.SplitN(dst, ":", 2)
			if !changedSet[dstParts[0]] {
				filtered = append(filtered, dst)
			}
		}
		if len(filtered) == 0 {
			delete(sg.edges, srcKey)
		} else {
			sg.edges[srcKey] = filtered
		}
	}

	// Remove and re-add nodes for changed files
	unchangedContent := make(map[string]string)
	for _, node := range sg.nodes {
		if !changedSet[node.File] {
			unchangedContent[node.File] = ""
		}
	}
	for _, p := range changedFiles {
		for k, node := range sg.nodes {
			if node.File == p {
				delete(sg.nodes, k)
			}
		}
	}

	// Re-parse changed files and add new nodes
	type symInfo struct {
		key  string
		name string
		file string
		kind string
	}
	var allSyms []symInfo
	fileContents := make(map[string]string)

	for _, p := range changedFiles {
		absPath := filepath.Join(dir, p)
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		fileContents[p] = string(data)

		symbols := parseFileSymbols(absPath)
		for _, sym := range symbols {
			key := p + ":" + sym.Name
			sg.nodes[key] = &SymbolNode{
				File:   p,
				Symbol: sym.Name,
				Kind:   sym.Kind,
				Rank:   1.0,
			}
			allSyms = append(allSyms, symInfo{key: key, name: sym.Name, file: p, kind: sym.Kind})
		}
	}

	// Also re-read content of unchanged files that reference changed symbols
	for f := range unchangedContent {
		if _, ok := fileContents[f]; !ok {
			absPath := filepath.Join(dir, f)
			data, err := os.ReadFile(absPath)
			if err == nil {
				fileContents[f] = string(data)
			}
		}
	}

	// Build inverted index for changed symbols
	symIndex := make(map[string][]symRef)
	for _, sym := range allSyms {
		symIndex[sym.name] = append(symIndex[sym.name], symRef{file: sym.file, key: sym.key})
	}

	// Add edges from changed files and from unchanged files that reference changed symbols
	for _, fm := range allSyms {
		content, ok := fileContents[fm.file]
		if !ok {
			continue
		}
		seen := make(map[string]bool)
		for _, sym := range allSyms {
			if sym.file == fm.file {
				continue
			}
			if seen[sym.name] {
				continue
			}
			if strings.Contains(content, sym.name) {
				seen[sym.name] = true
				localKey := fm.file + ":" + fm.name
				for _, ref := range symIndex[sym.name] {
					if ref.file != fm.file {
						sg.edges[localKey] = appendUnique(sg.edges[localKey], ref.key)
					}
				}
			}
		}
	}

	// Rebuild inbound map and re-compute PageRank
	sg.ComputePageRank(20, 0.85)
}

// symRef is an inverted-index entry mapping a symbol name to its locations.
type symRef struct {
	file string
	key  string
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
