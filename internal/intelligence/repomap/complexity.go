// Package repomap: complexity.go computes cyclomatic and cognitive
// complexity per function (Go files via go/ast, other languages via
// brace-tracking regex), aggregates a ComplexityReport per file, and
// surfaces refactoring suggestions and a MaintainabilityIndex for the
// health score. FindHotspots walks an entire project and returns the
// highest-complexity functions first.
package repomap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// ComplexityReport holds complexity metrics for a single file.
type ComplexityReport struct {
	File           string
	Functions      []FunctionComplexity
	FileComplexity float64
	LOC            int
	CLOC           int // comment lines
	BlankLines     int
}

// FunctionComplexity holds complexity metrics for a single function.
type FunctionComplexity struct {
	Name       string
	StartLine  int
	EndLine    int
	Cyclomatic int
	Cognitive  int
	LOC        int
	Parameters int
	Returns    int
	Nesting    int // max depth
}

// ComplexityAnalyzer computes complexity metrics with configurable thresholds.
type ComplexityAnalyzer struct {
	HighCyclomatic int
	HighCognitive  int
	HighNesting    int
	HighLOC        int
}

// NewComplexityAnalyzer creates a ComplexityAnalyzer with default thresholds.
func NewComplexityAnalyzer() *ComplexityAnalyzer {
	return &ComplexityAnalyzer{
		HighCyclomatic: 10,
		HighCognitive:  15,
		HighNesting:    4,
		HighLOC:        50,
	}
}

// AnalyzeFile analyzes the complexity of a file given its path and content.
func (ca *ComplexityAnalyzer) AnalyzeFile(path, content string) (*ComplexityReport, error) {
	report := &ComplexityReport{
		File: path,
	}

	// Count LOC, CLOC, BlankLines
	lines := strings.Split(content, "\n")
	report.LOC = len(lines)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			report.BlankLines++
		} else if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			report.CLOC++
		}
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		report.Functions = ca.AnalyzeGoAST(content)
	default:
		lang := languageFromExt(ext)
		report.Functions = ca.AnalyzeGeneric(content, lang)
	}

	// Calculate file-level average cyclomatic complexity
	if len(report.Functions) > 0 {
		total := 0
		for _, fn := range report.Functions {
			total += fn.Cyclomatic
		}
		report.FileComplexity = float64(total) / float64(len(report.Functions))
	}

	return report, nil
}

// AnalyzeGoAST performs full AST-based complexity analysis on Go source code.
func (ca *ComplexityAnalyzer) AnalyzeGoAST(content string) []FunctionComplexity {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", content, parser.ParseComments)
	if err != nil {
		return nil
	}

	var results []FunctionComplexity

	ast.Inspect(f, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			fc := analyzeGoFunc(fset, fn)
			results = append(results, fc)
			return false
		}
		return true
	})

	return results
}

// analyzeGoFunc computes complexity metrics for a single Go function declaration.
func analyzeGoFunc(fset *token.FileSet, fn *ast.FuncDecl) FunctionComplexity {
	name := fn.Name.Name
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		// Include receiver type in name for methods
		if t, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
			if ident, ok := t.X.(*ast.Ident); ok {
				name = ident.Name + "." + name
			}
		} else if ident, ok := fn.Recv.List[0].Type.(*ast.Ident); ok {
			name = ident.Name + "." + name
		}
	}

	startLine := fset.Position(fn.Pos()).Line
	endLine := fset.Position(fn.End()).Line

	fc := FunctionComplexity{
		Name:      name,
		StartLine: startLine,
		EndLine:   endLine,
		LOC:       endLine - startLine + 1,
	}

	// Count parameters
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			if len(field.Names) == 0 {
				fc.Parameters++
			} else {
				fc.Parameters += len(field.Names)
			}
		}
	}

	// Count returns
	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			if len(field.Names) == 0 {
				fc.Returns++
			} else {
				fc.Returns += len(field.Names)
			}
		}
	}

	// Calculate cyclomatic and cognitive complexity
	if fn.Body != nil {
		cc, cog, maxNest := walkComplexity(fn.Body, 0)
		fc.Cyclomatic = cc + 1 // Base complexity is 1
		fc.Cognitive = cog
		fc.Nesting = maxNest
	} else {
		fc.Cyclomatic = 1
	}

	return fc
}

// walkComplexity recursively walks an AST block computing cyclomatic increments,
// cognitive complexity, and max nesting depth.
func walkComplexity(node ast.Node, depth int) (cyclomatic, cognitive, maxNesting int) {
	maxNesting = depth

	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		switch stmt := n.(type) {
		case *ast.IfStmt:
			cyclomatic++
			cognitive += 1 + depth
			// Walk the condition to count logical operators
			cc0, cog0, _ := countBinaryOps(stmt.Cond)
			cyclomatic += cc0
			cognitive += cog0
			// Walk the body at increased depth
			cc, cog, mn := walkBlocksAtDepth(stmt.Body, depth+1)
			cyclomatic += cc
			cognitive += cog
			if mn > maxNesting {
				maxNesting = mn
			}
			// Handle else
			if stmt.Else != nil {
				cognitive++ // break in linear flow
				cc2, cog2, mn2 := walkElse(stmt.Else, depth+1)
				cyclomatic += cc2
				cognitive += cog2
				if mn2 > maxNesting {
					maxNesting = mn2
				}
			}
			return false

		case *ast.ForStmt:
			cyclomatic++
			cognitive += 1 + depth
			// Walk the condition
			if stmt.Cond != nil {
				cc0, cog0, _ := countBinaryOps(stmt.Cond)
				cyclomatic += cc0
				cognitive += cog0
			}
			cc, cog, mn := walkBlocksAtDepth(stmt.Body, depth+1)
			cyclomatic += cc
			cognitive += cog
			if mn > maxNesting {
				maxNesting = mn
			}
			return false

		case *ast.RangeStmt:
			cyclomatic++
			cognitive += 1 + depth
			cc, cog, mn := walkBlocksAtDepth(stmt.Body, depth+1)
			cyclomatic += cc
			cognitive += cog
			if mn > maxNesting {
				maxNesting = mn
			}
			return false

		case *ast.SwitchStmt:
			cyclomatic++
			cognitive += 1 + depth
			cc, cog, mn := walkBlocksAtDepth(stmt.Body, depth+1)
			cyclomatic += cc
			cognitive += cog
			if mn > maxNesting {
				maxNesting = mn
			}
			return false

		case *ast.TypeSwitchStmt:
			cyclomatic++
			cognitive += 1 + depth
			cc, cog, mn := walkBlocksAtDepth(stmt.Body, depth+1)
			cyclomatic += cc
			cognitive += cog
			if mn > maxNesting {
				maxNesting = mn
			}
			return false

		case *ast.SelectStmt:
			cyclomatic++
			cognitive += 1 + depth
			cc, cog, mn := walkBlocksAtDepth(stmt.Body, depth+1)
			cyclomatic += cc
			cognitive += cog
			if mn > maxNesting {
				maxNesting = mn
			}
			return false

		case *ast.CaseClause:
			if stmt.List != nil { // non-default case
				cyclomatic++
			}
			for _, s := range stmt.Body {
				cc, cog, mn := walkComplexity(s, depth+1)
				cyclomatic += cc
				cognitive += cog
				if mn > maxNesting {
					maxNesting = mn
				}
			}
			return false

		case *ast.CommClause:
			if stmt.Comm != nil { // non-default case
				cyclomatic++
			}
			for _, s := range stmt.Body {
				cc, cog, mn := walkComplexity(s, depth+1)
				cyclomatic += cc
				cognitive += cog
				if mn > maxNesting {
					maxNesting = mn
				}
			}
			return false

		case *ast.BinaryExpr:
			if stmt.Op == token.LAND || stmt.Op == token.LOR {
				cyclomatic++
				cognitive++
			}
			return true

		case *ast.FuncLit:
			// Anonymous functions: increase nesting
			if stmt.Body != nil {
				cc, cog, mn := walkBlocksAtDepth(stmt.Body, depth+1)
				cyclomatic += cc
				cognitive += cog
				if mn > maxNesting {
					maxNesting = mn
				}
			}
			return false

		case *ast.FuncDecl:
			// Skip nested func decls (shouldn't happen in Go but be safe)
			return false
		}
		return true
	})

	return cyclomatic, cognitive, maxNesting
}

// countBinaryOps counts logical operators (&&, ||) in an expression.
func countBinaryOps(expr ast.Expr) (cyclomatic, cognitive, maxNesting int) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		if be, ok := n.(*ast.BinaryExpr); ok {
			if be.Op == token.LAND || be.Op == token.LOR {
				cyclomatic++
				cognitive++
			}
		}
		return true
	})
	return
}

// walkBlocksAtDepth walks a block statement at a given nesting depth.
func walkBlocksAtDepth(block *ast.BlockStmt, depth int) (cyclomatic, cognitive, maxNesting int) {
	maxNesting = depth
	for _, stmt := range block.List {
		cc, cog, mn := walkComplexity(stmt, depth)
		cyclomatic += cc
		cognitive += cog
		if mn > maxNesting {
			maxNesting = mn
		}
	}
	return
}

// walkElse handles else branches which may be else-if chains.
func walkElse(node ast.Node, depth int) (cyclomatic, cognitive, maxNesting int) {
	maxNesting = depth
	switch e := node.(type) {
	case *ast.BlockStmt:
		return walkBlocksAtDepth(e, depth)
	case *ast.IfStmt:
		cyclomatic++
		cognitive += 1 + depth
		cc, cog, mn := walkBlocksAtDepth(e.Body, depth+1)
		cyclomatic += cc
		cognitive += cog
		if mn > maxNesting {
			maxNesting = mn
		}
		if e.Else != nil {
			cognitive++
			cc2, cog2, mn2 := walkElse(e.Else, depth+1)
			cyclomatic += cc2
			cognitive += cog2
			if mn2 > maxNesting {
				maxNesting = mn2
			}
		}
	}
	return
}

// AnalyzeGeneric performs regex-based complexity analysis for non-Go languages.
func (ca *ComplexityAnalyzer) AnalyzeGeneric(content, language string) []FunctionComplexity {
	var results []FunctionComplexity

	funcPattern := getFuncPattern(language)
	if funcPattern == nil {
		return results
	}

	lines := strings.Split(content, "\n")
	matches := funcPattern.FindAllStringSubmatchIndex(content, -1)

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		// Get function name from the first non-negative capture group
		nameStart, nameEnd := -1, -1
		for i := 2; i < len(match)-1; i += 2 {
			if match[i] >= 0 && match[i+1] >= 0 {
				nameStart = match[i]
				nameEnd = match[i+1]
				break
			}
		}
		if nameStart < 0 || nameEnd < 0 {
			continue
		}
		name := content[nameStart:nameEnd]

		// Find the start line
		startLine := strings.Count(content[:match[0]], "\n") + 1

		// Find the end of function (heuristic: matching braces or dedent)
		endLine := findFuncEnd(lines, startLine-1, language)

		funcContent := strings.Join(lines[startLine-1:endLine], "\n")

		fc := FunctionComplexity{
			Name:      name,
			StartLine: startLine,
			EndLine:   endLine,
			LOC:       endLine - startLine + 1,
		}

		// Count cyclomatic complexity
		fc.Cyclomatic = countCyclomaticGeneric(funcContent) + 1

		// Count cognitive complexity
		fc.Cognitive = countCognitiveGeneric(funcContent, language)

		// Count parameters
		fc.Parameters = countParameters(content[match[0]:match[1]], language)

		// Count max nesting
		fc.Nesting = countMaxNesting(funcContent, language)

		results = append(results, fc)
	}

	return results
}

// getFuncPattern returns a compiled regex for function declarations in the given language.
func getFuncPattern(language string) *regexp.Regexp {
	switch language {
	case "python":
		return regexp.MustCompile(`(?m)^[ \t]*def\s+(\w+)\s*\([^)]*\)`)
	case "typescript", "javascript":
		return regexp.MustCompile(`(?m)function\s+(\w+)\s*\([^)]*\)`)
	case "rust":
		return regexp.MustCompile(`(?m)^\s*(?:pub\s+)?(?:async\s+)?fn\s+(\w+)\s*\([^)]*\)`)
	case "java", "csharp":
		return regexp.MustCompile(`(?m)(?:public|private|protected|static|\s)+[\w<>\[\]]+\s+(\w+)\s*\([^)]*\)`)
	default:
		return regexp.MustCompile(`(?m)(?:func|function|def|fn)\s+(\w+)\s*\([^)]*\)`)
	}
}

// findFuncEnd finds the ending line of a function using brace/indent tracking.
func findFuncEnd(lines []string, startIdx int, language string) int {
	if language == "python" {
		// Python: find by indentation
		if startIdx >= len(lines) {
			return startIdx + 1
		}
		baseIndent := countIndent(lines[startIdx])
		for i := startIdx + 1; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "" {
				continue
			}
			if countIndent(lines[i]) <= baseIndent {
				return i
			}
		}
		return len(lines)
	}

	// Brace-based languages
	braceCount := 0
	started := false
	for i := startIdx; i < len(lines); i++ {
		for _, ch := range lines[i] {
			if ch == '{' {
				braceCount++
				started = true
			} else if ch == '}' {
				braceCount--
				if started && braceCount == 0 {
					return i + 1
				}
			}
		}
	}
	return min(startIdx+20, len(lines))
}

// countIndent returns the number of leading spaces/tabs in a line.
func countIndent(line string) int {
	count := 0
	for _, ch := range line {
		if ch == ' ' {
			count++
		} else if ch == '\t' {
			count += 4
		} else {
			break
		}
	}
	return count
}

// countCyclomaticGeneric counts decision points in code using regex.
func countCyclomaticGeneric(content string) int {
	count := 0
	patterns := []string{
		`\bif\b`, `\belse\s+if\b`, `\belif\b`,
		`\bfor\b`, `\bwhile\b`,
		`\bcase\b`,
		`\bcatch\b`, `\bexcept\b`,
		`&&`, `\|\|`,
	}
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		count += len(re.FindAllString(content, -1))
	}
	return count
}

// countCognitiveGeneric estimates cognitive complexity for non-Go languages.
func countCognitiveGeneric(content, language string) int {
	cognitive := 0
	lines := strings.Split(content, "\n")

	nestingPatterns := regexp.MustCompile(`\b(if|for|while|switch|match)\b`)
	breakPatterns := regexp.MustCompile(`\b(else|elif|else\s+if|break|continue|goto)\b`)

	depth := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Track nesting via braces (simplified)
		if nestingPatterns.MatchString(trimmed) {
			cognitive += 1 + depth
			depth++
		}
		if breakPatterns.MatchString(trimmed) {
			cognitive++
		}

		// Decrease depth on closing
		braceClose := strings.Count(trimmed, "}")
		braceOpen := strings.Count(trimmed, "{")
		if language == "python" {
			// Python uses dedent - handled differently
			_ = braceClose
		} else {
			if braceClose > braceOpen {
				depth -= (braceClose - braceOpen)
				if depth < 0 {
					depth = 0
				}
			}
		}
	}

	return cognitive
}

// countParameters counts function parameters from a declaration line.
func countParameters(decl, language string) int {
	// Find content inside parentheses
	start := strings.Index(decl, "(")
	end := strings.LastIndex(decl, ")")
	if start < 0 || end < 0 || end <= start+1 {
		return 0
	}
	params := decl[start+1 : end]
	if strings.TrimSpace(params) == "" {
		return 0
	}
	// Count commas + 1
	return strings.Count(params, ",") + 1
}

// countMaxNesting counts the maximum nesting depth in a code block.
func countMaxNesting(content, language string) int {
	if language == "python" {
		return countMaxNestingPython(content)
	}

	maxDepth := 0
	depth := 0
	for _, ch := range content {
		if ch == '{' {
			depth++
			if depth > maxDepth {
				maxDepth = depth
			}
		} else if ch == '}' {
			depth--
		}
	}
	return maxDepth
}

// countMaxNestingPython counts nesting in Python by indentation.
func countMaxNestingPython(content string) int {
	maxDepth := 0
	baseIndent := -1
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := countIndent(line)
		if baseIndent < 0 {
			baseIndent = indent
		}
		depth := (indent - baseIndent) / 4
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	return maxDepth
}

// languageFromExt maps file extensions to language identifiers.
func languageFromExt(ext string) string {
	switch ext {
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".cs":
		return "csharp"
	default:
		return "unknown"
	}
}

// FindHotspots walks all files under dir, analyzes them, and returns
// the top N most complex functions sorted by cyclomatic complexity descending.
func (ca *ComplexityAnalyzer) FindHotspots(dir string, limit int) []FunctionComplexity {
	var mu sync.Mutex
	var allFuncs []FunctionComplexity

	supportedExts := map[string]bool{
		".go": true, ".py": true, ".ts": true, ".tsx": true,
		".js": true, ".jsx": true, ".rs": true, ".java": true, ".cs": true,
	}

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip hidden directories and common non-source dirs
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !supportedExts[ext] {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		report, err := ca.AnalyzeFile(path, string(data))
		if err != nil {
			return nil
		}

		mu.Lock()
		allFuncs = append(allFuncs, report.Functions...)
		mu.Unlock()

		return nil
	})

	// Sort by cyclomatic complexity descending
	sort.Slice(allFuncs, func(i, j int) bool {
		if allFuncs[i].Cyclomatic != allFuncs[j].Cyclomatic {
			return allFuncs[i].Cyclomatic > allFuncs[j].Cyclomatic
		}
		return allFuncs[i].Cognitive > allFuncs[j].Cognitive
	})

	if limit > 0 && limit < len(allFuncs) {
		return allFuncs[:limit]
	}
	return allFuncs
}

// SuggestRefactoring generates refactoring suggestions based on function metrics.
func (ca *ComplexityAnalyzer) SuggestRefactoring(fc FunctionComplexity) []string {
	var suggestions []string

	if fc.Cyclomatic > ca.HighCyclomatic {
		suggestions = append(suggestions,
			fmt.Sprintf("Extract method for each branch (cyclomatic complexity: %d, threshold: %d)", fc.Cyclomatic, ca.HighCyclomatic))
	}

	if fc.Nesting > ca.HighNesting {
		suggestions = append(suggestions,
			fmt.Sprintf("Use early returns to reduce nesting (depth: %d, threshold: %d)", fc.Nesting, ca.HighNesting))
	}

	if fc.Parameters > 4 {
		suggestions = append(suggestions,
			fmt.Sprintf("Group parameters into a struct (%d parameters)", fc.Parameters))
	}

	if fc.LOC > ca.HighLOC {
		suggestions = append(suggestions,
			fmt.Sprintf("Split into smaller focused functions (%d lines exceeds threshold of %d)", fc.LOC, ca.HighLOC))
	}

	if fc.Cognitive > ca.HighCognitive {
		suggestions = append(suggestions,
			fmt.Sprintf("Simplify control flow to reduce cognitive load (cognitive complexity: %d, threshold: %d)", fc.Cognitive, ca.HighCognitive))
	}

	return suggestions
}

// MaintainabilityIndex computes a simplified maintainability index.
// Based on the standard formula: 171 - 5.2*ln(HV) - 0.23*CC - 16.2*ln(LOC)
// Simplified using CC and LOC (Halstead volume approximated from LOC).
func MaintainabilityIndex(fc FunctionComplexity) float64 {
	loc := float64(fc.LOC)
	if loc < 1 {
		loc = 1
	}
	cc := float64(fc.Cyclomatic)

	// Approximate Halstead Volume from LOC (rough heuristic: HV ~ LOC * 5)
	hv := loc * 5.0
	if hv < 1 {
		hv = 1
	}

	mi := 171.0 - 5.2*math.Log(hv) - 0.23*cc - 16.2*math.Log(loc)

	// Clamp to [0, 100] range (normalized)
	if mi < 0 {
		mi = 0
	}
	if mi > 100 {
		mi = 100
	}
	return math.Round(mi*100) / 100
}

// FormatReport formats a ComplexityReport into a human-readable string.
func (ca *ComplexityAnalyzer) FormatReport(report *ComplexityReport) string {
	if report == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Complexity: %s\n", report.File))
	sb.WriteString(strings.Repeat("─", 50) + "\n")
	sb.WriteString(fmt.Sprintf("%-25s %4s %4s %4s %4s\n", "Function", "CC", "Cog", "LOC", "Nest"))

	for _, fn := range report.Functions {
		status := icons.CheckBold()
		if fn.Cyclomatic > ca.HighCyclomatic*2 || fn.Cognitive > ca.HighCognitive*2 {
			status = "CRIT" + " CRITICAL"
		} else if fn.Cyclomatic > ca.HighCyclomatic || fn.Cognitive > ca.HighCognitive {
			status = icons.Alert() + " HIGH"
		}

		name := fn.Name
		if len(name) > 25 {
			name = name[:22] + "..."
		}
		sb.WriteString(fmt.Sprintf("%-25s %4d %4d %4d %4d    %s\n",
			name, fn.Cyclomatic, fn.Cognitive, fn.LOC, fn.Nesting, status))
	}

	sb.WriteString(strings.Repeat("─", 50) + "\n")
	sb.WriteString(fmt.Sprintf("File: CC avg=%.1f, LOC=%d, Functions=%d\n",
		report.FileComplexity, report.LOC, len(report.Functions)))

	// Add suggestions for high-complexity functions
	for _, fn := range report.Functions {
		suggestions := ca.SuggestRefactoring(fn)
		if len(suggestions) > 0 {
			sb.WriteString(fmt.Sprintf("\nSuggestions for %s:\n", fn.Name))
			for _, s := range suggestions {
				sb.WriteString(fmt.Sprintf("  - %s\n", s))
			}
		}
	}

	return sb.String()
}
