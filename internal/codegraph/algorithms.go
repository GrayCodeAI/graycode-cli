package codegraph

import (
	"sort"
	"strings"
)

// BetweennessResult holds centrality scores for nodes.
type BetweennessResult struct {
	Scores map[string]float64 `json:"scores"` // nodeID -> centrality score
	Top    []NodeCentrality   `json:"top"`    // top-N by centrality
}

type NodeCentrality struct {
	NodeID   string  `json:"node_id"`
	Name     string  `json:"name"`
	FilePath string  `json:"file_path"`
	Score    float64 `json:"score"`
	Kind     string  `json:"kind"`
}

// BetweennessCentrality computes betweenness centrality for all nodes in the graph.
// Betweenness centrality measures how often a node lies on shortest paths between
// other nodes — high-centrality nodes are "bridges" connecting different parts of
// the codebase. Useful for finding coupling hotspots.
//
// Algorithm: Brandes' algorithm (O(VE) for unweighted graphs).
func (cg *CodeGraph) BetweennessCentrality(topN int) (*BetweennessResult, error) {
	if topN <= 0 {
		topN = 20
	}

	cg.mu.RLock()
	defer cg.mu.RUnlock()

	// Build adjacency list from edges
	adj := make(map[string][]string)
	rows, err := cg.db.Query("SELECT source, target FROM edges WHERE kind IN ('calls', 'references', 'imports', 'extends', 'implements')")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := make(map[string]bool)
	for rows.Next() {
		var src, tgt string
		rows.Scan(&src, &tgt)
		adj[src] = append(adj[src], tgt)
		adj[tgt] = append(adj[tgt], src) // undirected for centrality
		nodes[src] = true
		nodes[tgt] = true
	}

	// Brandes' algorithm
	cb := make(map[string]float64)
	for nodeID := range nodes {
		cb[nodeID] = 0
	}

	for s := range nodes {
		// BFS from s
		var stack []string
		predecessors := make(map[string][]string)
		sigma := make(map[string]float64)
		sigma[s] = 1
		dist := make(map[string]int)
		dist[s] = 0

		var queue []string
		queue = append(queue, s)

		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			stack = append(stack, v)

			for _, w := range adj[v] {
				// First time visiting w?
				if _, exists := dist[w]; !exists {
					dist[w] = dist[v] + 1
					queue = append(queue, w)
				}
				// Shortest path to w via v?
				if dist[w] == dist[v]+1 {
					sigma[w] += sigma[v]
					predecessors[w] = append(predecessors[w], v)
				}
			}
		}

		// Back-propagation
		delta := make(map[string]float64)
		for len(stack) > 0 {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			for _, v := range predecessors[w] {
				delta[v] += (sigma[v] / sigma[w]) * (1 + delta[w])
			}
			if w != s {
				cb[w] += delta[w]
			}
		}
	}

	// Normalize
	n := float64(len(nodes))
	if n > 2 {
		scale := 1.0 / ((n - 1) * (n - 2))
		for id := range cb {
			cb[id] *= scale
		}
	}

	// Get top-N
	type scored struct {
		id    string
		score float64
	}
	var sorted []scored
	for id, score := range cb {
		sorted = append(sorted, scored{id, score})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	result := &BetweennessResult{
		Scores: cb,
	}

	limit := topN
	if limit > len(sorted) {
		limit = len(sorted)
	}

	// Load node details for top results
	for _, s := range sorted[:limit] {
		nc := NodeCentrality{
			NodeID: s.id,
			Score:  s.score,
		}
		// Try to get node name and file
		var name, filePath, kind string
		err := cg.db.QueryRow("SELECT name, file_path, kind FROM nodes WHERE id = ?", s.id).Scan(&name, &filePath, &kind)
		if err == nil {
			nc.Name = name
			nc.FilePath = filePath
			nc.Kind = kind
		}
		result.Top = append(result.Top, nc)
	}

	return result, nil
}

// Community holds a cluster of nodes detected by community detection.
type Community struct {
	ID    int      `json:"id"`
	Nodes []string `json:"nodes"`
	Score float64  `json:"modularity_score"`
}

// CommunityDetectionResult holds the result of community detection.
type CommunityDetectionResult struct {
	Communities []Community `json:"communities"`
	Modularity  float64     `json:"modularity"`
}

// CommunityDetection finds communities (module boundaries) using the Louvain algorithm.
// Communities are groups of nodes that are more densely connected to each other than
// to the rest of the graph. This automatically discovers module boundaries.
//
// Algorithm: Louvain method (greedy modularity optimization).
func (cg *CodeGraph) CommunityDetection() (*CommunityDetectionResult, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	// Build adjacency with weights
	type edge struct {
		source string
		target string
		weight float64
	}

	adj := make(map[string]map[string]float64)
	var edges []edge

	rows, err := cg.db.Query("SELECT source, target, kind FROM edges WHERE kind IN ('calls', 'references', 'imports', 'extends', 'implements', 'contains')")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var src, tgt, kind string
		rows.Scan(&src, &tgt, &kind)

		// Weight by edge type
		weight := 1.0
		switch kind {
		case "contains":
			weight = 2.0
		case "extends", "implements":
			weight = 1.5
		}

		if adj[src] == nil {
			adj[src] = make(map[string]float64)
		}
		if adj[tgt] == nil {
			adj[tgt] = make(map[string]float64)
		}
		adj[src][tgt] += weight
		adj[tgt][src] += weight
		edges = append(edges, edge{src, tgt, weight})
	}

	// Collect all nodes
	allNodes := make(map[string]bool)
	for id := range adj {
		allNodes[id] = true
	}

	// Initialize: each node in its own community
	community := make(map[string]int)
	commID := 0
	for id := range allNodes {
		community[id] = commID
		commID++
	}

	// Compute total edge weight
	totalWeight := 0.0
	for _, e := range edges {
		totalWeight += e.weight
	}
	if totalWeight == 0 {
		return &CommunityDetectionResult{
			Communities: []Community{},
			Modularity:  0,
		}, nil
	}

	// Louvain iterations
	for iter := 0; iter < 10; iter++ {
		improved := false

		for node := range allNodes {
			currentComm := community[node]

			// Compute node's weighted degree
			degree := 0.0
			for _, w := range adj[node] {
				degree += w
			}

			// Try moving to each neighbor's community
			bestComm := currentComm
			bestGain := 0.0

			neighborComms := make(map[int]float64)
			for neighbor, w := range adj[node] {
				comm := community[neighbor]
				neighborComms[comm] += w
			}

			for targetComm, weightToComm := range neighborComms {
				if targetComm == currentComm {
					continue
				}

				// Compute modularity gain
				// ΔQ = [Σin + 2*ki,in] / (2*m) - [(Σtot + ki) / (2*m)]²
				// Simplified: gain proportional to internal edges minus expected
				gain := weightToComm/totalWeight - degree*communityStrength(adj, community, targetComm, allNodes)/(totalWeight*totalWeight)
				if gain > bestGain {
					bestGain = gain
					bestComm = targetComm
				}
			}

			if bestComm != currentComm {
				community[node] = bestComm
				improved = true
			}
		}

		if !improved {
			break
		}
	}

	// Merge communities (renumber to consecutive IDs)
	commMap := make(map[int]int)
	nextID := 0
	for _, c := range community {
		if _, exists := commMap[c]; !exists {
			commMap[c] = nextID
			nextID++
		}
	}

	// Build result
	commNodes := make(map[int][]string)
	for id, c := range community {
		commNodes[commMap[c]] = append(commNodes[commMap[c]], id)
	}

	result := &CommunityDetectionResult{
		Modularity: computeModularity(adj, community, totalWeight),
	}

	for id, nodes := range commNodes {
		if len(nodes) >= 2 { // Only communities with 2+ nodes
			result.Communities = append(result.Communities, Community{
				ID:    id,
				Nodes: nodes,
			})
		}
	}

	// Sort by size descending
	sort.Slice(result.Communities, func(i, j int) bool {
		return len(result.Communities[i].Nodes) > len(result.Communities[j].Nodes)
	})

	return result, nil
}

// ConnectedComponents finds isolated subsystems in the code graph.
// Each component is a set of nodes where every node is reachable from every other.
func (cg *CodeGraph) ConnectedComponents() ([][]string, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	// Build adjacency list
	adj := make(map[string][]string)
	rows, err := cg.db.Query("SELECT source, target FROM edges WHERE kind IN ('calls', 'references', 'imports', 'extends', 'implements')")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := make(map[string]bool)
	for rows.Next() {
		var src, tgt string
		rows.Scan(&src, &tgt)
		adj[src] = append(adj[src], tgt)
		adj[tgt] = append(adj[tgt], src)
		nodes[src] = true
		nodes[tgt] = true
	}

	// BFS to find components
	visited := make(map[string]bool)
	var components [][]string

	for node := range nodes {
		if visited[node] {
			continue
		}

		var component []string
		queue := []string{node}
		visited[node] = true

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			component = append(component, current)

			for _, neighbor := range adj[current] {
				if !visited[neighbor] {
					visited[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
		}

		components = append(components, component)
	}

	// Sort by size descending
	sort.Slice(components, func(i, j int) bool {
		return len(components[i]) > len(components[j])
	})

	return components, nil
}

// GraphDiff represents structural changes between two graph states.
type GraphDiff struct {
	AddedNodes   []string `json:"added_nodes"`
	RemovedNodes []string `json:"removed_nodes"`
	AddedEdges   int      `json:"added_edges"`
	RemovedEdges int      `json:"removed_edges"`
	Affected     []string `json:"affected_files"` // files whose symbols changed
}

// GraphDiff computes the structural difference between the current graph and a snapshot.
// Useful for detecting what changed after a sync.
func (cg *CodeGraph) DiffGraph(beforeNodes map[string]bool, beforeEdges map[string]bool) *GraphDiff {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	diff := &GraphDiff{}

	// Get current nodes
	currentNodes := make(map[string]bool)
	rows, _ := cg.db.Query("SELECT id, file_path FROM nodes")
	if rows != nil {
		for rows.Next() {
			var id, filePath string
			rows.Scan(&id, &filePath)
			currentNodes[id] = true
			if !beforeNodes[id] {
				diff.AddedNodes = append(diff.AddedNodes, id)
				if !contains(diff.Affected, filePath) {
					diff.Affected = append(diff.Affected, filePath)
				}
			}
		}
		rows.Close()
	}

	for id := range beforeNodes {
		if !currentNodes[id] {
			diff.RemovedNodes = append(diff.RemovedNodes, id)
		}
	}

	// Get current edges
	currentEdges := make(map[string]bool)
	rows, _ = cg.db.Query("SELECT source || '->' || target || ':' || kind FROM edges")
	if rows != nil {
		for rows.Next() {
			var edgeKey string
			rows.Scan(&edgeKey)
			currentEdges[edgeKey] = true
			if !beforeEdges[edgeKey] {
				diff.AddedEdges++
			}
		}
		rows.Close()
	}

	for edgeKey := range beforeEdges {
		if !currentEdges[edgeKey] {
			diff.RemovedEdges++
		}
	}

	return diff
}

// SnapshotGraph returns the current graph state for diffing.
func (cg *CodeGraph) SnapshotGraph() (nodes map[string]bool, edges map[string]bool, err error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	nodes = make(map[string]bool)
	edges = make(map[string]bool)

	rows, err := cg.db.Query("SELECT id FROM nodes")
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		nodes[id] = true
	}
	rows.Close()

	rows, err = cg.db.Query("SELECT source || '->' || target || ':' || kind FROM edges")
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var edgeKey string
		rows.Scan(&edgeKey)
		edges[edgeKey] = true
	}
	rows.Close()

	return nodes, edges, nil
}

// DeadCodeEntry represents a potentially dead code symbol.
type DeadCodeEntry struct {
	Node       Node    `json:"node"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// FindDeadCode uses the call graph to find symbols that are never called/referenced.
// More accurate than standalone dead code detection because it uses the full
// resolved call graph from codegraph.
func (cg *CodeGraph) FindDeadCode() ([]DeadCodeEntry, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	// Get all nodes
	rows, err := cg.db.Query(
		`SELECT id, kind, name, qualified_name, file_path, language,
		        start_line, end_line, signature, docstring, visibility, is_exported
		 FROM nodes WHERE kind IN ('function', 'method', 'class', 'interface', 'struct')`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allNodes, _ := scanNodes(rows)

	// Get all referenced node IDs
	referenced := make(map[string]bool)
	edgeRows, _ := cg.db.Query("SELECT DISTINCT target FROM edges WHERE kind IN ('calls', 'references', 'imports', 'extends', 'implements')")
	if edgeRows != nil {
		for edgeRows.Next() {
			var target string
			edgeRows.Scan(&target)
			referenced[target] = true
		}
		edgeRows.Close()
	}

	// Also mark source nodes as referenced (they're being used)
	edgeRows, _ = cg.db.Query("SELECT DISTINCT source FROM edges")
	if edgeRows != nil {
		for edgeRows.Next() {
			var source string
			edgeRows.Scan(&source)
			referenced[source] = true
		}
		edgeRows.Close()
	}

	var dead []DeadCodeEntry
	for _, n := range allNodes {
		if referenced[n.ID] {
			continue
		}

		confidence := 0.9
		reason := "no incoming references in call graph"

		// Exported symbols might be used externally
		if n.IsExported {
			confidence = 0.5
			reason = "exported but no internal references"
		}

		// Test files are often not called directly
		if strings.Contains(n.FilePath, "_test.go") || strings.Contains(n.FilePath, ".test.") {
			confidence = 0.3
			reason = "test file - may be called by test runner"
		}

		// main functions are entry points
		if n.Name == "main" || n.Name == "init" || n.Name == "TestMain" {
			continue
		}

		dead = append(dead, DeadCodeEntry{
			Node:       n,
			Confidence: confidence,
			Reason:     reason,
		})
	}

	// Sort by confidence descending
	sort.Slice(dead, func(i, j int) bool {
		return dead[i].Confidence > dead[j].Confidence
	})

	return dead, nil
}

// Helper functions

func communityStrength(adj map[string]map[string]float64, community map[string]int, targetComm int, nodes map[string]bool) float64 {
	sum := 0.0
	for node := range nodes {
		if community[node] == targetComm {
			for _, w := range adj[node] {
				sum += w
			}
		}
	}
	return sum
}

func computeModularity(adj map[string]map[string]float64, community map[string]int, totalWeight float64) float64 {
	if totalWeight == 0 {
		return 0
	}

	q := 0.0
	for i := range adj {
		for j, wij := range adj[i] {
			if community[i] == community[j] {
				ki := 0.0
				for _, w := range adj[i] {
					ki += w
				}
				kj := 0.0
				for _, w := range adj[j] {
					kj += w
				}
				q += wij - (ki*kj)/(2*totalWeight)
			}
		}
	}

	return q / (2 * totalWeight)
}

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

	rows, err := cg.db.Query("SELECT source, target FROM edges WHERE kind IN ('calls', 'references', 'imports')")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var src, tgt string
		rows.Scan(&src, &tgt)
		outlinks[src] = append(outlinks[src], tgt)
		inlinks[tgt] = append(inlinks[tgt], src)
		nodes[src] = true
		nodes[tgt] = true
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
		rows, _ := cg.db.Query(
			`SELECT source FROM edges WHERE target = ? AND kind IN ('calls', 'references', 'imports', 'extends', 'implements')`, s.id,
		)
		if rows != nil {
			for rows.Next() {
				var source string
				rows.Scan(&source)
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
		err := cg.db.QueryRow(
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
	rows, err := cg.db.Query("SELECT file_path, target FROM edges e JOIN nodes n ON n.id = e.source WHERE e.kind IN ('calls', 'references', 'imports')")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var filePath, target string
		rows.Scan(&filePath, &target)
		if fileDeps[filePath] == nil {
			fileDeps[filePath] = make(map[string]bool)
		}
		fileDeps[filePath][target] = true
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
		rows, err := cg.db.Query("SELECT id, kind, name, qualified_name, file_path, language, start_line, end_line, signature, docstring, visibility, is_exported FROM nodes")
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
		rows, err := cg.db.Query("SELECT from_node_id, reference_name, file_path, line FROM unresolved_refs")
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
