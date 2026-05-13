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

// SemanticDiff holds the result of semantic analysis of a diff.
type SemanticDiff struct {
	Changes      []SemanticChange
	Summary      string
	RiskLevel    string
	AffectedAPIs []string
}

// SemanticChange describes a single semantic change detected in a diff.
type SemanticChange struct {
	File        string
	Type        string // "function_added", "function_removed", "function_modified", "type_changed", "import_added", "import_removed", "signature_changed", "behavior_changed"
	Name        string
	Description string
	Breaking    bool
	Risk        string
}

// SignatureChange describes how a function signature changed between versions.
type SignatureChange struct {
	Name           string
	ParamsAdded    []string
	ParamsRemoved  []string
	ParamsReordered bool
	ReturnChanged  bool
	OldReturn      string
	NewReturn      string
	ReceiverChanged bool
	OldReceiver    string
	NewReceiver    string
}

// SemanticAnalyzer performs semantic diff analysis on Go source code.
type SemanticAnalyzer struct {
	routePatterns []*regexp.Regexp
}

// NewSemanticAnalyzer creates a new SemanticAnalyzer instance.
func NewSemanticAnalyzer() *SemanticAnalyzer {
	return &SemanticAnalyzer{
		routePatterns: []*regexp.Regexp{
			regexp.MustCompile(`\.Handle(?:Func)?\s*\(\s*"([^"]+)"`),
			regexp.MustCompile(`\.(?:GET|POST|PUT|DELETE|PATCH)\s*\(\s*"([^"]+)"`),
			regexp.MustCompile(`\.Route\s*\(\s*"([^"]+)"`),
			regexp.MustCompile(`\.Path\s*\(\s*"([^"]+)"`),
			regexp.MustCompile(`"(\/api\/[^"]+)"`),
		},
	}
}

// AnalyzeDiff parses a unified diff and performs semantic analysis on Go code changes.
func (sa *SemanticAnalyzer) AnalyzeDiff(diff string) (*SemanticDiff, error) {
	if diff == "" {
		return &SemanticDiff{
			Changes:      []SemanticChange{},
			RiskLevel:    "low",
			AffectedAPIs: []string{},
		}, nil
	}

	files := parseSemanticDiffFiles(diff)
	var allChanges []SemanticChange

	for _, f := range files {
		changes := sa.analyzeFileChanges(f)
		allChanges = append(allChanges, changes...)
	}

	result := &SemanticDiff{
		Changes:   allChanges,
		RiskLevel: ClassifyRisk(allChanges),
	}

	// Find affected APIs using the full diff content
	result.AffectedAPIs = sa.FindAffectedAPIs(allChanges, diff)
	result.Summary = GenerateSummary(result)

	return result, nil
}

// diffFile holds parsed information about a single file's changes in the diff.
type diffFile struct {
	path       string
	oldContent string
	newContent string
	addedLines []string
	removedLines []string
}

// parseSemanticDiffFiles extracts per-file old/new content from a unified diff.
func parseSemanticDiffFiles(diff string) []diffFile {
	var files []diffFile
	lines := strings.Split(diff, "\n")

	var currentFile *diffFile
	var oldLines []string
	var newLines []string
	inHunk := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(line, "--- a/") {
			// Start of a new file pair
			if currentFile != nil {
				currentFile.oldContent = strings.Join(oldLines, "\n")
				currentFile.newContent = strings.Join(newLines, "\n")
				files = append(files, *currentFile)
			}
			oldLines = nil
			newLines = nil
			inHunk = false
			currentFile = &diffFile{}
			continue
		}

		if strings.HasPrefix(line, "+++ b/") && currentFile != nil {
			currentFile.path = strings.TrimPrefix(line, "+++ b/")
			continue
		}

		if strings.HasPrefix(line, "@@") {
			inHunk = true
			continue
		}

		if !inHunk || currentFile == nil {
			continue
		}

		if strings.HasPrefix(line, "+") {
			content := line[1:]
			newLines = append(newLines, content)
			currentFile.addedLines = append(currentFile.addedLines, content)
		} else if strings.HasPrefix(line, "-") {
			content := line[1:]
			oldLines = append(oldLines, content)
			currentFile.removedLines = append(currentFile.removedLines, content)
		} else if strings.HasPrefix(line, " ") {
			content := line[1:]
			oldLines = append(oldLines, content)
			newLines = append(newLines, content)
		}
	}

	if currentFile != nil {
		currentFile.oldContent = strings.Join(oldLines, "\n")
		currentFile.newContent = strings.Join(newLines, "\n")
		files = append(files, *currentFile)
	}

	return files
}

// analyzeFileChanges performs semantic analysis on a single file's changes.
func (sa *SemanticAnalyzer) analyzeFileChanges(f diffFile) []SemanticChange {
	var changes []SemanticChange

	if !strings.HasSuffix(f.path, ".go") {
		// For non-Go files, produce basic textual analysis
		if len(f.addedLines) > 0 && len(f.removedLines) == 0 {
			changes = append(changes, SemanticChange{
				File:        f.path,
				Type:        "function_added",
				Name:        f.path,
				Description: fmt.Sprintf("Added %d lines", len(f.addedLines)),
				Breaking:    false,
				Risk:        "low",
			})
		} else if len(f.removedLines) > 0 && len(f.addedLines) == 0 {
			changes = append(changes, SemanticChange{
				File:        f.path,
				Type:        "function_removed",
				Name:        f.path,
				Description: fmt.Sprintf("Removed %d lines", len(f.removedLines)),
				Breaking:    true,
				Risk:        "high",
			})
		} else if len(f.addedLines) > 0 && len(f.removedLines) > 0 {
			changes = append(changes, SemanticChange{
				File:        f.path,
				Type:        "function_modified",
				Name:        f.path,
				Description: fmt.Sprintf("Modified: +%d/-%d lines", len(f.addedLines), len(f.removedLines)),
				Breaking:    false,
				Risk:        "medium",
			})
		}
		return changes
	}

	// Go file: use AST-based analysis
	breaking := DetectBreakingChanges(f.oldContent, f.newContent)
	changes = append(changes, breaking...)

	behavior := DetectBehaviorChanges(f.oldContent, f.newContent)
	changes = append(changes, behavior...)

	// Detect import changes
	importChanges := detectImportChanges(f.oldContent, f.newContent)
	changes = append(changes, importChanges...)

	// Detect added functions (not detected as breaking or behavior)
	added := detectAddedFunctions(f.oldContent, f.newContent)
	changes = append(changes, added...)

	// Set file path on all changes
	for i := range changes {
		if changes[i].File == "" {
			changes[i].File = f.path
		}
	}

	return changes
}

// DetectBreakingChanges compares old and new Go source content and returns semantic
// changes that represent breaking API changes.
func DetectBreakingChanges(oldContent, newContent string) []SemanticChange {
	var changes []SemanticChange

	if oldContent == "" {
		return changes
	}

	oldFuncs := parseFunctions(oldContent)
	newFuncs := parseFunctions(newContent)

	oldTypes := parseTypes(oldContent)
	newTypes := parseTypes(newContent)

	oldInterfaces := parseInterfaces(oldContent)
	newInterfaces := parseInterfaces(newContent)

	// Check for removed exported functions
	for name, oldSig := range oldFuncs {
		if !isExported(name) {
			continue
		}
		if newSig, exists := newFuncs[name]; !exists {
			changes = append(changes, SemanticChange{
				Type:        "function_removed",
				Name:        name,
				Description: fmt.Sprintf("Exported function %s removed (was: %s)", name, oldSig),
				Breaking:    true,
				Risk:        "high",
			})
		} else if oldSig != newSig {
			// Signature changed
			changes = append(changes, SemanticChange{
				Type:        "signature_changed",
				Name:        name,
				Description: fmt.Sprintf("Signature changed: %s -> %s", oldSig, newSig),
				Breaking:    true,
				Risk:        "high",
			})
		}
	}

	// Check for changed exported types
	for name, oldDef := range oldTypes {
		if !isExported(name) {
			continue
		}
		if newDef, exists := newTypes[name]; exists {
			if oldDef != newDef {
				changes = append(changes, SemanticChange{
					Type:        "type_changed",
					Name:        name,
					Description: fmt.Sprintf("Type definition changed for %s", name),
					Breaking:    true,
					Risk:        "high",
				})
			}
		} else {
			changes = append(changes, SemanticChange{
				Type:        "type_changed",
				Name:        name,
				Description: fmt.Sprintf("Exported type %s removed", name),
				Breaking:    true,
				Risk:        "high",
			})
		}
	}

	// Check for interface method additions (breaks implementors)
	for name, oldMethods := range oldInterfaces {
		if !isExported(name) {
			continue
		}
		if newMethods, exists := newInterfaces[name]; exists {
			for methodName := range newMethods {
				if _, had := oldMethods[methodName]; !had {
					changes = append(changes, SemanticChange{
						Type:        "type_changed",
						Name:        name + "." + methodName,
						Description: fmt.Sprintf("Method %s added to interface %s (breaks existing implementors)", methodName, name),
						Breaking:    true,
						Risk:        "high",
					})
				}
			}
		}
	}

	return changes
}

// DetectBehaviorChanges detects changes in code behavior such as error handling,
// nil checks, loop bounds, and conditional logic.
func DetectBehaviorChanges(oldContent, newContent string) []SemanticChange {
	var changes []SemanticChange

	if oldContent == "" || newContent == "" {
		return changes
	}

	oldFuncBodies := parseFuncBodies(oldContent)
	newFuncBodies := parseFuncBodies(newContent)

	for name, oldBody := range oldFuncBodies {
		newBody, exists := newFuncBodies[name]
		if !exists {
			continue
		}
		if oldBody == newBody {
			continue
		}

		// Error handling changes
		oldErrors := countPattern(oldBody, `if\s+err\s*!=\s*nil`)
		newErrors := countPattern(newBody, `if\s+err\s*!=\s*nil`)
		if newErrors > oldErrors {
			changes = append(changes, SemanticChange{
				Type:        "behavior_changed",
				Name:        name,
				Description: fmt.Sprintf("Error handling added in %s (%d -> %d checks)", name, oldErrors, newErrors),
				Breaking:    false,
				Risk:        "low",
			})
		} else if newErrors < oldErrors {
			changes = append(changes, SemanticChange{
				Type:        "behavior_changed",
				Name:        name,
				Description: fmt.Sprintf("Error handling removed in %s (%d -> %d checks)", name, oldErrors, newErrors),
				Breaking:    false,
				Risk:        "medium",
			})
		}

		// Nil check changes
		oldNilChecks := countPattern(oldBody, `!=\s*nil|==\s*nil`)
		newNilChecks := countPattern(newBody, `!=\s*nil|==\s*nil`)
		if newNilChecks > oldNilChecks && newErrors == oldErrors {
			changes = append(changes, SemanticChange{
				Type:        "behavior_changed",
				Name:        name,
				Description: fmt.Sprintf("Nil checks added in %s (%d -> %d)", name, oldNilChecks, newNilChecks),
				Breaking:    false,
				Risk:        "low",
			})
		} else if newNilChecks < oldNilChecks && newErrors == oldErrors {
			changes = append(changes, SemanticChange{
				Type:        "behavior_changed",
				Name:        name,
				Description: fmt.Sprintf("Nil checks removed in %s (%d -> %d)", name, oldNilChecks, newNilChecks),
				Breaking:    false,
				Risk:        "medium",
			})
		}

		// Loop bounds changes
		oldLoops := extractLoopBounds(oldBody)
		newLoops := extractLoopBounds(newBody)
		if oldLoops != newLoops && oldLoops != "" && newLoops != "" {
			changes = append(changes, SemanticChange{
				Type:        "behavior_changed",
				Name:        name,
				Description: fmt.Sprintf("Loop bounds changed in %s", name),
				Breaking:    false,
				Risk:        "medium",
			})
		}

		// Conditional logic changes
		oldConds := countPattern(oldBody, `if\s+[^{]+{`)
		newConds := countPattern(newBody, `if\s+[^{]+{`)
		if oldConds != newConds && oldErrors == newErrors && oldNilChecks == newNilChecks {
			changes = append(changes, SemanticChange{
				Type:        "behavior_changed",
				Name:        name,
				Description: fmt.Sprintf("Conditional logic changed in %s (%d -> %d branches)", name, oldConds, newConds),
				Breaking:    false,
				Risk:        "medium",
			})
		}
	}

	return changes
}

// ClassifyRisk determines the overall risk level based on a set of semantic changes.
func ClassifyRisk(changes []SemanticChange) string {
	if len(changes) == 0 {
		return "low"
	}

	hasBreaking := false
	hasModification := false
	hasBehaviorChange := false

	for _, c := range changes {
		if c.Breaking {
			hasBreaking = true
		}
		switch c.Type {
		case "function_modified", "signature_changed", "type_changed":
			hasModification = true
		case "behavior_changed":
			hasBehaviorChange = true
		}
	}

	if hasBreaking {
		return "high"
	}
	if hasModification || hasBehaviorChange {
		return "medium"
	}
	return "low"
}

// GenerateSummary produces a human-readable summary of a semantic diff analysis.
func GenerateSummary(diff *SemanticDiff) string {
	var sb strings.Builder

	sb.WriteString("Semantic Analysis:\n")
	sb.WriteString("─────────────────\n")
	sb.WriteString(fmt.Sprintf("Risk: %s\n", strings.ToUpper(diff.RiskLevel)))
	sb.WriteString("\nChanges:\n")

	additions := 0
	modifications := 0
	breaking := 0

	for _, c := range diff.Changes {
		if c.Breaking {
			breaking++
		}
		switch c.Type {
		case "function_added", "import_added":
			additions++
			sb.WriteString(fmt.Sprintf("+ Added: %s\n", formatChangeLine(c)))
		case "function_removed", "import_removed":
			if c.Breaking {
				sb.WriteString(fmt.Sprintf("⚠ Breaking: %s\n", formatChangeLine(c)))
			} else {
				sb.WriteString(fmt.Sprintf("- Removed: %s\n", formatChangeLine(c)))
			}
		case "function_modified", "behavior_changed":
			modifications++
			sb.WriteString(fmt.Sprintf("~ Modified: %s\n", formatChangeLine(c)))
		case "signature_changed":
			sb.WriteString(fmt.Sprintf("⚠ Breaking: %s\n", formatChangeLine(c)))
		case "type_changed":
			if c.Breaking {
				sb.WriteString(fmt.Sprintf("⚠ Breaking: %s\n", formatChangeLine(c)))
			} else {
				modifications++
				sb.WriteString(fmt.Sprintf("~ Modified: %s\n", formatChangeLine(c)))
			}
		}
	}

	if len(diff.AffectedAPIs) > 0 {
		sb.WriteString(fmt.Sprintf("\nAffected APIs: %s\n", strings.Join(diff.AffectedAPIs, ", ")))
	}

	sb.WriteString(fmt.Sprintf("Impact: %d breaking changes, %d additions, %d modifications\n",
		breaking, additions, modifications))

	return sb.String()
}

// FindAffectedAPIs traces changed functions to HTTP handlers/routes in the content.
func (sa *SemanticAnalyzer) FindAffectedAPIs(changes []SemanticChange, content string) []string {
	apiSet := make(map[string]bool)

	// Extract all route definitions from content
	routes := sa.extractRoutes(content)

	// For each changed function, check if it's referenced in route handlers
	for _, c := range changes {
		name := c.Name
		// Check if this function name appears near a route definition
		for route, handlers := range routes {
			for _, handler := range handlers {
				if handler == name || strings.Contains(handler, name) {
					apiSet[route] = true
				}
			}
		}

		// Also check direct mentions in route registrations
		for _, pattern := range sa.routePatterns {
			// Find routes that are on lines containing or near the function name
			lines := strings.Split(content, "\n")
			for i, line := range lines {
				matches := pattern.FindStringSubmatch(line)
				if len(matches) > 1 {
					// Check nearby lines for the function name
					start := i - 5
					if start < 0 {
						start = 0
					}
					end := i + 5
					if end > len(lines) {
						end = len(lines)
					}
					contextBlock := strings.Join(lines[start:end], "\n")
					if strings.Contains(contextBlock, name) {
						apiSet[matches[1]] = true
					}
				}
			}
		}
	}

	var apis []string
	for api := range apiSet {
		apis = append(apis, api)
	}
	sort.Strings(apis)
	return apis
}

// CompareSignatures compares two function signature strings and returns the differences.
func CompareSignatures(oldSig, newSig string) *SignatureChange {
	if oldSig == newSig {
		return nil
	}

	change := &SignatureChange{}

	// Parse function name
	oldName := extractFuncName(oldSig)
	newName := extractFuncName(newSig)
	if oldName != "" {
		change.Name = oldName
	} else {
		change.Name = newName
	}

	// Compare receivers
	oldReceiver := extractReceiver(oldSig)
	newReceiver := extractReceiver(newSig)
	if oldReceiver != newReceiver {
		change.ReceiverChanged = true
		change.OldReceiver = oldReceiver
		change.NewReceiver = newReceiver
	}

	// Compare parameters
	oldParams := extractParams(oldSig)
	newParams := extractParams(newSig)

	oldParamSet := make(map[string]bool)
	newParamSet := make(map[string]bool)
	for _, p := range oldParams {
		oldParamSet[p] = true
	}
	for _, p := range newParams {
		newParamSet[p] = true
	}

	for _, p := range newParams {
		if !oldParamSet[p] {
			change.ParamsAdded = append(change.ParamsAdded, p)
		}
	}
	for _, p := range oldParams {
		if !newParamSet[p] {
			change.ParamsRemoved = append(change.ParamsRemoved, p)
		}
	}

	// Check for reordering (same params but different order)
	if len(change.ParamsAdded) == 0 && len(change.ParamsRemoved) == 0 && len(oldParams) == len(newParams) {
		for i := range oldParams {
			if oldParams[i] != newParams[i] {
				change.ParamsReordered = true
				break
			}
		}
	}

	// Compare return values
	oldReturn := extractReturnType(oldSig)
	newReturn := extractReturnType(newSig)
	if oldReturn != newReturn {
		change.ReturnChanged = true
		change.OldReturn = oldReturn
		change.NewReturn = newReturn
	}

	return change
}

// --- Internal helpers ---

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
		return formatFieldType(node.(ast.Expr))
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
