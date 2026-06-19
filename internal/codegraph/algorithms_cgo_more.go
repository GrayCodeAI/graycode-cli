//go:build cgo

package codegraph

import (
	"context"
	"fmt"
	"sort"
)

// This file holds the ranking, impact, coupling, and cross-repo graph
// algorithms. Centrality, community detection, connected components, graph
// diff/snapshot, and dead-code analysis live in algorithms_cgo.go.

// PageRank computes PageRank on the code graph's call/reference edges.
// This is more accurate than repomap's PageRank because it uses the precise
// call graph from tree-sitter parsing rather than string matching.
func (cg *CodeGraph) PageRank(iterations int, damping float64) (map[string]float64, error) {
	if iterations <= 0 {
		iterations = 20
	}
	if damping <= 0 {
		damping = 0.85
	}

	cg.mu.RLock()
	defer cg.mu.RUnlock()

	// Build adjacency
	outlinks := make(map[string][]string)
	inlinks := make(map[string][]string)
	nodes := make(map[string]bool)

	rows, err := cg.db.QueryContext(context.Background(), "SELECT source, target FROM edges WHERE kind IN ('calls', 'references', 'imports')")
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // deferred Close on read-only rows is cleanup-only

	for rows.Next() {
		var src, tgt string
		if err := rows.Scan(&src, &tgt); err != nil {
			return nil, fmt.Errorf("scanning edge row: %w", err)
		}
		outlinks[src] = append(outlinks[src], tgt)
		inlinks[tgt] = append(inlinks[tgt], src)
		nodes[src] = true
		nodes[tgt] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating edges: %w", err)
	}

	n := float64(len(nodes))
	if n == 0 {
		return make(map[string]float64), nil
	}

	// Initialize ranks
	rank := make(map[string]float64)
	for id := range nodes {
		rank[id] = 1.0 / n
	}

	// Iterate
	for iter := 0; iter < iterations; iter++ {
		newRank := make(map[string]float64)

		// Collect dangling rank (nodes with no outlinks)
		danglingSum := 0.0
		for id := range nodes {
			if len(outlinks[id]) == 0 {
				danglingSum += rank[id]
			}
		}

		for id := range nodes {
			sum := 0.0
			for _, src := range inlinks[id] {
				sum += rank[src] / float64(len(outlinks[src]))
			}
			newRank[id] = (1-damping)/n + damping*(sum+danglingSum/n)
		}

		rank = newRank
	}

	return rank, nil
}

// ImpactAnalysis computes the blast radius of changing a symbol.
// Uses the full call graph to find all directly and transitively affected nodes.
func (cg *CodeGraph) ImpactAnalysis(nodeID string, maxDepth int) (*ImpactResult, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}

	cg.mu.RLock()
	defer cg.mu.RUnlock()

	result := &ImpactResult{
		Root:     nodeID,
		Impacted: make(map[string]int), // nodeID -> depth
	}

	// BFS from the changed node through incoming edges
	visited := make(map[string]bool)
	type step struct {
		id    string
		depth int
	}
	queue := []step{{nodeID, 0}}

	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]

		if visited[s.id] || s.depth > maxDepth {
			continue
		}
		visited[s.id] = true
		result.Impacted[s.id] = s.depth

		// Get all nodes that depend on this one
		rows, _ := cg.db.QueryContext(
			context.Background(),
			`SELECT source FROM edges WHERE target = ? AND kind IN ('calls', 'references', 'imports', 'extends', 'implements')`, s.id,
		)
		if rows != nil {
			for rows.Next() {
				var source string
				if err := rows.Scan(&source); err != nil {
					rows.Close()
					return nil, fmt.Errorf("scanning dependency row for %s: %w", s.id, err)
				}
				if !visited[source] {
					queue = append(queue, step{source, s.depth + 1})
				}
			}
			rows.Close()
		}
	}

	// Load node details
	for id, depth := range result.Impacted {
		var n Node
		err := cg.db.QueryRowContext(
			context.Background(),
			`SELECT id, kind, name, qualified_name, file_path, language,
			        start_line, end_line, signature, docstring, visibility, is_exported
			 FROM nodes WHERE id = ?`, id,
		).Scan(
			&n.ID, &n.Kind, &n.Name, &n.QualifiedName, &n.FilePath, &n.Language,
			&n.StartLine, &n.EndLine, &n.Signature, &n.Docstring, &n.Visibility, &n.IsExported,
		)
		if err == nil {
			result.Nodes = append(result.Nodes, n)
			if depth > result.MaxDepth {
				result.MaxDepth = depth
			}
		}
	}

	// Sort by depth
	sort.Slice(result.Nodes, func(i, j int) bool {
		return result.Impacted[result.Nodes[i].ID] < result.Impacted[result.Nodes[j].ID]
	})

	return result, nil
}

// ImpactResult holds the result of impact analysis.
type ImpactResult struct {
	Root     string         `json:"root"`
	Impacted map[string]int `json:"impacted"` // nodeID -> depth
	Nodes    []Node         `json:"nodes"`
	MaxDepth int            `json:"max_depth"`
}

// CouplingMetric represents coupling between two modules/files.
type CouplingMetric struct {
	FileA      string  `json:"file_a"`
	FileB      string  `json:"file_b"`
	SharedDeps int     `json:"shared_deps"` // number of shared dependencies
	Coupling   float64 `json:"coupling"`    // 0-1 coupling score
}

// AnalyzeCoupling finds pairs of files that are tightly coupled (share many dependencies).
func (cg *CodeGraph) AnalyzeCoupling(topN int) ([]CouplingMetric, error) {
	if topN <= 0 {
		topN = 10
	}

	cg.mu.RLock()
	defer cg.mu.RUnlock()

	// Build file -> set of referenced symbols
	fileDeps := make(map[string]map[string]bool)
	rows, err := cg.db.QueryContext(context.Background(), "SELECT file_path, target FROM edges e JOIN nodes n ON n.id = e.source WHERE e.kind IN ('calls', 'references', 'imports')")
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // deferred Close on read-only rows is cleanup-only

	for rows.Next() {
		var filePath, target string
		if err := rows.Scan(&filePath, &target); err != nil {
			return nil, fmt.Errorf("scanning file dependency row: %w", err)
		}
		if fileDeps[filePath] == nil {
			fileDeps[filePath] = make(map[string]bool)
		}
		fileDeps[filePath][target] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating file dependencies: %w", err)
	}

	// Compute pairwise coupling
	var metrics []CouplingMetric
	files := make([]string, 0, len(fileDeps))
	for f := range fileDeps {
		files = append(files, f)
	}

	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			shared := 0
			for dep := range fileDeps[files[i]] {
				if fileDeps[files[j]][dep] {
					shared++
				}
			}
			if shared > 0 {
				total := len(fileDeps[files[i]]) + len(fileDeps[files[j]])
				coupling := float64(shared*2) / float64(total)
				metrics = append(metrics, CouplingMetric{
					FileA:      files[i],
					FileB:      files[j],
					SharedDeps: shared,
					Coupling:   coupling,
				})
			}
		}
	}

	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].Coupling > metrics[j].Coupling
	})

	if topN > len(metrics) {
		topN = len(metrics)
	}
	return metrics[:topN], nil
}

// CrossRepoQuery queries across multiple codegraph databases.
// Useful for finding relationships between hawk, eyrie, tok, yaad, etc.
func CrossRepoQuery(repos []string, query string, limit int) (map[string][]Node, error) {
	results := make(map[string][]Node)

	for _, repoRoot := range repos {
		cg, err := Open(repoRoot)
		if err != nil {
			continue // Skip repos without codegraph
		}

		nodes, err := cg.Search(query, limit)
		cg.Close()
		if err != nil || len(nodes) == 0 {
			continue
		}

		results[repoRoot] = nodes
	}

	return results, nil
}

// CrossRepoImpact finds the impact of changing a symbol across multiple repos.
// If a symbol in hawk calls a symbol in eyrie, this traces that cross-repo dependency.
func CrossRepoImpact(repos []string, symbol string, maxDepth int) (map[string]*ImpactResult, error) {
	results := make(map[string]*ImpactResult)

	for _, repoRoot := range repos {
		cg, err := Open(repoRoot)
		if err != nil {
			continue
		}

		// Search for the symbol
		nodes, err := cg.Search(symbol, 5)
		if err != nil || len(nodes) == 0 {
			cg.Close()
			continue
		}

		// Get impact for each matching symbol
		for _, n := range nodes {
			impact, err := cg.ImpactAnalysis(n.ID, maxDepth)
			if err != nil {
				continue
			}
			results[repoRoot+":"+n.Name] = impact
		}

		cg.Close()
	}

	return results, nil
}

// FindCrossRepoCalls finds function calls that cross repo boundaries.
// For example, hawk calling eyrie functions.
func FindCrossRepoCalls(repos []string) ([]CrossRepoCall, error) {
	type repoSymbol struct {
		repo string
		node Node
	}

	// Build a map of all symbols across repos
	allSymbols := make(map[string][]repoSymbol) // name -> [{repo, node}]
	repoNodes := make(map[string]map[string]bool)

	for _, repoRoot := range repos {
		cg, err := Open(repoRoot)
		if err != nil {
			continue
		}

		repoNodes[repoRoot] = make(map[string]bool)

		// Get all nodes
		rows, err := cg.db.QueryContext(context.Background(), "SELECT id, kind, name, qualified_name, file_path, language, start_line, end_line, signature, docstring, visibility, is_exported FROM nodes")
		if err != nil {
			cg.Close()
			continue
		}

		nodes, _ := scanNodes(rows)
		rows.Close()

		for _, n := range nodes {
			allSymbols[n.Name] = append(allSymbols[n.Name], repoSymbol{repoRoot, n})
			repoNodes[repoRoot][n.ID] = true
		}

		cg.Close()
	}

	// Find calls that reference symbols in other repos
	var crossCalls []CrossRepoCall

	for _, repoRoot := range repos {
		cg, err := Open(repoRoot)
		if err != nil {
			continue
		}

		// Get unresolved refs (calls to symbols not in this repo)
		rows, err := cg.db.QueryContext(context.Background(), "SELECT from_node_id, reference_name, file_path, line FROM unresolved_refs")
		if err != nil {
			cg.Close()
			continue
		}

		for rows.Next() {
			var fromID, refName, filePath string
			var line int
			rows.Scan(&fromID, &refName, &filePath, &line)

			// Check if this reference exists in another repo
			for _, target := range allSymbols[refName] {
				if target.repo != repoRoot {
					crossCalls = append(crossCalls, CrossRepoCall{
						FromRepo: repoRoot,
						ToRepo:   target.repo,
						Symbol:   refName,
						File:     filePath,
						Line:     line,
						Target:   target.node,
					})
				}
			}
		}

		rows.Close()
		cg.Close()
	}

	return crossCalls, nil
}

// CrossRepoCall represents a function call that crosses repo boundaries.
type CrossRepoCall struct {
	FromRepo string `json:"from_repo"`
	ToRepo   string `json:"to_repo"`
	Symbol   string `json:"symbol"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Target   Node   `json:"target"`
}
