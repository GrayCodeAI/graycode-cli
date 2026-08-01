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
	"strings"
)

// This file holds the graph builders (Go via go.mod + go/parser, JS/TS via
// package.json + import regexes) and their parsing helpers. The graph type and
// renderers live in depgraph.go; traversal algorithms live in
// depgraph_analysis.go.

// BuildFromGoMod reads go.mod and scans .go files to build the dependency graph.
func (dg *DepGraph) BuildFromGoMod(projectDir string) error {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	goModPath := filepath.Join(projectDir, "go.mod")
	modData, err := os.ReadFile(goModPath) // #nosec G304 -- goModPath is the go.mod of the project directory being analyzed by this dev CLI
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
	data, err := os.ReadFile(pkgJSONPath) // #nosec G304 -- pkgJSONPath is the package.json of the project directory being analyzed by this dev CLI
	if err != nil {
		return fmt.Errorf("depgraph: read package.json: %w", err)
	}

	var pkgJSON struct {
		Name            string            `json:"name"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if unmarshalErr := json.Unmarshal(data, &pkgJSON); unmarshalErr != nil {
		return fmt.Errorf("depgraph: parse package.json: %w", unmarshalErr)
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
		content, readErr := os.ReadFile(path) // #nosec G304,G122 -- read-only repository analysis
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
	f, err := os.Open(path) // #nosec G304 -- path is a repo file discovered while walking the project directory being analyzed by this dev CLI
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
