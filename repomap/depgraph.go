package repomap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
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

// BuildFromGoMod reads go.mod and scans .go files to build the dependency graph.
func (dg *DepGraph) BuildFromGoMod(projectDir string) error {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	goModPath := filepath.Join(projectDir, "go.mod")
	modData, err := os.ReadFile(goModPath)
	if err != nil {
		return fmt.Errorf("depgraph: read go.mod: %w", err)
	}

	moduleName := parseModuleName(string(modData))
	if moduleName == "" {
		return fmt.Errorf("depgraph: cannot determine module name from go.mod")
	}
	dg.Root = moduleName

	// Parse external dependencies from go.mod require blocks.
	externalDeps := parseGoModRequires(string(modData))

	// Add external dependency nodes.
	for _, dep := range externalDeps {
		shortName := filepath.Base(dep)
		dg.Nodes[dep] = &DepNode{
			ID:         dep,
			Name:       shortName,
			Type:       "external",
			ImportedBy: []string{},
			Imports:    []string{},
		}
	}

	// Scan all .go files to collect imports and build internal packages.
	internalPkgs := make(map[string]*DepNode)
	// pkgImports maps each internal package path to a set of import paths.
	pkgImports := make(map[string]map[string]bool)

	fset := token.NewFileSet()
	err = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip test files for dependency analysis.
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return nil
		}

		relDir, _ := filepath.Rel(projectDir, filepath.Dir(path))
		if relDir == "" || relDir == "." {
			relDir = ""
		}
		var pkgPath string
		if relDir == "" {
			pkgPath = moduleName
		} else {
			pkgPath = moduleName + "/" + filepath.ToSlash(relDir)
		}

		if _, ok := internalPkgs[pkgPath]; !ok {
			shortName := filepath.Base(pkgPath)
			if pkgPath == moduleName {
				shortName = filepath.Base(moduleName)
			}
			internalPkgs[pkgPath] = &DepNode{
				ID:         pkgPath,
				Name:       shortName,
				Type:       "internal",
				FileCount:  0,
				LOC:        0,
				ImportedBy: []string{},
				Imports:    []string{},
			}
			pkgImports[pkgPath] = make(map[string]bool)
		}

		internalPkgs[pkgPath].FileCount++

		// Count LOC.
		loc := countFileLOC(path)
		internalPkgs[pkgPath].LOC += loc

		// Collect imports.
		for _, imp := range f.Imports {
			impPath := strings.Trim(imp.Path.Value, `"`)
			pkgImports[pkgPath][impPath] = true
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("depgraph: walk project: %w", err)
	}

	// Add internal package nodes.
	for id, node := range internalPkgs {
		dg.Nodes[id] = node
	}

	// Process imports and create edges.
	for pkgPath, imports := range pkgImports {
		for imp := range imports {
			impType := classifyImport(imp, moduleName, externalDeps)

			// Ensure stdlib nodes exist.
			if impType == "stdlib" {
				if _, ok := dg.Nodes[imp]; !ok {
					dg.Nodes[imp] = &DepNode{
						ID:         imp,
						Name:       filepath.Base(imp),
						Type:       "stdlib",
						ImportedBy: []string{},
						Imports:    []string{},
					}
				}
			}

			// Record the import relationship.
			if node, ok := dg.Nodes[pkgPath]; ok {
				node.Imports = appendUniqueStr(node.Imports, imp)
			}
			if node, ok := dg.Nodes[imp]; ok {
				node.ImportedBy = appendUniqueStr(node.ImportedBy, pkgPath)
			}

			// Add edge.
			found := false
			for i, e := range dg.Edges {
				if e.From == pkgPath && e.To == imp {
					dg.Edges[i].Weight++
					found = true
					break
				}
			}
			if !found {
				dg.Edges = append(dg.Edges, DepEdge{
					From:   pkgPath,
					To:     imp,
					Weight: 1,
				})
			}
		}
	}

	return nil
}

// BuildFromPackageJSON reads package.json and scans JS/TS files to build the
// dependency graph.
func (dg *DepGraph) BuildFromPackageJSON(projectDir string) error {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	pkgJSONPath := filepath.Join(projectDir, "package.json")
	data, err := os.ReadFile(pkgJSONPath)
	if err != nil {
		return fmt.Errorf("depgraph: read package.json: %w", err)
	}

	var pkgJSON struct {
		Name            string            `json:"name"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkgJSON); err != nil {
		return fmt.Errorf("depgraph: parse package.json: %w", err)
	}

	dg.Root = pkgJSON.Name

	// Add the root package node.
	dg.Nodes[pkgJSON.Name] = &DepNode{
		ID:         pkgJSON.Name,
		Name:       pkgJSON.Name,
		Type:       "internal",
		ImportedBy: []string{},
		Imports:    []string{},
	}

	// Collect all declared dependencies.
	allDeps := make(map[string]bool)
	for dep := range pkgJSON.Dependencies {
		allDeps[dep] = true
		dg.Nodes[dep] = &DepNode{
			ID:         dep,
			Name:       dep,
			Type:       "external",
			ImportedBy: []string{},
			Imports:    []string{},
		}
	}
	for dep := range pkgJSON.DevDependencies {
		allDeps[dep] = true
		if _, ok := dg.Nodes[dep]; !ok {
			dg.Nodes[dep] = &DepNode{
				ID:         dep,
				Name:       dep,
				Type:       "external",
				ImportedBy: []string{},
				Imports:    []string{},
			}
		}
	}

	// Scan JS/TS files for imports.
	jsImportRe := regexp.MustCompile(`(?:import\s+.*?\s+from\s+['"]([^'"]+)['"]|require\s*\(\s*['"]([^'"]+)['"]\s*\))`)

	// Internal modules map (relative imports).
	internalModules := make(map[string]*DepNode)

	err = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "node_modules" || base == ".git" || base == "dist" || base == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".js" && ext != ".ts" && ext != ".jsx" && ext != ".tsx" {
			return nil
		}

		relPath, _ := filepath.Rel(projectDir, path)
		relPath = filepath.ToSlash(relPath)

		// Determine the "module" path for this file.
		modPath := pkgJSON.Name + "/" + relPath

		if _, ok := internalModules[modPath]; !ok {
			internalModules[modPath] = &DepNode{
				ID:         modPath,
				Name:       filepath.Base(relPath),
				Type:       "internal",
				FileCount:  1,
				LOC:        0,
				ImportedBy: []string{},
				Imports:    []string{},
			}
		}
		internalModules[modPath].LOC += countFileLOC(path)

		// Read file and find imports.
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		matches := jsImportRe.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			imported := match[1]
			if imported == "" {
				imported = match[2]
			}
			if imported == "" {
				continue
			}

			var targetID string
			if strings.HasPrefix(imported, ".") {
				// Relative import => internal.
				dir := filepath.Dir(relPath)
				resolved := filepath.ToSlash(filepath.Join(dir, imported))
				targetID = pkgJSON.Name + "/" + resolved
				if _, ok := internalModules[targetID]; !ok {
					internalModules[targetID] = &DepNode{
						ID:         targetID,
						Name:       filepath.Base(imported),
						Type:       "internal",
						ImportedBy: []string{},
						Imports:    []string{},
					}
				}
			} else {
				// Package import => external.
				// Extract package name (handle scoped packages).
				pkgName := imported
				if strings.HasPrefix(imported, "@") {
					parts := strings.SplitN(imported, "/", 3)
					if len(parts) >= 2 {
						pkgName = parts[0] + "/" + parts[1]
					}
				} else {
					parts := strings.SplitN(imported, "/", 2)
					pkgName = parts[0]
				}
				targetID = pkgName
				if _, ok := dg.Nodes[targetID]; !ok {
					nodeType := "external"
					if isNodeBuiltin(pkgName) {
						nodeType = "stdlib"
					}
					dg.Nodes[targetID] = &DepNode{
						ID:         targetID,
						Name:       pkgName,
						Type:       nodeType,
						ImportedBy: []string{},
						Imports:    []string{},
					}
				}
			}

			internalModules[modPath].Imports = appendUniqueStr(internalModules[modPath].Imports, targetID)
			if node, ok := dg.Nodes[targetID]; ok {
				node.ImportedBy = appendUniqueStr(node.ImportedBy, modPath)
			} else if mod, ok := internalModules[targetID]; ok {
				mod.ImportedBy = appendUniqueStr(mod.ImportedBy, modPath)
			}

			// Add edge.
			found := false
			for i, e := range dg.Edges {
				if e.From == modPath && e.To == targetID {
					dg.Edges[i].Weight++
					found = true
					break
				}
			}
			if !found {
				dg.Edges = append(dg.Edges, DepEdge{
					From:   modPath,
					To:     targetID,
					Weight: 1,
				})
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("depgraph: walk project: %w", err)
	}

	// Merge internal modules into the graph.
	for id, node := range internalModules {
		dg.Nodes[id] = node
	}

	return nil
}

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

// --- Internal helpers ---

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

// shortName returns the short display name for a node.
func (dg *DepGraph) shortName(id string) string {
	if node, ok := dg.Nodes[id]; ok && node.Name != "" {
		return node.Name
	}
	return filepath.Base(id)
}

// parseModuleName extracts the module name from go.mod content.
func parseModuleName(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

// parseGoModRequires extracts dependency paths from go.mod require blocks.
func parseGoModRequires(content string) []string {
	var deps []string
	inRequire := false
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "require (") || line == "require (" {
			inRequire = true
			continue
		}
		if inRequire {
			if line == ")" {
				inRequire = false
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				deps = append(deps, parts[0])
			}
		}
		// Single-line require.
		if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				deps = append(deps, parts[1])
			}
		}
	}
	return deps
}

// classifyImport determines the type of an import path.
func classifyImport(importPath, moduleName string, externalDeps []string) string {
	// Internal: starts with module name.
	if strings.HasPrefix(importPath, moduleName) {
		return "internal"
	}

	// External: matches a known dependency.
	for _, dep := range externalDeps {
		if strings.HasPrefix(importPath, dep) {
			return "external"
		}
	}

	// If it contains a dot in the first path component, it's likely external.
	firstComponent := strings.SplitN(importPath, "/", 2)[0]
	if strings.Contains(firstComponent, ".") {
		return "external"
	}

	return "stdlib"
}

// countFileLOC counts lines of code in a file (non-blank lines).
func countFileLOC(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			count++
		}
	}
	return count
}

// appendUniqueStr is defined in callgraph.go — reused here.

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

// isNodeBuiltin checks if a package name is a Node.js built-in module.
func isNodeBuiltin(name string) bool {
	builtins := map[string]bool{
		"fs": true, "path": true, "os": true, "http": true, "https": true,
		"crypto": true, "util": true, "events": true, "stream": true,
		"child_process": true, "net": true, "url": true, "querystring": true,
		"buffer": true, "assert": true, "cluster": true, "dns": true,
		"readline": true, "tls": true, "zlib": true, "vm": true,
		"process": true, "module": true, "console": true, "timers": true,
	}
	// Also handle "node:" prefix.
	if strings.HasPrefix(name, "node:") {
		return true
	}
	return builtins[name]
}
