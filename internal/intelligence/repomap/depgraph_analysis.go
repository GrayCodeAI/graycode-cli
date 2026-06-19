package repomap

import "sort"

// This file holds the graph traversal algorithms (topological sort, cycle
// detection, layering, hot-path scoring) and their unlocked helpers. The graph
// type, mutators, and renderers live in depgraph.go; the builders live in
// depgraph_build.go.

// TopologicalSort returns packages in dependency order (leaves first).
// Packages with no dependencies appear first.
func (dg *DepGraph) TopologicalSort() []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	// Build adjacency list and in-degree count.
	inDegree := make(map[string]int)
	adj := make(map[string][]string)

	for id := range dg.Nodes {
		inDegree[id] = 0
	}
	for _, edge := range dg.Edges {
		if _, ok := dg.Nodes[edge.From]; !ok {
			continue
		}
		if _, ok := dg.Nodes[edge.To]; !ok {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
		inDegree[edge.To]++
	}

	// Kahn's algorithm.
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue) // deterministic order

	for len(queue) > 0 {
		sort.Strings(queue)
		node := queue[0]
		queue = queue[1:]

		for _, neighbor := range adj[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}
	// For "leaves first" (packages with no dependencies), we reverse the edge direction.

	// Re-do with reversed edges: nodes that IMPORT nothing come first.
	outDegree := make(map[string]int)
	revAdj := make(map[string][]string)
	for id := range dg.Nodes {
		outDegree[id] = 0
	}
	for _, edge := range dg.Edges {
		if _, ok := dg.Nodes[edge.From]; !ok {
			continue
		}
		if _, ok := dg.Nodes[edge.To]; !ok {
			continue
		}
		outDegree[edge.From]++
		revAdj[edge.To] = append(revAdj[edge.To], edge.From)
	}

	// Collect leaves (nodes with no outgoing edges = no imports).
	var leafQueue []string
	for id, deg := range outDegree {
		if deg == 0 {
			leafQueue = append(leafQueue, id)
		}
	}
	sort.Strings(leafQueue)

	visited := make(map[string]bool)
	var sorted []string
	for len(leafQueue) > 0 {
		sort.Strings(leafQueue)
		node := leafQueue[0]
		leafQueue = leafQueue[1:]
		if visited[node] {
			continue
		}
		visited[node] = true
		sorted = append(sorted, node)

		for _, parent := range revAdj[node] {
			// Check if all of parent's dependencies are visited.
			allVisited := true
			for _, e := range dg.Edges {
				if e.From == parent {
					if _, ok := dg.Nodes[e.To]; ok {
						if !visited[e.To] {
							allVisited = false
							break
						}
					}
				}
			}
			if allVisited && !visited[parent] {
				leafQueue = append(leafQueue, parent)
			}
		}
	}

	// Add any remaining nodes (part of cycles) at the end.
	for id := range dg.Nodes {
		if !visited[id] {
			sorted = append(sorted, id)
		}
	}

	return sorted
}

// FindCycles detects circular dependencies and returns all cycles found.
func (dg *DepGraph) FindCycles() [][]string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	// Build adjacency list.
	adj := make(map[string][]string)
	for _, edge := range dg.Edges {
		if _, ok := dg.Nodes[edge.From]; !ok {
			continue
		}
		if _, ok := dg.Nodes[edge.To]; !ok {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
	}

	// Sort adjacency lists for determinism.
	for k := range adj {
		sort.Strings(adj[k])
	}

	// Johnson's algorithm simplified: DFS-based cycle detection.
	var cycles [][]string
	visited := make(map[string]bool)
	onStack := make(map[string]bool)
	var path []string

	var dfs func(node string)
	dfs = func(node string) {
		visited[node] = true
		onStack[node] = true
		path = append(path, node)

		for _, next := range adj[node] {
			if !visited[next] {
				dfs(next)
			} else if onStack[next] {
				// Found a cycle: extract it from path.
				cycleStart := -1
				for i, p := range path {
					if p == next {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := make([]string, len(path)-cycleStart)
					copy(cycle, path[cycleStart:])
					// Check for duplicates.
					if !containsCycle(cycles, cycle) {
						cycles = append(cycles, cycle)
					}
				}
			}
		}

		path = path[:len(path)-1]
		onStack[node] = false
	}

	// Sort nodes for deterministic order.
	nodeIDs := make([]string, 0, len(dg.Nodes))
	for id := range dg.Nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)

	for _, id := range nodeIDs {
		if !visited[id] {
			dfs(id)
		}
	}

	return cycles
}

// Layers groups packages into layers based on dependency depth.
// Layer 0 contains packages with no dependencies, layer 1 depends only on layer 0, etc.
func (dg *DepGraph) Layers() [][]string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	if len(dg.Nodes) == 0 {
		return nil
	}

	// Build adjacency (from -> deps).
	deps := make(map[string][]string)
	for _, edge := range dg.Edges {
		if _, ok := dg.Nodes[edge.From]; !ok {
			continue
		}
		if _, ok := dg.Nodes[edge.To]; !ok {
			continue
		}
		deps[edge.From] = append(deps[edge.From], edge.To)
	}

	layerOf := make(map[string]int)
	var computeLayer func(id string, visiting map[string]bool) int
	computeLayer = func(id string, visiting map[string]bool) int {
		if l, ok := layerOf[id]; ok {
			return l
		}
		if visiting[id] {
			// Cycle detected, assign 0 to break it.
			return 0
		}
		visiting[id] = true

		maxDep := -1
		for _, dep := range deps[id] {
			l := computeLayer(dep, visiting)
			if l > maxDep {
				maxDep = l
			}
		}
		layerOf[id] = maxDep + 1
		delete(visiting, id)
		return maxDep + 1
	}

	for id := range dg.Nodes {
		if _, ok := layerOf[id]; !ok {
			computeLayer(id, make(map[string]bool))
		}
	}

	// Group by layer.
	maxLayer := 0
	for _, l := range layerOf {
		if l > maxLayer {
			maxLayer = l
		}
	}

	layers := make([][]string, maxLayer+1)
	for id, l := range layerOf {
		layers[l] = append(layers[l], id)
	}

	// Sort within each layer for determinism.
	for i := range layers {
		sort.Strings(layers[i])
	}

	return layers
}

// HotPaths finds the most-depended-on paths using a PageRank-like importance scoring.
// Returns paths sorted by importance (most critical first).
func (dg *DepGraph) HotPaths() [][]string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	if len(dg.Nodes) == 0 {
		return nil
	}

	// Compute PageRank-like scores.
	scores := make(map[string]float64)
	n := float64(len(dg.Nodes))
	for id := range dg.Nodes {
		scores[id] = 1.0 / n
	}

	// Build inbound edges.
	inbound := make(map[string][]string)
	outCount := make(map[string]int)
	for _, edge := range dg.Edges {
		if _, ok := dg.Nodes[edge.From]; !ok {
			continue
		}
		if _, ok := dg.Nodes[edge.To]; !ok {
			continue
		}
		inbound[edge.To] = append(inbound[edge.To], edge.From)
		outCount[edge.From]++
	}

	damping := 0.85
	for iter := 0; iter < 20; iter++ {
		newScores := make(map[string]float64)
		for id := range dg.Nodes {
			sum := 0.0
			for _, src := range inbound[id] {
				if outCount[src] > 0 {
					sum += scores[src] / float64(outCount[src])
				}
			}
			newScores[id] = (1.0-damping)/n + damping*sum
		}
		scores = newScores
	}

	// Sort nodes by score descending.
	type scoredNode struct {
		id    string
		score float64
	}
	var ranked []scoredNode
	for id, score := range scores {
		ranked = append(ranked, scoredNode{id, score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].id < ranked[j].id
		}
		return ranked[i].score > ranked[j].score
	})

	// Build paths from top-ranked nodes following their heaviest dependency chains.
	adj := make(map[string][]string)
	edgeWeight := make(map[string]int) // "from->to" => weight
	for _, edge := range dg.Edges {
		if _, ok := dg.Nodes[edge.From]; !ok {
			continue
		}
		if _, ok := dg.Nodes[edge.To]; !ok {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
		key := edge.From + "->" + edge.To
		edgeWeight[key] = edge.Weight
	}

	var paths [][]string
	used := make(map[string]bool)

	for _, rn := range ranked {
		if used[rn.id] {
			continue
		}
		if len(paths) >= 5 {
			break
		}

		// Follow the heaviest outgoing chain.
		path := []string{rn.id}
		current := rn.id
		visited := map[string]bool{current: true}

		for {
			neighbors := adj[current]
			if len(neighbors) == 0 {
				break
			}
			// Pick the heaviest edge.
			bestNeighbor := ""
			bestWeight := 0
			for _, nb := range neighbors {
				if visited[nb] {
					continue
				}
				key := current + "->" + nb
				w := edgeWeight[key]
				if w > bestWeight || (w == bestWeight && nb < bestNeighbor) {
					bestWeight = w
					bestNeighbor = nb
				}
			}
			if bestNeighbor == "" {
				break
			}
			path = append(path, bestNeighbor)
			visited[bestNeighbor] = true
			current = bestNeighbor
		}

		if len(path) > 1 {
			paths = append(paths, path)
			for _, p := range path {
				used[p] = true
			}
		}
	}

	return paths
}

// layersUnlocked computes layers without holding the lock (caller must hold RLock).
func (dg *DepGraph) layersUnlocked() [][]string {
	if len(dg.Nodes) == 0 {
		return nil
	}

	deps := make(map[string][]string)
	for _, edge := range dg.Edges {
		if _, ok := dg.Nodes[edge.From]; !ok {
			continue
		}
		if _, ok := dg.Nodes[edge.To]; !ok {
			continue
		}
		deps[edge.From] = append(deps[edge.From], edge.To)
	}

	layerOf := make(map[string]int)
	var computeLayer func(id string, visiting map[string]bool) int
	computeLayer = func(id string, visiting map[string]bool) int {
		if l, ok := layerOf[id]; ok {
			return l
		}
		if visiting[id] {
			return 0
		}
		visiting[id] = true

		maxDep := -1
		for _, dep := range deps[id] {
			l := computeLayer(dep, visiting)
			if l > maxDep {
				maxDep = l
			}
		}
		layerOf[id] = maxDep + 1
		delete(visiting, id)
		return maxDep + 1
	}

	for id := range dg.Nodes {
		if _, ok := layerOf[id]; !ok {
			computeLayer(id, make(map[string]bool))
		}
	}

	maxLayer := 0
	for _, l := range layerOf {
		if l > maxLayer {
			maxLayer = l
		}
	}

	layers := make([][]string, maxLayer+1)
	for id, l := range layerOf {
		layers[l] = append(layers[l], id)
	}
	for i := range layers {
		sort.Strings(layers[i])
	}

	return layers
}

// findCyclesUnlocked detects cycles without holding the lock (caller must hold RLock).
func (dg *DepGraph) findCyclesUnlocked() [][]string {
	adj := make(map[string][]string)
	for _, edge := range dg.Edges {
		if _, ok := dg.Nodes[edge.From]; !ok {
			continue
		}
		if _, ok := dg.Nodes[edge.To]; !ok {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
	}
	for k := range adj {
		sort.Strings(adj[k])
	}

	var cycles [][]string
	visited := make(map[string]bool)
	onStack := make(map[string]bool)
	var path []string

	var dfs func(node string)
	dfs = func(node string) {
		visited[node] = true
		onStack[node] = true
		path = append(path, node)

		for _, next := range adj[node] {
			if !visited[next] {
				dfs(next)
			} else if onStack[next] {
				cycleStart := -1
				for i, p := range path {
					if p == next {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := make([]string, len(path)-cycleStart)
					copy(cycle, path[cycleStart:])
					if !containsCycle(cycles, cycle) {
						cycles = append(cycles, cycle)
					}
				}
			}
		}

		path = path[:len(path)-1]
		onStack[node] = false
	}

	nodeIDs := make([]string, 0, len(dg.Nodes))
	for id := range dg.Nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)

	for _, id := range nodeIDs {
		if !visited[id] {
			dfs(id)
		}
	}

	return cycles
}

// containsCycle checks if cycles already contains an equivalent cycle.
func containsCycle(cycles [][]string, cycle []string) bool {
	for _, existing := range cycles {
		if len(existing) == len(cycle) {
			match := true
			for i := range existing {
				if existing[i] != cycle[i] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
