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

// NavIndex provides code navigation capabilities (go-to-definition, find-references,
// find-implementations) without requiring an LSP server. It parses Go source files
// using go/ast and builds an in-memory index of definitions, references, and
// interface implementations.
type NavIndex struct {
	Definitions     map[string]*Definition
	References      map[string][]*Reference
	Implementations map[string][]string
	mu              sync.RWMutex

	// internal: maps funcName -> list of functions it calls
	callees map[string][]string
	// internal: interface method sets for implementation matching
	ifaceMethods map[string][]string
	// internal: type method sets
	typeMethods map[string][]string
}

// Definition represents a symbol definition in the codebase.
type Definition struct {
	Name       string
	Kind       string // "func", "type", "var", "const", "method", "interface"
	File       string
	Line       int
	Package    string
	Exported   bool
	Signature  string
	DocComment string
}

// Reference represents a usage of a symbol in the codebase.
type Reference struct {
	File    string
	Line    int
	Context string // surrounding line text
	Kind    string // "call", "type_use", "import", "assignment"
}

// NewNavIndex creates a new empty navigation index.
func NewNavIndex() *NavIndex {
	return &NavIndex{
		Definitions:     make(map[string]*Definition),
		References:      make(map[string][]*Reference),
		Implementations: make(map[string][]string),
		callees:         make(map[string][]string),
		ifaceMethods:    make(map[string][]string),
		typeMethods:     make(map[string][]string),
	}
}

// BuildIndex parses all Go files under projectDir and populates the index
// with definitions, references, and implementation mappings.
func (idx *NavIndex) BuildIndex(projectDir string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Reset maps
	idx.Definitions = make(map[string]*Definition)
	idx.References = make(map[string][]*Reference)
	idx.Implementations = make(map[string][]string)
	idx.callees = make(map[string][]string)
	idx.ifaceMethods = make(map[string][]string)
	idx.typeMethods = make(map[string][]string)

	fset := token.NewFileSet()

	err := filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == "node_modules" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relPath, _ := filepath.Rel(projectDir, path)
		if relPath == "" {
			relPath = path
		}

		f, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return nil // skip unparseable files
		}

		pkgName := ""
		if f.Name != nil {
			pkgName = f.Name.Name
		}

		// Read file lines for context extraction
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		lines := strings.Split(string(content), "\n")

		// First pass: extract definitions
		idx.extractDefinitions(f, fset, relPath, pkgName, lines)

		// Second pass: extract references and callees
		idx.extractReferences(f, fset, relPath, lines)

		return nil
	})

	if err != nil {
		return fmt.Errorf("walking project directory: %w", err)
	}

	// Build implementation mappings
	idx.buildImplementations()

	return nil
}

// extractDefinitions visits AST nodes to find top-level definitions.
func (idx *NavIndex) extractDefinitions(f *ast.File, fset *token.FileSet, relPath, pkgName string, lines []string) {
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			idx.extractFuncDef(d, fset, relPath, pkgName, lines)
		case *ast.GenDecl:
			idx.extractGenDecl(d, fset, relPath, pkgName, lines)
		}
	}
}

func (idx *NavIndex) extractFuncDef(d *ast.FuncDecl, fset *token.FileSet, relPath, pkgName string, lines []string) {
	name := d.Name.Name
	line := fset.Position(d.Pos()).Line
	exported := navIsExported(name)

	kind := "func"
	key := name
	if d.Recv != nil && len(d.Recv.List) > 0 {
		kind = "method"
		recvType := extractRecvTypeName(d.Recv.List[0].Type)
		key = recvType + "." + name
		// Track method for type
		idx.typeMethods[recvType] = append(idx.typeMethods[recvType], name)
	}

	sig := buildFuncSignature(d)
	doc := extractDocComment(d.Doc)

	idx.Definitions[key] = &Definition{
		Name:       name,
		Kind:       kind,
		File:       relPath,
		Line:       line,
		Package:    pkgName,
		Exported:   exported,
		Signature:  sig,
		DocComment: doc,
	}
}

func (idx *NavIndex) extractGenDecl(d *ast.GenDecl, fset *token.FileSet, relPath, pkgName string, lines []string) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			name := s.Name.Name
			line := fset.Position(s.Pos()).Line
			exported := navIsExported(name)
			doc := extractDocComment(d.Doc)

			kind := "type"
			sig := "type " + name

			switch t := s.Type.(type) {
			case *ast.InterfaceType:
				kind = "interface"
				sig = "type " + name + " interface"
				// Extract interface methods
				if t.Methods != nil {
					for _, m := range t.Methods.List {
						if len(m.Names) > 0 {
							idx.ifaceMethods[name] = append(idx.ifaceMethods[name], m.Names[0].Name)
						}
					}
				}
			case *ast.StructType:
				sig = "type " + name + " struct"
			}

			idx.Definitions[name] = &Definition{
				Name:       name,
				Kind:       kind,
				File:       relPath,
				Line:       line,
				Package:    pkgName,
				Exported:   exported,
				Signature:  sig,
				DocComment: doc,
			}

		case *ast.ValueSpec:
			kind := "var"
			if d.Tok == token.CONST {
				kind = "const"
			}
			doc := extractDocComment(d.Doc)
			if doc == "" {
				doc = extractDocComment(s.Doc)
			}

			for _, ident := range s.Names {
				if ident.Name == "_" {
					continue
				}
				name := ident.Name
				line := fset.Position(ident.Pos()).Line
				exported := navIsExported(name)

				sig := kind + " " + name
				if s.Type != nil {
					sig += " " + formatTypeExpr(s.Type)
				}

				idx.Definitions[name] = &Definition{
					Name:       name,
					Kind:       kind,
					File:       relPath,
					Line:       line,
					Package:    pkgName,
					Exported:   exported,
					Signature:  sig,
					DocComment: doc,
				}
			}
		}
	}
}

// extractReferences visits all identifiers in the AST and records references.
func (idx *NavIndex) extractReferences(f *ast.File, fset *token.FileSet, relPath string, lines []string) {
	// Track current function for callee mapping
	var currentFunc string

	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			currentFunc = node.Name.Name
			if node.Recv != nil && len(node.Recv.List) > 0 {
				recvType := extractRecvTypeName(node.Recv.List[0].Type)
				currentFunc = recvType + "." + node.Name.Name
			}

		case *ast.CallExpr:
			switch fn := node.Fun.(type) {
			case *ast.Ident:
				line := fset.Position(fn.Pos()).Line
				ctx := getLineContext(lines, line)
				idx.References[fn.Name] = append(idx.References[fn.Name], &Reference{
					File:    relPath,
					Line:    line,
					Context: ctx,
					Kind:    "call",
				})
				if currentFunc != "" {
					idx.callees[currentFunc] = navAppendUnique(idx.callees[currentFunc], fn.Name)
				}
			case *ast.SelectorExpr:
				line := fset.Position(fn.Sel.Pos()).Line
				ctx := getLineContext(lines, line)
				idx.References[fn.Sel.Name] = append(idx.References[fn.Sel.Name], &Reference{
					File:    relPath,
					Line:    line,
					Context: ctx,
					Kind:    "call",
				})
				if currentFunc != "" {
					idx.callees[currentFunc] = navAppendUnique(idx.callees[currentFunc], fn.Sel.Name)
				}
			}

		case *ast.SelectorExpr:
			// Type usage via selector (e.g., pkg.Type) that isn't a call
			if _, isCall := n.(*ast.CallExpr); !isCall {
				line := fset.Position(node.Sel.Pos()).Line
				ctx := getLineContext(lines, line)
				ref := &Reference{
					File:    relPath,
					Line:    line,
					Context: ctx,
					Kind:    "type_use",
				}
				// Only add if not already added as a call at this location
				if !hasRefAtLine(idx.References[node.Sel.Name], relPath, line) {
					idx.References[node.Sel.Name] = append(idx.References[node.Sel.Name], ref)
				}
			}

		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					line := fset.Position(ident.Pos()).Line
					ctx := getLineContext(lines, line)
					idx.References[ident.Name] = append(idx.References[ident.Name], &Reference{
						File:    relPath,
						Line:    line,
						Context: ctx,
						Kind:    "assignment",
					})
				}
			}

		case *ast.ImportSpec:
			if node.Path != nil {
				importPath := strings.Trim(node.Path.Value, `"`)
				parts := strings.Split(importPath, "/")
				pkgAlias := parts[len(parts)-1]
				if node.Name != nil {
					pkgAlias = node.Name.Name
				}
				line := fset.Position(node.Pos()).Line
				ctx := getLineContext(lines, line)
				idx.References[pkgAlias] = append(idx.References[pkgAlias], &Reference{
					File:    relPath,
					Line:    line,
					Context: ctx,
					Kind:    "import",
				})
			}
		}
		return true
	})
}

// buildImplementations determines which types implement which interfaces
// by comparing method sets.
func (idx *NavIndex) buildImplementations() {
	for ifaceName, ifaceMethods := range idx.ifaceMethods {
		if len(ifaceMethods) == 0 {
			continue
		}
		for typeName, typeMethods := range idx.typeMethods {
			if implementsInterface(ifaceMethods, typeMethods) {
				idx.Implementations[ifaceName] = append(idx.Implementations[ifaceName], typeName)
			}
		}
	}
}

// GoToDefinition returns the definition for the given symbol name.
func (idx *NavIndex) GoToDefinition(symbol string) *Definition {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if def, ok := idx.Definitions[symbol]; ok {
		return def
	}
	return nil
}

// FindReferences returns all references to the given symbol.
func (idx *NavIndex) FindReferences(symbol string) []*Reference {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	refs := idx.References[symbol]
	if refs == nil {
		return nil
	}
	// Return a sorted copy
	result := make([]*Reference, len(refs))
	copy(result, refs)
	sort.Slice(result, func(i, j int) bool {
		if result[i].File != result[j].File {
			return result[i].File < result[j].File
		}
		return result[i].Line < result[j].Line
	})
	return result
}

// FindImplementations returns all types that implement the given interface.
func (idx *NavIndex) FindImplementations(interfaceName string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	impls := idx.Implementations[interfaceName]
	if impls == nil {
		return nil
	}
	result := make([]string, len(impls))
	copy(result, impls)
	sort.Strings(result)
	return result
}

// FindCallers returns all call sites of the given function.
func (idx *NavIndex) FindCallers(funcName string) []*Reference {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	refs := idx.References[funcName]
	if refs == nil {
		return nil
	}

	var callers []*Reference
	for _, ref := range refs {
		if ref.Kind == "call" {
			callers = append(callers, ref)
		}
	}
	sort.Slice(callers, func(i, j int) bool {
		if callers[i].File != callers[j].File {
			return callers[i].File < callers[j].File
		}
		return callers[i].Line < callers[j].Line
	})
	return callers
}

// FindCallees returns all functions called by the given function.
func (idx *NavIndex) FindCallees(funcName string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	callees := idx.callees[funcName]
	if callees == nil {
		return nil
	}
	result := make([]string, len(callees))
	copy(result, callees)
	sort.Strings(result)
	return result
}

// SearchSymbols performs a fuzzy search across all definitions, optionally filtered by kind.
func (idx *NavIndex) SearchSymbols(query string, kind string) []*Definition {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []*Definition
	queryLower := strings.ToLower(query)

	for _, def := range idx.Definitions {
		if kind != "" && def.Kind != kind {
			continue
		}
		if fuzzyMatch(queryLower, strings.ToLower(def.Name)) {
			results = append(results, def)
		}
	}

	// Sort by relevance: exact prefix match first, then by name
	sort.Slice(results, func(i, j int) bool {
		iName := strings.ToLower(results[i].Name)
		jName := strings.ToLower(results[j].Name)
		iPrefix := strings.HasPrefix(iName, queryLower)
		jPrefix := strings.HasPrefix(jName, queryLower)
		if iPrefix != jPrefix {
			return iPrefix
		}
		return iName < jName
	})

	return results
}

// FormatDefinition formats a definition for display.
func FormatDefinition(def *Definition) string {
	if def == nil {
		return ""
	}
	return fmt.Sprintf("%s %s (%s:%d)\n  %s",
		def.Kind, def.Name, def.File, def.Line, def.Signature)
}

// FormatReferences formats a list of references for display.
func FormatReferences(symbol string, refs []*Reference) string {
	if len(refs) == 0 {
		return fmt.Sprintf("No references found for %s", symbol)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("References to %s (%d found):\n", symbol, len(refs)))

	for _, ref := range refs {
		ctx := strings.TrimSpace(ref.Context)
		b.WriteString(fmt.Sprintf("  %s:%d    %s\n", ref.File, ref.Line, ctx))
	}

	return strings.TrimRight(b.String(), "\n")
}

// TypeHierarchy shows the interface -> implementations tree for a type.
func (idx *NavIndex) TypeHierarchy(typeName string) string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var b strings.Builder

	// Check if it's an interface
	if impls, ok := idx.Implementations[typeName]; ok {
		b.WriteString(fmt.Sprintf("interface %s\n", typeName))
		sorted := make([]string, len(impls))
		copy(sorted, impls)
		sort.Strings(sorted)
		for i, impl := range sorted {
			prefix := "├── "
			if i == len(sorted)-1 {
				prefix = "└── "
			}
			b.WriteString(fmt.Sprintf("  %s%s\n", prefix, impl))
		}
	} else {
		// Check if this type implements any interfaces
		b.WriteString(fmt.Sprintf("type %s\n", typeName))
		var implementsIfaces []string
		for ifaceName, impls := range idx.Implementations {
			for _, impl := range impls {
				if impl == typeName {
					implementsIfaces = append(implementsIfaces, ifaceName)
					break
				}
			}
		}
		sort.Strings(implementsIfaces)
		if len(implementsIfaces) > 0 {
			b.WriteString("  implements:\n")
			for _, iface := range implementsIfaces {
				b.WriteString(fmt.Sprintf("    - %s\n", iface))
			}
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// --- Helper functions ---

func navIsExported(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper(rune(name[0]))
}

func extractRecvTypeName(expr ast.Expr) string {
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

func buildFuncSignature(d *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("func ")

	if d.Recv != nil && len(d.Recv.List) > 0 {
		b.WriteString("(")
		field := d.Recv.List[0]
		if len(field.Names) > 0 {
			b.WriteString(field.Names[0].Name + " ")
		}
		b.WriteString(formatTypeExpr(field.Type))
		b.WriteString(") ")
	}

	b.WriteString(d.Name.Name)
	b.WriteString("(")

	// Parameters
	if d.Type.Params != nil {
		var params []string
		for _, field := range d.Type.Params.List {
			typeStr := formatTypeExpr(field.Type)
			if len(field.Names) > 0 {
				for _, name := range field.Names {
					params = append(params, name.Name+" "+typeStr)
				}
			} else {
				params = append(params, typeStr)
			}
		}
		b.WriteString(strings.Join(params, ", "))
	}
	b.WriteString(")")

	// Return types
	if d.Type.Results != nil && len(d.Type.Results.List) > 0 {
		var results []string
		for _, field := range d.Type.Results.List {
			typeStr := formatTypeExpr(field.Type)
			if len(field.Names) > 0 {
				for _, name := range field.Names {
					results = append(results, name.Name+" "+typeStr)
				}
			} else {
				results = append(results, typeStr)
			}
		}
		if len(results) == 1 {
			b.WriteString(" " + results[0])
		} else {
			b.WriteString(" (" + strings.Join(results, ", ") + ")")
		}
	}

	return b.String()
}

func formatTypeExpr(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + formatTypeExpr(t.X)
	case *ast.SelectorExpr:
		return formatTypeExpr(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + formatTypeExpr(t.Elt)
		}
		return "[...]" + formatTypeExpr(t.Elt)
	case *ast.MapType:
		return "map[" + formatTypeExpr(t.Key) + "]" + formatTypeExpr(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.Ellipsis:
		return "..." + formatTypeExpr(t.Elt)
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + formatTypeExpr(t.Value)
	default:
		return "any"
	}
}

func extractDocComment(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	return strings.TrimSpace(cg.Text())
}

func getLineContext(lines []string, lineNum int) string {
	if lineNum < 1 || lineNum > len(lines) {
		return ""
	}
	return lines[lineNum-1]
}

func hasRefAtLine(refs []*Reference, file string, line int) bool {
	for _, r := range refs {
		if r.File == file && r.Line == line {
			return true
		}
	}
	return false
}

func implementsInterface(ifaceMethods, typeMethods []string) bool {
	if len(ifaceMethods) == 0 {
		return false
	}
	methodSet := make(map[string]bool, len(typeMethods))
	for _, m := range typeMethods {
		methodSet[m] = true
	}
	for _, m := range ifaceMethods {
		if !methodSet[m] {
			return false
		}
	}
	return true
}

func navAppendUnique(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}

// fuzzyMatch checks if query characters appear in order within target.
func fuzzyMatch(query, target string) bool {
	if query == "" {
		return true
	}
	qi := 0
	for i := 0; i < len(target) && qi < len(query); i++ {
		if target[i] == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}
