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
)

// CodeSmell represents a detected design issue in source code.
type CodeSmell struct {
	ID                    string
	Name                  string
	File                  string
	Line                  int
	Severity              string // "minor", "major", "critical"
	Description           string
	RefactoringSuggestion string
	Category              string // "design", "complexity", "coupling", "naming"
}

// SmellDetector identifies code smells using structural analysis.
type SmellDetector struct {
	Thresholds SmellThresholds
	mu         sync.RWMutex
}

// SmellThresholds defines configurable limits for code smell detection.
type SmellThresholds struct {
	MaxParams          int
	MaxMethodsPerType  int
	MaxFieldsPerStruct int
	MaxImports         int
	MaxFileLines       int
	MaxFuncLines       int
	MinMethodCohesion  float64
}

// NewSmellDetector creates a SmellDetector with default thresholds.
func NewSmellDetector() *SmellDetector {
	return &SmellDetector{
		Thresholds: SmellThresholds{
			MaxParams:          5,
			MaxMethodsPerType:  15,
			MaxFieldsPerStruct: 12,
			MaxImports:         15,
			MaxFileLines:       500,
			MaxFuncLines:       50,
			MinMethodCohesion:  0.3,
		},
	}
}

// DetectInFile runs all smell detectors on a file given its path and content.
func (sd *SmellDetector) DetectInFile(path, content string) []CodeSmell {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	var smells []CodeSmell

	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".go" {
		// For non-Go files, only run line-count checks
		lines := strings.Split(content, "\n")
		if len(lines) > sd.Thresholds.MaxFileLines {
			smells = append(smells, CodeSmell{
				ID:                    "large-file",
				Name:                  "Large File",
				File:                  path,
				Line:                  1,
				Severity:              severityForFileSize(len(lines), sd.Thresholds.MaxFileLines),
				Description:           fmt.Sprintf("File has %d lines (threshold: %d)", len(lines), sd.Thresholds.MaxFileLines),
				RefactoringSuggestion: "Split into multiple focused files with single responsibilities",
				Category:              "complexity",
			})
		}
		return smells
	}

	// Go-specific AST analysis
	smells = append(smells, sd.DetectGodObject(content)...)
	smells = append(smells, sd.DetectLongParamList(content)...)
	smells = append(smells, sd.DetectFeatureEnvy(content)...)
	smells = append(smells, sd.DetectDataClump(content)...)
	smells = append(smells, sd.DetectPrimitiveObsession(content)...)
	smells = append(smells, sd.detectLongMethod(content)...)
	smells = append(smells, sd.detectExcessiveImports(content)...)
	smells = append(smells, sd.detectLargeFile(path, content)...)

	// Set file path on all smells
	for i := range smells {
		if smells[i].File == "" {
			smells[i].File = path
		}
	}

	return smells
}

// DetectGodObject finds types with too many methods or fields.
func (sd *SmellDetector) DetectGodObject(content string) []CodeSmell {
	sd.mu.RLock()
	thresholds := sd.Thresholds
	sd.mu.RUnlock()

	var smells []CodeSmell

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", content, 0)
	if err != nil {
		return nil
	}

	// Count methods per type
	methodCounts := make(map[string]int)
	methodFirstLine := make(map[string]int)
	methodLastLine := make(map[string]int)

	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			return true
		}
		typeName := receiverTypeName(fn.Recv.List[0].Type)
		if typeName == "" {
			return true
		}
		methodCounts[typeName]++
		line := fset.Position(fn.Pos()).Line
		if _, exists := methodFirstLine[typeName]; !exists {
			methodFirstLine[typeName] = line
		}
		endLine := fset.Position(fn.End()).Line
		if endLine > methodLastLine[typeName] {
			methodLastLine[typeName] = endLine
		}
		return true
	})

	for typeName, count := range methodCounts {
		if count > thresholds.MaxMethodsPerType {
			severity := "major"
			if count > thresholds.MaxMethodsPerType*2 {
				severity = "critical"
			}
			smells = append(smells, CodeSmell{
				ID:       "god-object-methods",
				Name:     "God Object",
				Line:     methodFirstLine[typeName],
				Severity: severity,
				Description: fmt.Sprintf("%s has %d methods — extract into smaller services",
					typeName, count),
				RefactoringSuggestion: fmt.Sprintf("Split %s into focused interfaces/structs with single responsibilities", typeName),
				Category:              "design",
			})
		}
	}

	// Count fields per struct
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}

		fieldCount := 0
		if st.Fields != nil {
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 {
					fieldCount++ // embedded
				} else {
					fieldCount += len(field.Names)
				}
			}
		}

		if fieldCount > thresholds.MaxFieldsPerStruct {
			severity := "major"
			if fieldCount > thresholds.MaxFieldsPerStruct*2 {
				severity = "critical"
			}
			smells = append(smells, CodeSmell{
				ID:       "god-object-fields",
				Name:     "God Object",
				Line:     fset.Position(ts.Pos()).Line,
				Severity: severity,
				Description: fmt.Sprintf("%s has %d fields — too many responsibilities",
					ts.Name.Name, fieldCount),
				RefactoringSuggestion: fmt.Sprintf("Extract related fields from %s into smaller focused structs", ts.Name.Name),
				Category:              "design",
			})
		}
		return true
	})

	return smells
}

// DetectLongParamList finds functions with too many parameters.
func (sd *SmellDetector) DetectLongParamList(content string) []CodeSmell {
	sd.mu.RLock()
	thresholds := sd.Thresholds
	sd.mu.RUnlock()

	var smells []CodeSmell

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", content, 0)
	if err != nil {
		return nil
	}

	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		paramCount := countFuncParams(fn)
		if paramCount > thresholds.MaxParams {
			severity := "major"
			if paramCount > thresholds.MaxParams*2 {
				severity = "critical"
			}

			paramNames := extractParamNames(fn)
			smells = append(smells, CodeSmell{
				ID:       "long-param-list",
				Name:     "Long Parameter List",
				Line:     fset.Position(fn.Pos()).Line,
				Severity: severity,
				Description: fmt.Sprintf("func %s(%s) has %d parameters (threshold: %d)",
					fn.Name.Name, strings.Join(paramNames, ", "), paramCount, thresholds.MaxParams),
				RefactoringSuggestion: "Group related parameters into a config/options struct",
				Category:              "design",
			})
		}
		return true
	})

	return smells
}

// DetectFeatureEnvy finds methods that use another type's fields more than their own.
func (sd *SmellDetector) DetectFeatureEnvy(content string) []CodeSmell {
	var smells []CodeSmell

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", content, 0)
	if err != nil {
		return nil
	}

	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 || fn.Body == nil {
			return true
		}

		recvTypeName := receiverTypeName(fn.Recv.List[0].Type)
		if recvTypeName == "" {
			return true
		}

		// Get receiver variable name
		recvVarName := ""
		if len(fn.Recv.List[0].Names) > 0 {
			recvVarName = fn.Recv.List[0].Names[0].Name
		}

		// Count selector expression usage by target
		typeCounts := make(map[string]int)
		ownCount := 0

		ast.Inspect(fn.Body, func(inner ast.Node) bool {
			sel, ok := inner.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == recvVarName {
					ownCount++
				} else {
					typeCounts[ident.Name]++
				}
			}
			return true
		})

		// Check if any external type is used more than own receiver
		for externalVar, count := range typeCounts {
			if count > ownCount && count >= 3 {
				smells = append(smells, CodeSmell{
					ID:       "feature-envy",
					Name:     "Feature Envy",
					Line:     fset.Position(fn.Pos()).Line,
					Severity: "minor",
					Description: fmt.Sprintf("method %s uses %s fields %d times but own fields %d times",
						fn.Name.Name, externalVar, count, ownCount),
					RefactoringSuggestion: fmt.Sprintf("Consider moving %s to the type that owns %s's data", fn.Name.Name, externalVar),
					Category:              "coupling",
				})
			}
		}

		return true
	})

	return smells
}

// DetectDataClump finds groups of parameters that appear together in multiple functions.
func (sd *SmellDetector) DetectDataClump(content string) []CodeSmell {
	var smells []CodeSmell

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", content, 0)
	if err != nil {
		return nil
	}

	// Collect parameter lists for all functions
	type funcParams struct {
		name   string
		line   int
		params []string // "name:type" pairs
	}

	var allFuncs []funcParams

	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Type.Params == nil {
			return true
		}

		var params []string
		for _, field := range fn.Type.Params.List {
			typStr := exprToString(field.Type)
			for _, name := range field.Names {
				params = append(params, name.Name+":"+typStr)
			}
			if len(field.Names) == 0 {
				params = append(params, "_:"+typStr)
			}
		}

		if len(params) >= 3 {
			allFuncs = append(allFuncs, funcParams{
				name:   fn.Name.Name,
				line:   fset.Position(fn.Pos()).Line,
				params: params,
			})
		}
		return true
	})

	// Find parameter groups that appear in multiple functions
	// Group by type signature (ignoring names)
	type paramGroup struct {
		types []string
		funcs []string
		line  int
	}

	// Check all pairs of functions for shared parameter type sequences of length >= 3
	reported := make(map[string]bool)
	for i := 0; i < len(allFuncs); i++ {
		for j := i + 1; j < len(allFuncs); j++ {
			shared := findSharedParamTypes(allFuncs[i].params, allFuncs[j].params)
			if len(shared) >= 3 {
				key := strings.Join(shared, ",")
				if reported[key] {
					continue
				}
				reported[key] = true

				smells = append(smells, CodeSmell{
					ID:       "data-clump",
					Name:     "Data Clump",
					Line:     allFuncs[i].line,
					Severity: "minor",
					Description: fmt.Sprintf("Parameters (%s) appear together in %s and %s",
						strings.Join(shared, ", "), allFuncs[i].name, allFuncs[j].name),
					RefactoringSuggestion: "Extract these parameters into a dedicated struct",
					Category:              "design",
				})
			}
		}
	}

	return smells
}

// DetectPrimitiveObsession finds functions taking too many string/int params.
func (sd *SmellDetector) DetectPrimitiveObsession(content string) []CodeSmell {
	var smells []CodeSmell

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", content, 0)
	if err != nil {
		return nil
	}

	primitiveTypes := map[string]bool{
		"string": true, "int": true, "int8": true, "int16": true, "int32": true, "int64": true,
		"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
		"float32": true, "float64": true, "bool": true,
	}

	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Type.Params == nil {
			return true
		}

		primitiveCount := 0
		totalCount := 0
		var primitiveNames []string

		for _, field := range fn.Type.Params.List {
			typStr := exprToString(field.Type)
			isPrimitive := primitiveTypes[typStr]

			count := len(field.Names)
			if count == 0 {
				count = 1
			}
			totalCount += count

			if isPrimitive {
				primitiveCount += count
				for _, name := range field.Names {
					primitiveNames = append(primitiveNames, name.Name+" "+typStr)
				}
			}
		}

		if primitiveCount > 3 && totalCount >= 4 {
			smells = append(smells, CodeSmell{
				ID:       "primitive-obsession",
				Name:     "Primitive Obsession",
				Line:     fset.Position(fn.Pos()).Line,
				Severity: "minor",
				Description: fmt.Sprintf("func %s has %d primitive parameters (%s) — consider typed alternatives",
					fn.Name.Name, primitiveCount, strings.Join(primitiveNames, ", ")),
				RefactoringSuggestion: "Define domain types (e.g., type UserID string) to add meaning and type safety",
				Category:              "design",
			})
		}

		return true
	})

	return smells
}

// detectLongMethod finds functions exceeding MaxFuncLines.
func (sd *SmellDetector) detectLongMethod(content string) []CodeSmell {
	var smells []CodeSmell

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", content, 0)
	if err != nil {
		return nil
	}

	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}

		startLine := fset.Position(fn.Body.Pos()).Line
		endLine := fset.Position(fn.Body.End()).Line
		funcLines := endLine - startLine + 1

		if funcLines > sd.Thresholds.MaxFuncLines {
			severity := "major"
			if funcLines > sd.Thresholds.MaxFuncLines*2 {
				severity = "critical"
			}
			smells = append(smells, CodeSmell{
				ID:       "long-method",
				Name:     "Long Method",
				Line:     fset.Position(fn.Pos()).Line,
				Severity: severity,
				Description: fmt.Sprintf("func %s is %d lines long (threshold: %d)",
					fn.Name.Name, funcLines, sd.Thresholds.MaxFuncLines),
				RefactoringSuggestion: "Extract logical sections into well-named helper functions",
				Category:              "complexity",
			})
		}
		return true
	})

	return smells
}

// detectExcessiveImports finds files with too many imports.
func (sd *SmellDetector) detectExcessiveImports(content string) []CodeSmell {
	var smells []CodeSmell

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", content, parser.ImportsOnly)
	if err != nil {
		return nil
	}

	importCount := len(f.Imports)
	if importCount > sd.Thresholds.MaxImports {
		severity := "minor"
		if importCount > sd.Thresholds.MaxImports*2 {
			severity = "major"
		}
		smells = append(smells, CodeSmell{
			ID:       "excessive-imports",
			Name:     "Excessive Imports",
			Line:     1,
			Severity: severity,
			Description: fmt.Sprintf("File has %d imports (threshold: %d) — may indicate too many responsibilities",
				importCount, sd.Thresholds.MaxImports),
			RefactoringSuggestion: "Split file into multiple packages with focused responsibilities",
			Category:              "coupling",
		})
	}

	return smells
}

// detectLargeFile detects files that exceed MaxFileLines.
func (sd *SmellDetector) detectLargeFile(path, content string) []CodeSmell {
	var smells []CodeSmell

	lines := strings.Split(content, "\n")
	if len(lines) > sd.Thresholds.MaxFileLines {
		smells = append(smells, CodeSmell{
			ID:       "large-file",
			Name:     "Large File",
			File:     path,
			Line:     1,
			Severity: severityForFileSize(len(lines), sd.Thresholds.MaxFileLines),
			Description: fmt.Sprintf("File has %d lines (threshold: %d)",
				len(lines), sd.Thresholds.MaxFileLines),
			RefactoringSuggestion: "Split into multiple focused files with single responsibilities",
			Category:              "complexity",
		})
	}

	return smells
}

// FormatSmells formats a list of code smells into a human-readable report.
func FormatSmells(smells []CodeSmell) string {
	if len(smells) == 0 {
		return "No code smells detected.\n"
	}

	// Group by file
	byFile := make(map[string][]CodeSmell)
	for _, s := range smells {
		byFile[s.File] = append(byFile[s.File], s)
	}

	// Sort file names for deterministic output
	var files []string
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)

	var sb strings.Builder

	for _, file := range files {
		fileSmells := byFile[file]

		// Sort by severity (critical > major > minor), then by line
		sort.Slice(fileSmells, func(i, j int) bool {
			si := severityRank(fileSmells[i].Severity)
			sj := severityRank(fileSmells[j].Severity)
			if si != sj {
				return si > sj
			}
			return fileSmells[i].Line < fileSmells[j].Line
		})

		sb.WriteString(fmt.Sprintf("Code Smells in %s:\n", file))
		sb.WriteString("─────────────────────────────────\n")

		for _, smell := range fileSmells {
			icon := severityIcon(smell.Severity)
			sb.WriteString(fmt.Sprintf("%s [%s] %s (L%d)\n",
				icon, smell.Severity, smell.Name, smell.Line))
			sb.WriteString(fmt.Sprintf("   %s\n", smell.Description))
			if smell.RefactoringSuggestion != "" {
				sb.WriteString(fmt.Sprintf("   → %s\n", smell.RefactoringSuggestion))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// ScanDirectory walks a directory and detects code smells in all Go files.
func (sd *SmellDetector) ScanDirectory(dir string) []CodeSmell {
	var mu sync.Mutex
	var allSmells []CodeSmell

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".go" {
			return nil
		}

		// Skip test files for dead code detection to reduce noise
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		smells := sd.DetectInFile(path, string(data))

		mu.Lock()
		allSmells = append(allSmells, smells...)
		mu.Unlock()

		return nil
	})

	// Sort results: critical first, then by file and line
	sort.Slice(allSmells, func(i, j int) bool {
		si := severityRank(allSmells[i].Severity)
		sj := severityRank(allSmells[j].Severity)
		if si != sj {
			return si > sj
		}
		if allSmells[i].File != allSmells[j].File {
			return allSmells[i].File < allSmells[j].File
		}
		return allSmells[i].Line < allSmells[j].Line
	})

	return allSmells
}

// --- Helper functions ---

// receiverTypeName extracts the type name from a receiver expression.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// countFuncParams counts the total number of parameters in a function declaration.
func countFuncParams(fn *ast.FuncDecl) int {
	if fn.Type.Params == nil {
		return 0
	}
	count := 0
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			count++
		} else {
			count += len(field.Names)
		}
	}
	return count
}

// extractParamNames returns the names of all parameters in a function.
func extractParamNames(fn *ast.FuncDecl) []string {
	if fn.Type.Params == nil {
		return nil
	}
	var names []string
	for _, field := range fn.Type.Params.List {
		typStr := exprToString(field.Type)
		for _, name := range field.Names {
			names = append(names, name.Name+" "+typStr)
		}
		if len(field.Names) == 0 {
			names = append(names, typStr)
		}
	}
	return names
}

// exprToString converts a type expression to a simple string representation.
func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprToString(t.Elt)
	case *ast.MapType:
		return "map[" + exprToString(t.Key) + "]" + exprToString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.Ellipsis:
		return "..." + exprToString(t.Elt)
	case *ast.FuncType:
		return "func"
	case *ast.ChanType:
		return "chan"
	default:
		return "unknown"
	}
}

// findSharedParamTypes finds parameter types that appear in both parameter lists.
func findSharedParamTypes(params1, params2 []string) []string {
	// Extract types from "name:type" format
	types1 := make(map[string]int)
	for _, p := range params1 {
		parts := strings.SplitN(p, ":", 2)
		if len(parts) == 2 {
			types1[parts[1]]++
		}
	}

	types2 := make(map[string]int)
	for _, p := range params2 {
		parts := strings.SplitN(p, ":", 2)
		if len(parts) == 2 {
			types2[parts[1]]++
		}
	}

	// Find types present in both, counting minimum occurrences
	var shared []string
	for typ, count1 := range types1 {
		if count2, ok := types2[typ]; ok {
			minCount := count1
			if count2 < minCount {
				minCount = count2
			}
			for i := 0; i < minCount; i++ {
				shared = append(shared, typ)
			}
		}
	}

	return shared
}

// severityRank converts severity to a numeric rank for sorting.
func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 3
	case "major":
		return 2
	case "minor":
		return 1
	default:
		return 0
	}
}

// severityIcon returns the display icon for a severity level.
func severityIcon(severity string) string {
	switch severity {
	case "critical":
		return "\U0001f534"
	case "major":
		return "\U0001f7e1"
	case "minor":
		return "\U0001f7e2"
	default:
		return "•"
	}
}

// severityForFileSize determines severity based on how much a file exceeds the threshold.
func severityForFileSize(lines, threshold int) string {
	if lines > threshold*3 {
		return "critical"
	}
	if lines > threshold*2 {
		return "major"
	}
	return "minor"
}
