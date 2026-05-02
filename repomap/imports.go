package repomap

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ImportGraph builds and queries file-level import/dependency relationships.
// When file A is identified as relevant, this finds:
// - Files that A imports (its dependencies)
// - Files that import A (its dependents)
//
// This is the cheapest cross-file signal. Research shows import graph
// traversal prevents "undefined symbol" errors by 30-40%.
type ImportGraph struct {
	// edges maps filepath -> set of imported filepaths
	edges map[string][]string
	// reverse maps filepath -> set of files that import it
	reverse map[string][]string
	// modulePath is the Go module path (e.g., "github.com/GrayCodeAI/hawk")
	modulePath string
	// root is the repository root directory
	root string
	// pkgToFiles maps Go package import paths to the files in that package
	pkgToFiles map[string][]string
}

// ── Import regex patterns per language ──

var (
	// Go: captures the import path inside quotes from import blocks and single imports
	goImportSingleRe = regexp.MustCompile(`^\s*import\s+"([^"]+)"`)
	goImportPathRe   = regexp.MustCompile(`^\s*(?:\w+\s+)?"([^"]+)"`)
	goImportBlockRe  = regexp.MustCompile(`^\s*import\s*\(`)
	goImportBlockEnd = regexp.MustCompile(`^\s*\)`)

	// Python: import X / from X import Y
	pyImportRe     = regexp.MustCompile(`^\s*import\s+([\w.]+)`)
	pyFromImportRe = regexp.MustCompile(`^\s*from\s+([\w.]+)\s+import`)

	// TypeScript/JavaScript: import ... from '...' or import '...'
	tsImportFromRe = regexp.MustCompile(`^\s*import\s+.*\s+from\s+['"]([^'"]+)['"]`)
	tsImportBareRe = regexp.MustCompile(`^\s*import\s+['"]([^'"]+)['"]`)
)

// BuildImportGraph scans source files in the given root directory and builds
// the import graph. For Go, this parses import statements and maps import
// paths to local files. Also supports basic Python and TypeScript/JavaScript.
func BuildImportGraph(root string) (*ImportGraph, error) {
	g := &ImportGraph{
		edges:      make(map[string][]string),
		reverse:    make(map[string][]string),
		root:       root,
		pkgToFiles: make(map[string][]string),
	}

	// Detect Go module path from go.mod
	g.modulePath = detectModulePath(root)

	ignoreSet := make(map[string]bool)
	for _, p := range defaultIgnorePatterns {
		ignoreSet[p] = true
	}

	// Phase 1: collect all source files and build package-to-file mapping
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if ignoreSet[base] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if !isSupportedExt(ext) {
			return nil
		}
		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		files = append(files, relPath)

		// For Go files, map the directory (package) to files
		if ext == ".go" && g.modulePath != "" {
			dir := filepath.Dir(relPath)
			pkgPath := g.modulePath
			if dir != "." {
				pkgPath = g.modulePath + "/" + filepath.ToSlash(dir)
			}
			g.pkgToFiles[pkgPath] = append(g.pkgToFiles[pkgPath], relPath)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Phase 2: parse imports for each file and resolve to local paths
	for _, relPath := range files {
		absPath := filepath.Join(root, relPath)
		ext := filepath.Ext(relPath)

		var rawImports []string
		switch ext {
		case ".go":
			rawImports = parseGoImports(absPath)
		case ".py":
			rawImports = parsePythonImports(absPath)
		case ".ts", ".tsx", ".js", ".jsx":
			rawImports = parseTSImports(absPath)
		}

		var resolved []string
		for _, imp := range rawImports {
			targets := g.resolveImport(relPath, imp, ext)
			resolved = append(resolved, targets...)
		}

		// Deduplicate and exclude self-references
		resolved = dedup(resolved)
		var filtered []string
		for _, r := range resolved {
			if r != relPath {
				filtered = append(filtered, r)
			}
		}

		if len(filtered) > 0 {
			g.edges[relPath] = filtered
		}
	}

	// Phase 3: build reverse edges
	for src, deps := range g.edges {
		for _, dep := range deps {
			g.reverse[dep] = append(g.reverse[dep], src)
		}
	}

	// Deduplicate reverse edges
	for k, v := range g.reverse {
		g.reverse[k] = dedup(v)
	}

	return g, nil
}

// DependenciesOf returns files that the given file imports (up to maxDepth).
func (g *ImportGraph) DependenciesOf(filePath string, maxDepth int) []string {
	if maxDepth <= 0 {
		maxDepth = 1
	}
	return g.bfs(filePath, maxDepth, g.edges)
}

// DependentsOf returns files that import the given file (up to maxDepth).
func (g *ImportGraph) DependentsOf(filePath string, maxDepth int) []string {
	if maxDepth <= 0 {
		maxDepth = 1
	}
	return g.bfs(filePath, maxDepth, g.reverse)
}

// ImpactSet returns the union of dependencies and dependents for a set of files.
// This is used for change-set-aware context: "what other files matter given these changes?"
func (g *ImportGraph) ImpactSet(files []string, maxDepth int) []string {
	if maxDepth <= 0 {
		maxDepth = 1
	}

	seen := make(map[string]bool)
	inputSet := make(map[string]bool)
	for _, f := range files {
		inputSet[f] = true
	}

	for _, f := range files {
		for _, dep := range g.DependenciesOf(f, maxDepth) {
			if !inputSet[dep] {
				seen[dep] = true
			}
		}
		for _, dep := range g.DependentsOf(f, maxDepth) {
			if !inputSet[dep] {
				seen[dep] = true
			}
		}
	}

	result := make([]string, 0, len(seen))
	for f := range seen {
		result = append(result, f)
	}
	sort.Strings(result)
	return result
}

// Edges returns the forward edge map (file -> imports). Useful for inspection.
func (g *ImportGraph) Edges() map[string][]string {
	return g.edges
}

// Reverse returns the reverse edge map (file -> dependents). Useful for inspection.
func (g *ImportGraph) Reverse() map[string][]string {
	return g.reverse
}

// ── BFS traversal ──

func (g *ImportGraph) bfs(start string, maxDepth int, adj map[string][]string) []string {
	visited := make(map[string]bool)
	visited[start] = true

	type item struct {
		path  string
		depth int
	}
	queue := []item{{path: start, depth: 0}}

	var result []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if cur.depth >= maxDepth {
			continue
		}

		for _, next := range adj[cur.path] {
			if !visited[next] {
				visited[next] = true
				result = append(result, next)
				queue = append(queue, item{path: next, depth: cur.depth + 1})
			}
		}
	}

	sort.Strings(result)
	return result
}

// ── Import resolution ──

// resolveImport maps a raw import string to local file paths relative to root.
func (g *ImportGraph) resolveImport(fromFile, imp, ext string) []string {
	switch ext {
	case ".go":
		return g.resolveGoImport(imp)
	case ".py":
		return g.resolvePythonImport(fromFile, imp)
	case ".ts", ".tsx", ".js", ".jsx":
		return g.resolveTSImport(fromFile, imp)
	}
	return nil
}

// resolveGoImport checks if a Go import path is within our module and maps
// it to the local files in that package.
func (g *ImportGraph) resolveGoImport(imp string) []string {
	if g.modulePath == "" {
		return nil
	}
	if !strings.HasPrefix(imp, g.modulePath) {
		return nil // external dependency
	}

	// Look up which files belong to this package
	if files, ok := g.pkgToFiles[imp]; ok {
		result := make([]string, len(files))
		copy(result, files)
		return result
	}
	return nil
}

// resolvePythonImport tries to resolve a Python dotted module path to a local file.
func (g *ImportGraph) resolvePythonImport(fromFile, imp string) []string {
	// Convert dotted path to directory path: "pkg.sub.module" -> "pkg/sub/module"
	parts := strings.Split(imp, ".")
	candidates := []string{
		filepath.Join(parts...) + ".py",                          // pkg/sub/module.py
		filepath.Join(append(parts, "__init__")...) + ".py",      // pkg/sub/module/__init__.py
	}

	var result []string
	for _, c := range candidates {
		absPath := filepath.Join(g.root, c)
		if _, err := os.Stat(absPath); err == nil {
			result = append(result, c)
		}
	}
	return result
}

// resolveTSImport resolves a TypeScript/JavaScript relative import to a local file.
func (g *ImportGraph) resolveTSImport(fromFile, imp string) []string {
	// Only resolve relative imports (starting with . or ..)
	if !strings.HasPrefix(imp, ".") {
		return nil
	}

	fromDir := filepath.Dir(fromFile)
	resolved := filepath.Join(fromDir, imp)
	resolved = filepath.Clean(resolved)

	// Try various extensions
	extensions := []string{"", ".ts", ".tsx", ".js", ".jsx", "/index.ts", "/index.tsx", "/index.js", "/index.jsx"}
	var result []string
	for _, ext := range extensions {
		candidate := resolved + ext
		absPath := filepath.Join(g.root, candidate)
		if info, err := os.Stat(absPath); err == nil && !info.IsDir() {
			result = append(result, candidate)
			break // take the first match
		}
	}
	return result
}

// ── Import parsing per language ──

// parseGoImports extracts import paths from a Go source file.
func parseGoImports(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var imports []string
	scanner := bufio.NewScanner(f)
	inBlock := false

	for scanner.Scan() {
		line := scanner.Text()

		if inBlock {
			if goImportBlockEnd.MatchString(line) {
				inBlock = false
				continue
			}
			if m := goImportPathRe.FindStringSubmatch(line); m != nil {
				imports = append(imports, m[1])
			}
			continue
		}

		if goImportBlockRe.MatchString(line) {
			inBlock = true
			continue
		}

		if m := goImportSingleRe.FindStringSubmatch(line); m != nil {
			imports = append(imports, m[1])
		}
	}

	return imports
}

// parsePythonImports extracts imported module names from a Python file.
func parsePythonImports(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var imports []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if m := pyFromImportRe.FindStringSubmatch(line); m != nil {
			imports = append(imports, m[1])
		} else if m := pyImportRe.FindStringSubmatch(line); m != nil {
			imports = append(imports, m[1])
		}
	}
	return imports
}

// parseTSImports extracts import paths from a TypeScript/JavaScript file.
func parseTSImports(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var imports []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if m := tsImportFromRe.FindStringSubmatch(line); m != nil {
			imports = append(imports, m[1])
		} else if m := tsImportBareRe.FindStringSubmatch(line); m != nil {
			imports = append(imports, m[1])
		}
	}
	return imports
}

// ── Helpers ──

// detectModulePath reads the module path from go.mod in the given directory.
func detectModulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

// dedup returns a sorted, deduplicated copy of the slice.
func dedup(ss []string) []string {
	if len(ss) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(ss))
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	sort.Strings(result)
	return result
}
