package project

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/fsutil"
)

// This file holds the quantitative project metrics gathered by ProjectAnalyzer
// (dependency count, LOC, test coverage, complexity, interface count, and
// content scans). Architecture/pattern detection lives in project_patterns.go;
// the core analyzer and report formatting live in their own files.

func (pa *ProjectAnalyzer) countDependencies() int {
	modPath := filepath.Join(pa.Dir, "go.mod")
	data, err := os.ReadFile(modPath) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return 0
	}

	count := 0
	inRequire := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "require (") {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}
		if inRequire && line != "" && !strings.HasPrefix(line, "//") {
			count++
		}
		// Single-line require.
		if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			count++
		}
	}

	return count
}

func (pa *ProjectAnalyzer) countLOC() int {
	total := 0
	_ = filepath.WalkDir(pa.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == "node_modules" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			total += countFileLines(path)
		}
		return nil
	})
	return total
}

func (pa *ProjectAnalyzer) assessTestCoverage() string {
	totalPkgs := 0
	testedPkgs := 0

	_ = filepath.WalkDir(pa.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") || base == "testdata" {
			return filepath.SkipDir
		}
		if hasGoFiles(path) {
			totalPkgs++
			if hasTestFiles(path) {
				testedPkgs++
			}
		}
		return nil
	})

	if totalPkgs == 0 {
		return "unknown"
	}

	pct := float64(testedPkgs) / float64(totalPkgs) * 100
	return fmt.Sprintf("%.0f%% (%d/%d packages have tests)", pct, testedPkgs, totalPkgs)
}

func (pa *ProjectAnalyzer) assessComplexity() string {
	totalFuncs := 0
	longFuncs := 0 // functions > 50 lines

	_ = filepath.WalkDir(pa.Dir, func(path string, d fs.DirEntry, err error) error {
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

		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				totalFuncs++
				startLine := fset.Position(fn.Pos()).Line
				endLine := fset.Position(fn.End()).Line
				if endLine-startLine > 50 {
					longFuncs++
				}
			}
		}
		return nil
	})

	if totalFuncs == 0 {
		return "unknown"
	}

	longPct := float64(longFuncs) / float64(totalFuncs) * 100
	if longPct > 20 {
		return fmt.Sprintf("high (%d/%d functions >50 lines)", longFuncs, totalFuncs)
	}
	if longPct > 10 {
		return fmt.Sprintf("moderate (%d/%d functions >50 lines)", longFuncs, totalFuncs)
	}
	return fmt.Sprintf("low (%d/%d functions >50 lines)", longFuncs, totalFuncs)
}

func (pa *ProjectAnalyzer) hasPatternInFiles(pattern string) bool {
	found := false
	_ = filepath.WalkDir(pa.Dir, func(path string, d fs.DirEntry, err error) error {
		if found || err != nil {
			return filepath.SkipAll
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
		data, readErr := fsutil.ReadPinnedFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(data), pattern) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func (pa *ProjectAnalyzer) hasPatternInTestFiles(pattern string) bool {
	found := false
	_ = filepath.WalkDir(pa.Dir, func(path string, d fs.DirEntry, err error) error {
		if found || err != nil {
			return filepath.SkipAll
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := fsutil.ReadPinnedFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(data), pattern) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func (pa *ProjectAnalyzer) countInterfaces() int {
	count := 0
	_ = filepath.WalkDir(pa.Dir, func(path string, d fs.DirEntry, err error) error {
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

		for _, decl := range f.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						if _, isIface := ts.Type.(*ast.InterfaceType); isIface && ts.Name.IsExported() {
							count++
						}
					}
				}
			}
		}
		return nil
	})
	return count
}

func countFileLines(path string) int {
	f, err := os.Open(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count
}
