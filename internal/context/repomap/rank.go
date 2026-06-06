package repomap

import (
	"path"
	"strings"
)

// linkEdges resolves each file's raw imports into edges to other scanned files.
// Both Go module-path imports and relative file references are handled.
func (g *Graph) linkEdges() {
	// Index Go packages by their directory so module-path imports can be mapped
	// back to scanned files. We approximate the module path by the directory
	// suffix: any import path whose tail matches a scanned directory links to
	// every Go file in that directory.
	dirFiles := make(map[string][]string)
	for rel, n := range g.Nodes {
		dir := path.Dir(rel)
		if n.Lang == "go" {
			dirFiles[dir] = append(dirFiles[dir], rel)
		}
	}

	for src, n := range g.Nodes {
		for _, imp := range n.Imports {
			for _, dst := range g.resolveImport(src, imp, dirFiles) {
				if _, ok := g.Nodes[dst]; ok {
					g.addEdge(src, dst, 1.0)
				}
			}
		}
	}
}

// resolveImport maps a single import target to candidate destination file paths
// within the scanned tree. Unresolvable (external) imports yield nothing.
func (g *Graph) resolveImport(src, imp string, dirFiles map[string][]string) []string {
	// Relative file reference (JS/TS/Python-relative/C include).
	if strings.HasPrefix(imp, ".") || isRelativeRef(imp) {
		base := path.Dir(src)
		joined := normalizePath(path.Clean(path.Join(base, imp)))
		var out []string
		// Try the path directly and with common source extensions.
		for _, cand := range candidateFiles(joined) {
			if _, ok := g.Nodes[cand]; ok {
				out = append(out, cand)
			}
		}
		return out
	}

	// Module path (Go module path, dotted Python/Java package, slash package).
	// Match against scanned directories by longest directory suffix.
	impSlash := strings.ReplaceAll(imp, ".", "/")
	var best string
	var bestFiles []string
	for dir, files := range dirFiles {
		if dir == "." {
			continue
		}
		if (strings.HasSuffix(impSlash, "/"+dir) || impSlash == dir ||
			strings.HasSuffix(imp, "/"+dir) || imp == dir) && len(dir) > len(best) {
			best = dir
			bestFiles = files
		}
	}
	return bestFiles
}

// candidateFiles expands a path stem into likely file paths.
func candidateFiles(stem string) []string {
	exts := []string{"", ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".rs", ".java", ".rb", ".c", ".h", ".cc", ".cpp", ".hpp"}
	out := make([]string, 0, len(exts)+1)
	for _, e := range exts {
		out = append(out, stem+e)
	}
	// index files (e.g. JS package dirs)
	out = append(out, normalizePath(path.Join(stem, "index.js")), normalizePath(path.Join(stem, "index.ts")))
	return out
}

// isRelativeRef reports whether an import string looks like a direct file
// reference (C/JS include or path with an extension) rather than a package or
// module path. Leading "." relatives are handled separately by the caller.
func isRelativeRef(imp string) bool {
	if !strings.Contains(imp, "/") {
		return false
	}
	ext := path.Ext(imp)
	return ext != "" && detectLang("x"+ext) != ""
}

// Rank runs a PageRank-like pass over the graph, filling each node's Rank.
//
// It uses the standard damped iteration with uniform teleportation. Nodes with
// no outgoing edges distribute their mass uniformly (dangling handling). The
// result is stable and deterministic.
func (g *Graph) Rank() {
	const (
		damping = 0.85
		iters   = 30
		eps     = 1e-9
	)
	n := len(g.Nodes)
	if n == 0 {
		return
	}

	rank := make(map[string]float64, n)
	out := make(map[string]float64, n)
	init := 1.0 / float64(n)
	for k := range g.Nodes {
		rank[k] = init
		for _, w := range g.edges[k] {
			out[k] += w
		}
	}

	base := (1.0 - damping) / float64(n)
	for it := 0; it < iters; it++ {
		next := make(map[string]float64, n)
		var dangling float64
		for k, r := range rank {
			if out[k] == 0 {
				dangling += r
			}
		}
		danglingShare := damping * dangling / float64(n)
		for k := range g.Nodes {
			next[k] = base + danglingShare
		}
		for src, r := range rank {
			ow := out[src]
			if ow == 0 {
				continue
			}
			for dst, w := range g.edges[src] {
				next[dst] += damping * r * (w / ow)
			}
		}
		var delta float64
		for k := range next {
			delta += abs(next[k] - rank[k])
		}
		rank = next
		if delta < eps {
			break
		}
	}

	for k, n := range g.Nodes {
		n.Rank = rank[k]
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
