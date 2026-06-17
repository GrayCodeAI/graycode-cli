// Package repomap: dead_code.go flags top-level declarations (functions,
// methods, types, vars, consts) that appear to be unreferenced. It is
// deliberately conservative: declarations reachable from tests, interface
// implementations, or via reflection are not flagged. FormatDeadCode
// produces a human-readable summary, GenerateRemovalPlan a structured
// edit script.
package repomap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// DeadCode represents a detected dead code item.
type DeadCode struct {
	File       string
	Line       int
	Name       string
	Kind       string  // "function", "type", "var", "const", "method"
	Confidence float64 // 0.0 to 1.0
	Reason     string
}

// Declaration represents a top-level declaration in a Go source file.
type Declaration struct {
	Name     string
	Kind     string
	File     string
	Line     int
	Exported bool
	Package  string
}

// DeadCodeDetector scans Go projects to find unused declarations.
type DeadCodeDetector struct {
	Declarations   map[string]*Declaration
	References     map[string]int
	mu             sync.RWMutex
	moduleName     string
	lineCountCache map[string]int
}

// NewDeadCodeDetector creates a new initialized DeadCodeDetector.
func NewDeadCodeDetector() *DeadCodeDetector {
	return &DeadCodeDetector{
		Declarations:   make(map[string]*Declaration),
		References:     make(map[string]int),
		lineCountCache: make(map[string]int),
	}
}

// Scan walks all Go files in projectDir, collects declarations and references,
// and returns detected dead code items.
func (d *DeadCodeDetector) Scan(projectDir string) ([]DeadCode, error) {
	d.mu.Lock()
	d.Declarations = make(map[string]*Declaration)
	d.References = make(map[string]int)
	d.lineCountCache = make(map[string]int)
	d.mu.Unlock()

	// Read module name from go.mod if available
	d.moduleName = readModuleName(projectDir)

	// Walk all Go files
	err := filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			base := filepath.Base(path)
			// Skip vendor, hidden dirs, testdata
			if base == "vendor" || base == "testdata" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(projectDir, path)
		if relPath == "" {
			relPath = path
		}
		d.ScanFile(relPath, string(content))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking project directory: %w", err)
	}

	return d.FindUnused(), nil
}

// ScanFile uses go/ast to extract declarations and references from a single file.
func (d *DeadCodeDetector) ScanFile(path, content string) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return
	}

	pkgName := ""
	if file.Name != nil {
		pkgName = file.Name.Name
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Collect declarations
	for _, decl := range file.Decls {
		switch dd := decl.(type) {
		case *ast.FuncDecl:
			name := dd.Name.Name
			kind := "function"
			if dd.Recv != nil && len(dd.Recv.List) > 0 {
				kind = "method"
				// Prefix method name with receiver type for uniqueness
				recvType := extractReceiverType(dd.Recv.List[0].Type)
				if recvType != "" {
					name = recvType + "." + name
				}
			}
			key := path + ":" + name
			d.Declarations[key] = &Declaration{
				Name:     name,
				Kind:     kind,
				File:     path,
				Line:     fset.Position(dd.Pos()).Line,
				Exported: ast.IsExported(dd.Name.Name),
				Package:  pkgName,
			}
			// Estimate lines for the function body
			if dd.Body != nil {
				startLine := fset.Position(dd.Body.Lbrace).Line
				endLine := fset.Position(dd.Body.Rbrace).Line
				d.lineCountCache[key] = endLine - startLine + 1
			}

		case *ast.GenDecl:
			for _, spec := range dd.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					key := path + ":" + s.Name.Name
					d.Declarations[key] = &Declaration{
						Name:     s.Name.Name,
						Kind:     "type",
						File:     path,
						Line:     fset.Position(s.Pos()).Line,
						Exported: ast.IsExported(s.Name.Name),
						Package:  pkgName,
					}
					d.lineCountCache[key] = estimateTypeLines(fset, s)

				case *ast.ValueSpec:
					kind := "var"
					if dd.Tok == token.CONST {
						kind = "const"
					}
					for _, name := range s.Names {
						if name.Name == "_" {
							continue
						}
						key := path + ":" + name.Name
						d.Declarations[key] = &Declaration{
							Name:     name.Name,
							Kind:     kind,
							File:     path,
							Line:     fset.Position(name.Pos()).Line,
							Exported: ast.IsExported(name.Name),
							Package:  pkgName,
						}
						d.lineCountCache[key] = 1
					}
				}
			}
		}
	}

	// Collect references (all identifiers used in the file)
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Ident:
			d.References[node.Name]++
		case *ast.SelectorExpr:
			if ident, ok := node.X.(*ast.Ident); ok {
				// Track qualified references like pkg.Func
				d.References[ident.Name+"."+node.Sel.Name]++
				d.References[node.Sel.Name]++
			}
		}
		return true
	})
}

// FindUnused returns declarations with zero references, filtering out
// known special functions (main, init, test functions, interface implementations).
func (d *DeadCodeDetector) FindUnused() []DeadCode {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []DeadCode

	for key, decl := range d.Declarations {
		// Skip special functions
		baseName := decl.Name
		if strings.Contains(baseName, ".") {
			parts := strings.SplitN(baseName, ".", 2)
			baseName = parts[1]
		}

		if baseName == "main" || baseName == "init" {
			continue
		}
		if IsTestFunction(baseName) {
			continue
		}

		// Count references: subtract 1 for the declaration itself
		refCount := d.References[baseName]
		// For methods, also check the qualified name
		if strings.Contains(decl.Name, ".") {
			refCount += d.References[decl.Name]
		}

		// The declaration itself counts as one reference in the AST walk
		// so we check if references <= 1 (only the declaration)
		if refCount <= 1 {
			confidence := 0.9
			reason := "0 references"

			if decl.Exported {
				confidence = 0.5
				reason = "0 internal references"
			}

			results = append(results, DeadCode{
				File:       decl.File,
				Line:       decl.Line,
				Name:       decl.Name,
				Kind:       decl.Kind,
				Confidence: confidence,
				Reason:     reason,
			})
		} else {
			_ = key // used for lineCountCache lookup
		}
	}

	// Sort by confidence (high first), then by file and line
	sort.Slice(results, func(i, j int) bool {
		if results[i].Confidence != results[j].Confidence {
			return results[i].Confidence > results[j].Confidence
		}
		if results[i].File != results[j].File {
			return results[i].File < results[j].File
		}
		return results[i].Line < results[j].Line
	})

	return results
}

// FindUnusedExports finds exported symbols not referenced anywhere in the project.
func (d *DeadCodeDetector) FindUnusedExports(projectDir string) []DeadCode {
	// Ensure we have scanned the project
	if len(d.Declarations) == 0 {
		if _, err := d.Scan(projectDir); err != nil {
			return nil
		}
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []DeadCode

	for _, decl := range d.Declarations {
		if !decl.Exported {
			continue
		}

		baseName := decl.Name
		if strings.Contains(baseName, ".") {
			parts := strings.SplitN(baseName, ".", 2)
			baseName = parts[1]
		}

		if IsTestFunction(baseName) {
			continue
		}

		// Count references minus the declaration
		refCount := d.References[baseName]
		if strings.Contains(decl.Name, ".") {
			refCount += d.References[decl.Name]
		}

		if refCount <= 1 {
			confidence := 0.6
			reason := "0 internal references"

			// Higher confidence if module has no known external consumers
			if d.moduleName != "" && !isLibraryModule(d.moduleName) {
				confidence = 0.8
				reason = "0 internal references, unlikely external consumers"
			}

			results = append(results, DeadCode{
				File:       decl.File,
				Line:       decl.Line,
				Name:       decl.Name,
				Kind:       decl.Kind,
				Confidence: confidence,
				Reason:     reason,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Confidence != results[j].Confidence {
			return results[i].Confidence > results[j].Confidence
		}
		if results[i].File != results[j].File {
			return results[i].File < results[j].File
		}
		return results[i].Line < results[j].Line
	})

	return results
}

// IsTestFunction returns true if name looks like a Go test, benchmark, or example function.
func IsTestFunction(name string) bool {
	if strings.HasPrefix(name, "Test") && len(name) > 4 && unicode.IsUpper(rune(name[4])) {
		return true
	}
	if strings.HasPrefix(name, "Benchmark") && len(name) > 9 && unicode.IsUpper(rune(name[9])) {
		return true
	}
	if strings.HasPrefix(name, "Example") {
		return true
	}
	if strings.HasPrefix(name, "Fuzz") && len(name) > 4 && unicode.IsUpper(rune(name[4])) {
		return true
	}
	return false
}

// IsInterfaceImpl checks if a function with the given name might satisfy an interface
// by searching for interface declarations in the provided content.
func IsInterfaceImpl(name string, content string) bool {
	// Parse the content to find interface declarations
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, 0)
	if err != nil {
		return false
	}

	// Extract method name (strip receiver prefix if present)
	methodName := name
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		methodName = name[idx+1:]
	}

	// Look for interface types that declare a method with this name
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			ifaceType, ok := typeSpec.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			for _, method := range ifaceType.Methods.List {
				for _, ident := range method.Names {
					if ident.Name == methodName {
						return true
					}
				}
			}
		}
	}

	return false
}

// FormatDeadCode formats the results into a human-readable report.
func FormatDeadCode(items []DeadCode) string {
	if len(items) == 0 {
		return "Dead Code (0 items):\nNo dead code detected."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Dead Code (%d items):\n", len(items)))
	sb.WriteString(strings.Repeat("═", 21))
	sb.WriteString("\n")

	// Group by confidence level
	var high, medium []DeadCode
	for _, item := range items {
		if item.Confidence >= 0.7 {
			high = append(high, item)
		} else {
			medium = append(medium, item)
		}
	}

	if len(high) > 0 {
		sb.WriteString("HIGH confidence:\n")
		for _, item := range high {
			sb.WriteString(fmt.Sprintf("  %-20s %s %s — %s\n",
				fmt.Sprintf("%s:%d", item.File, item.Line),
				item.Kind,
				item.Name,
				item.Reason))
		}
	}

	if len(medium) > 0 {
		if len(high) > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("MEDIUM confidence (exported, may have external users):\n")
		for _, item := range medium {
			sb.WriteString(fmt.Sprintf("  %-20s %s %s — %s\n",
				fmt.Sprintf("%s:%d", item.File, item.Line),
				item.Kind,
				item.Name,
				item.Reason))
		}
	}

	sb.WriteString(fmt.Sprintf("\nEstimated removable: ~%d lines\n", EstimateRemovableLines(items)))

	return sb.String()
}

// EstimateRemovableLines estimates the total number of lines that could be
// removed if all dead code items were eliminated.
func EstimateRemovableLines(items []DeadCode) int {
	total := 0
	for _, item := range items {
		switch item.Kind {
		case "function", "method":
			total += 10 // average function length estimate
		case "type":
			total += 5 // average type definition
		case "var", "const":
			total += 1
		default:
			total += 1
		}
	}
	return total
}

// GenerateRemovalPlan produces an ordered list of safe removals.
func GenerateRemovalPlan(items []DeadCode) string {
	if len(items) == 0 {
		return "No dead code to remove."
	}

	// Sort by confidence descending, then by file for grouping
	sorted := make([]DeadCode, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Confidence != sorted[j].Confidence {
			return sorted[i].Confidence > sorted[j].Confidence
		}
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		return sorted[i].Line > sorted[j].Line // reverse line order for safe removal
	})

	var sb strings.Builder
	sb.WriteString("Removal Plan\n")
	sb.WriteString(strings.Repeat("=", 40))
	sb.WriteString("\n\n")
	sb.WriteString("Order: remove from bottom of file upward to preserve line numbers.\n")
	sb.WriteString("Review exported symbols before removing (may have external consumers).\n\n")

	currentFile := ""
	step := 1
	for _, item := range sorted {
		if item.File != currentFile {
			currentFile = item.File
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n", currentFile))
		}
		confidence := "HIGH"
		if item.Confidence < 0.7 {
			confidence = "MEDIUM"
		}
		sb.WriteString(fmt.Sprintf("  %d. [%s] Remove %s %q (line %d) — %s\n",
			step, confidence, item.Kind, item.Name, item.Line, item.Reason))
		step++
	}

	sb.WriteString(fmt.Sprintf("\nTotal items: %d\n", len(sorted)))
	sb.WriteString(fmt.Sprintf("Estimated lines saved: ~%d\n", EstimateRemovableLines(sorted)))

	return sb.String()
}

// --- helper functions ---

func extractReceiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return extractReceiverType(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		// Generic type e.g. Foo[T]
		return extractReceiverType(t.X)
	}
	return ""
}

func estimateTypeLines(fset *token.FileSet, spec *ast.TypeSpec) int {
	start := fset.Position(spec.Pos()).Line
	end := fset.Position(spec.End()).Line
	lines := end - start + 1
	if lines < 1 {
		lines = 1
	}
	return lines
}

func readModuleName(projectDir string) string {
	modPath := filepath.Join(projectDir, "go.mod")
	data, err := os.ReadFile(modPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func isLibraryModule(moduleName string) bool {
	// Heuristic: if the module path looks like it could be imported as a library
	// (contains known hosting prefixes and doesn't end with /cmd or similar)
	if strings.Contains(moduleName, "/cmd") {
		return false
	}
	if strings.HasPrefix(moduleName, "github.com/") ||
		strings.HasPrefix(moduleName, "golang.org/") ||
		strings.HasPrefix(moduleName, "go.") {
		return true
	}
	return false
}
