package engine

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strings"
)

// This file holds the internal helpers for semantic diff analysis: AST-based
// parsing of Go source, import/function detection, and signature string
// extraction. The SemanticAnalyzer type and the high-level analysis entry
// points live in semantic_diff.go.

// parseFunctions extracts exported and unexported function signatures from Go source.
func parseFunctions(content string) map[string]string {
	funcs := make(map[string]string)
	if content == "" {
		return funcs
	}

	fset := token.NewFileSet()
	// Wrap content in a package declaration if needed
	src := ensurePackage(content)
	file, err := parser.ParseFile(fset, "", src, parser.AllErrors)
	if err != nil {
		// Fallback to regex-based parsing
		return parseFunctionsRegex(content)
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		name := fn.Name.Name
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			recvType := formatFieldType(fn.Recv.List[0].Type)
			name = recvType + "." + fn.Name.Name
		}

		sig := formatFuncSignature(fn)
		funcs[name] = sig
	}

	return funcs
}

// parseTypes extracts type definitions from Go source.
func parseTypes(content string) map[string]string {
	types := make(map[string]string)
	if content == "" {
		return types
	}

	fset := token.NewFileSet()
	src := ensurePackage(content)
	file, err := parser.ParseFile(fset, "", src, parser.AllErrors)
	if err != nil {
		return types
	}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			types[ts.Name.Name] = formatNode(fset, ts.Type)
		}
	}

	return types
}

// parseInterfaces extracts interface type definitions and their methods.
func parseInterfaces(content string) map[string]map[string]bool {
	interfaces := make(map[string]map[string]bool)
	if content == "" {
		return interfaces
	}

	fset := token.NewFileSet()
	src := ensurePackage(content)
	file, err := parser.ParseFile(fset, "", src, parser.AllErrors)
	if err != nil {
		return interfaces
	}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			iface, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			methods := make(map[string]bool)
			if iface.Methods != nil {
				for _, m := range iface.Methods.List {
					for _, name := range m.Names {
						methods[name.Name] = true
					}
				}
			}
			interfaces[ts.Name.Name] = methods
		}
	}

	return interfaces
}

// parseFuncBodies extracts function bodies as strings keyed by function name.
func parseFuncBodies(content string) map[string]string {
	bodies := make(map[string]string)
	if content == "" {
		return bodies
	}

	fset := token.NewFileSet()
	src := ensurePackage(content)
	file, err := parser.ParseFile(fset, "", src, parser.AllErrors)
	if err != nil {
		return bodies
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		name := fn.Name.Name
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			recvType := formatFieldType(fn.Recv.List[0].Type)
			name = recvType + "." + fn.Name.Name
		}

		start := fset.Position(fn.Body.Pos()).Offset
		end := fset.Position(fn.Body.End()).Offset
		if start >= 0 && end <= len(src) && start < end {
			bodies[name] = src[start:end]
		}
	}

	return bodies
}

// detectImportChanges identifies added and removed imports.
func detectImportChanges(oldContent, newContent string) []SemanticChange {
	var changes []SemanticChange

	oldImports := parseImports(oldContent)
	newImports := parseImports(newContent)

	for imp := range newImports {
		if !oldImports[imp] {
			changes = append(changes, SemanticChange{
				Type:        "import_added",
				Name:        imp,
				Description: fmt.Sprintf("Import added: %s", imp),
				Breaking:    false,
				Risk:        "low",
			})
		}
	}

	for imp := range oldImports {
		if !newImports[imp] {
			changes = append(changes, SemanticChange{
				Type:        "import_removed",
				Name:        imp,
				Description: fmt.Sprintf("Import removed: %s", imp),
				Breaking:    false,
				Risk:        "low",
			})
		}
	}

	return changes
}

// detectAddedFunctions finds functions present in new content but not in old.
func detectAddedFunctions(oldContent, newContent string) []SemanticChange {
	var changes []SemanticChange

	oldFuncs := parseFunctions(oldContent)
	newFuncs := parseFunctions(newContent)

	for name, sig := range newFuncs {
		if _, exists := oldFuncs[name]; !exists {
			changes = append(changes, SemanticChange{
				Type:        "function_added",
				Name:        name,
				Description: fmt.Sprintf("Function added: %s", sig),
				Breaking:    false,
				Risk:        "low",
			})
		}
	}

	return changes
}

// parseImports extracts import paths from Go source.
func parseImports(content string) map[string]bool {
	imports := make(map[string]bool)
	if content == "" {
		return imports
	}

	fset := token.NewFileSet()
	src := ensurePackage(content)
	file, err := parser.ParseFile(fset, "", src, parser.ImportsOnly)
	if err != nil {
		return imports
	}

	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		imports[path] = true
	}

	return imports
}

// ensurePackage wraps content with a package clause if missing.
func ensurePackage(content string) string {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "package ") {
		return content
	}
	return "package _semantic_diff_analysis\n\n" + content
}

// formatFuncSignature formats a function declaration's signature as a string.
func formatFuncSignature(fn *ast.FuncDecl) string {
	var sb strings.Builder

	sb.WriteString("func ")

	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		sb.WriteString("(")
		sb.WriteString(formatFieldType(fn.Recv.List[0].Type))
		sb.WriteString(") ")
	}

	sb.WriteString(fn.Name.Name)
	sb.WriteString("(")

	if fn.Type.Params != nil {
		params := make([]string, 0)
		for _, field := range fn.Type.Params.List {
			typeName := formatFieldType(field.Type)
			if len(field.Names) == 0 {
				params = append(params, typeName)
			} else {
				for _, name := range field.Names {
					params = append(params, name.Name+" "+typeName)
				}
			}
		}
		sb.WriteString(strings.Join(params, ", "))
	}

	sb.WriteString(")")

	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		results := make([]string, 0)
		for _, field := range fn.Type.Results.List {
			typeName := formatFieldType(field.Type)
			if len(field.Names) == 0 {
				results = append(results, typeName)
			} else {
				for _, name := range field.Names {
					results = append(results, name.Name+" "+typeName)
				}
			}
		}
		if len(results) == 1 {
			sb.WriteString(" " + results[0])
		} else {
			sb.WriteString(" (" + strings.Join(results, ", ") + ")")
		}
	}

	return sb.String()
}

// formatFieldType formats an AST expression representing a type.
func formatFieldType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + formatFieldType(t.X)
	case *ast.SelectorExpr:
		return formatFieldType(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + formatFieldType(t.Elt)
		}
		return "[...]" + formatFieldType(t.Elt)
	case *ast.MapType:
		return "map[" + formatFieldType(t.Key) + "]" + formatFieldType(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.Ellipsis:
		return "..." + formatFieldType(t.Elt)
	case *ast.ChanType:
		return "chan " + formatFieldType(t.Value)
	default:
		return "unknown"
	}
}

// formatNode returns a simple string representation of a type node.
func formatNode(fset *token.FileSet, node ast.Node) string {
	switch t := node.(type) {
	case *ast.StructType:
		var fields []string
		if t.Fields != nil {
			for _, f := range t.Fields.List {
				typeName := formatFieldType(f.Type)
				for _, name := range f.Names {
					fields = append(fields, name.Name+" "+typeName)
				}
				if len(f.Names) == 0 {
					fields = append(fields, typeName)
				}
			}
		}
		return "struct{" + strings.Join(fields, "; ") + "}"
	case *ast.InterfaceType:
		var methods []string
		if t.Methods != nil {
			for _, m := range t.Methods.List {
				for _, name := range m.Names {
					methods = append(methods, name.Name)
				}
			}
		}
		return "interface{" + strings.Join(methods, "; ") + "}"
	case *ast.Ident:
		return t.Name
	default:
		if expr, ok := node.(ast.Expr); ok {
			return formatFieldType(expr)
		}
		return "unknown"
	}
}

// parseFunctionsRegex is a fallback for when AST parsing fails.
func parseFunctionsRegex(content string) map[string]string {
	funcs := make(map[string]string)
	re := regexp.MustCompile(`func\s+(?:\([^)]*\)\s+)?(\w+)\s*\([^)]*\)(?:\s*(?:\([^)]*\)|[^\s{]+))?\s*{`)
	matches := re.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) > 1 {
			funcs[m[1]] = m[0]
		}
	}
	return funcs
}

// isExported returns true if the name starts with an uppercase letter.
func isExported(name string) bool {
	if name == "" {
		return false
	}
	// Handle receiver.method format
	parts := strings.Split(name, ".")
	checkName := parts[len(parts)-1]
	if checkName == "" {
		return false
	}
	return checkName[0] >= 'A' && checkName[0] <= 'Z'
}

// countPattern counts regex matches in text.
func countPattern(text, pattern string) int {
	re := regexp.MustCompile(pattern)
	return len(re.FindAllString(text, -1))
}

// extractLoopBounds extracts a hash of loop conditions for comparison.
func extractLoopBounds(body string) string {
	re := regexp.MustCompile(`for\s+([^{]+){`)
	matches := re.FindAllStringSubmatch(body, -1)
	var bounds []string
	for _, m := range matches {
		if len(m) > 1 {
			bounds = append(bounds, strings.TrimSpace(m[1]))
		}
	}
	sort.Strings(bounds)
	return strings.Join(bounds, ";")
}

// extractRoutes finds route patterns and associated handler names in content.
func (sa *SemanticAnalyzer) extractRoutes(content string) map[string][]string {
	routes := make(map[string][]string)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		for _, pattern := range sa.routePatterns {
			matches := pattern.FindStringSubmatch(line)
			if len(matches) > 1 {
				route := matches[1]
				// Extract handler name from the same line
				handlerRe := regexp.MustCompile(`(\w+)\s*[,)]`)
				handlerMatches := handlerRe.FindAllStringSubmatch(line, -1)
				for _, hm := range handlerMatches {
					if len(hm) > 1 && hm[1] != "http" && hm[1] != "func" {
						routes[route] = append(routes[route], hm[1])
					}
				}
			}
		}
	}

	return routes
}

// formatChangeLine formats a SemanticChange for display in the summary.
func formatChangeLine(c SemanticChange) string {
	if c.Description != "" {
		return c.Description
	}
	return c.Name
}

// extractFuncName extracts the function name from a signature string.
func extractFuncName(sig string) string {
	re := regexp.MustCompile(`func\s+(?:\([^)]*\)\s+)?(\w+)`)
	matches := re.FindStringSubmatch(sig)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// extractReceiver extracts the receiver type from a signature string.
func extractReceiver(sig string) string {
	re := regexp.MustCompile(`func\s+\(([^)]*)\)`)
	matches := re.FindStringSubmatch(sig)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// extractParams extracts parameter list from a function signature.
func extractParams(sig string) []string {
	// Find params after the function name
	re := regexp.MustCompile(`func\s+(?:\([^)]*\)\s+)?\w+\(([^)]*)\)`)
	matches := re.FindStringSubmatch(sig)
	if len(matches) < 2 || matches[1] == "" {
		return nil
	}
	params := strings.Split(matches[1], ",")
	var result []string
	for _, p := range params {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// extractReturnType extracts return type(s) from a function signature.
func extractReturnType(sig string) string {
	// Find the last closing paren of params, then get everything after
	re := regexp.MustCompile(`func\s+(?:\([^)]*\)\s+)?\w+\([^)]*\)\s*(.*)$`)
	matches := re.FindStringSubmatch(sig)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}
