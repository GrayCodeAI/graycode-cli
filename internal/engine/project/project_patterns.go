package project

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// This file holds architecture and design-pattern detection (DetectArchitecture,
// DetectPatterns and their scanners) plus the small structural helpers they
// share. Quantitative metrics live in project_metrics.go.

// DetectArchitecture determines the architectural style of a project by examining
// its directory structure.
func DetectArchitecture(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "unknown"
	}

	dirs := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			dirs[entry.Name()] = true
		}
	}

	// Hexagonal: domain/ + ports/ + adapters/
	if dirs["domain"] && dirs["ports"] && dirs["adapters"] {
		return "hexagonal"
	}

	// Microservices: multiple service directories.
	serviceCount := 0
	for name := range dirs {
		if strings.HasSuffix(name, "-service") || strings.HasSuffix(name, "-svc") {
			serviceCount++
		}
	}
	if dirs["services"] || serviceCount >= 2 {
		return "microservices"
	}

	// Layered: cmd/ -> service/ or internal/ -> repository/ or repo/
	if dirs["cmd"] && (dirs["service"] || dirs["internal"] || dirs["engine"]) {
		if dirs["repo"] || dirs["repository"] || dirs["store"] || dirs["tool"] {
			return "layered"
		}
		return "layered"
	}

	// Modular: feature-based directories (more than 4 sibling directories with similar structure).
	featureDirs := 0
	for name := range dirs {
		subPath := filepath.Join(dir, name)
		if hasGoFiles(subPath) {
			featureDirs++
		}
	}
	if featureDirs >= 5 && !dirs["cmd"] {
		return "modular"
	}

	// Monolith: single main package.
	if hasMainPackage(dir) && featureDirs <= 2 {
		return "monolith"
	}

	// Default to modular if there are many subdirectories.
	if featureDirs >= 4 {
		return "modular"
	}

	return "monolith"
}

// DetectPatterns identifies design patterns used in the codebase.
func DetectPatterns(dir string) []Pattern {
	var patterns []Pattern

	// Repository pattern: *Repository interfaces + implementations.
	repoFiles := findFilesWithPattern(dir, "repository", "repo")
	if len(repoFiles) > 0 {
		patterns = append(patterns, Pattern{
			Name:        "Repository",
			Description: "Data access abstracted behind repository interfaces",
			Files:       repoFiles,
			Confidence:  calculateConfidence(repoFiles, 2),
		})
	}

	// Middleware pattern: handler wrappers, interceptors.
	middlewareFiles := findFilesWithPattern(dir, "middleware", "interceptor")
	if len(middlewareFiles) > 0 {
		patterns = append(patterns, Pattern{
			Name:        "Middleware",
			Description: "Request/response processing chain with handler wrappers",
			Files:       middlewareFiles,
			Confidence:  calculateConfidence(middlewareFiles, 2),
		})
	}

	// Factory pattern: New* constructors.
	factoryFiles := findFactoryPattern(dir)
	if len(factoryFiles) > 0 {
		patterns = append(patterns, Pattern{
			Name:        "Factory",
			Description: "Object creation via New* constructor functions",
			Files:       factoryFiles,
			Confidence:  calculateConfidence(factoryFiles, 5),
		})
	}

	// Observer pattern: event/listener files.
	observerFiles := findFilesWithPattern(dir, "event", "listener", "observer", "hook")
	if len(observerFiles) > 0 {
		patterns = append(patterns, Pattern{
			Name:        "Observer",
			Description: "Event-driven communication with listeners/hooks",
			Files:       observerFiles,
			Confidence:  calculateConfidence(observerFiles, 2),
		})
	}

	// Strategy pattern: interface + multiple implementations.
	strategyFiles := findStrategyPattern(dir)
	if len(strategyFiles) > 0 {
		patterns = append(patterns, Pattern{
			Name:        "Strategy",
			Description: "Interface with multiple interchangeable implementations",
			Files:       strategyFiles,
			Confidence:  calculateConfidence(strategyFiles, 3),
		})
	}

	// Interface-driven tools pattern.
	toolFiles := findFilesWithPattern(dir, "tool")
	if len(toolFiles) >= 3 {
		patterns = append(patterns, Pattern{
			Name:        "Interface-driven tools",
			Description: "Tool interface with multiple implementations",
			Files:       toolFiles,
			Confidence:  calculateConfidence(toolFiles, 5),
		})
	}

	// Functional options pattern (WithXxx).
	optionFiles := findFunctionalOptionsPattern(dir)
	if len(optionFiles) > 0 {
		patterns = append(patterns, Pattern{
			Name:        "Functional Options",
			Description: "Configuration via WithXxx option functions",
			Files:       optionFiles,
			Confidence:  calculateConfidence(optionFiles, 3),
		})
	}

	return patterns
}

func hasGoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			return true
		}
	}
	return false
}

func hasMainPackage(dir string) bool {
	mainFile := filepath.Join(dir, "main.go")
	_, err := os.Stat(mainFile)
	return err == nil
}

func findFilesWithPattern(dir string, patterns ...string) []string {
	var files []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		lower := strings.ToLower(filepath.Base(path))
		for _, pattern := range patterns {
			if strings.Contains(lower, pattern) {
				relPath, relErr := filepath.Rel(dir, path)
				if relErr == nil {
					files = append(files, relPath)
				}
				break
			}
		}
		return nil
	})
	return files
}

func findFactoryPattern(dir string) []string {
	var files []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		newCount := 0
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if strings.HasPrefix(fn.Name.Name, "New") && fn.Name.IsExported() {
					newCount++
				}
			}
		}

		if newCount >= 2 {
			relPath, relErr := filepath.Rel(dir, path)
			if relErr == nil {
				files = append(files, relPath)
			}
		}
		return nil
	})

	// Limit results.
	if len(files) > 10 {
		files = files[:10]
	}
	return files
}

func findStrategyPattern(dir string) []string {
	// Look for files that define an interface and have sibling files implementing it.
	var files []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		// Check if the file defines interfaces with multiple methods.
		for _, decl := range f.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						if iface, isIface := ts.Type.(*ast.InterfaceType); isIface {
							if iface.Methods != nil && len(iface.Methods.List) >= 2 {
								relPath, relErr := filepath.Rel(dir, path)
								if relErr == nil {
									files = append(files, relPath)
								}
							}
						}
					}
				}
			}
		}
		return nil
	})

	if len(files) > 10 {
		files = files[:10]
	}
	return files
}

func findFunctionalOptionsPattern(dir string) []string {
	var files []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		withCount := 0
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if strings.HasPrefix(fn.Name.Name, "With") && fn.Name.IsExported() {
					withCount++
				}
			}
		}

		if withCount >= 3 {
			relPath, relErr := filepath.Rel(dir, path)
			if relErr == nil {
				files = append(files, relPath)
			}
		}
		return nil
	})

	if len(files) > 10 {
		files = files[:10]
	}
	return files
}

func calculateConfidence(files []string, threshold int) float64 {
	count := len(files)
	if count >= threshold*2 {
		return 0.95
	}
	if count >= threshold {
		return 0.8
	}
	if count >= 1 {
		return 0.5 + float64(count)/float64(threshold)*0.3
	}
	return 0.0
}
