// Package repomap: doclint.go checks Go (and selectively other) source
// files for missing, outdated, incomplete, or unclear doc comments on
// exported symbols, and produces a per-file DocLintResult with a score
// and issue list. The result feeds the documentation-coverage dimension
// of the repository health score.
package repomap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// DocLintResult holds the documentation quality analysis for a single file.
type DocLintResult struct {
	File   string
	Issues []DocIssue
	Score  float64
	Stats  DocStats
}

// DocIssue represents a single documentation problem found in a file.
type DocIssue struct {
	Line       int
	Symbol     string
	Type       string // "missing", "outdated", "incomplete", "unclear"
	Message    string
	Severity   string // "error", "warning", "info"
	Suggestion string
}

// DocStats holds aggregate documentation statistics for a file.
type DocStats struct {
	TotalExported int
	Documented    int
	Coverage      float64
	AvgLength     int
	MissingCount  int
}

// DocLinter performs documentation quality analysis on Go source files.
type DocLinter struct {
	MinCommentLength int
	RequireExported  bool
	mu               sync.RWMutex
}

// NewDocLinter creates a DocLinter with sensible default configuration.
func NewDocLinter() *DocLinter {
	return &DocLinter{
		MinCommentLength: 10,
		RequireExported:  true,
	}
}

// LintFile analyzes the documentation quality of a single Go source file.
func (dl *DocLinter) LintFile(path, content string) (*DocLintResult, error) {
	dl.mu.RLock()
	minLen := dl.MinCommentLength
	requireExported := dl.RequireExported
	dl.mu.RUnlock()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	result := &DocLintResult{
		File: path,
	}

	var totalExported int
	var documented int
	var commentLengths []int

	// Check package doc
	if f.Doc != nil {
		pkgComment := f.Doc.Text()
		commentLengths = append(commentLengths, len(pkgComment))
	}

	// Collect all exported symbols and their doc comments
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if !s.Name.IsExported() {
						continue
					}
					if !requireExported {
						continue
					}
					totalExported++
					line := fset.Position(s.Pos()).Line
					comment := ""
					if d.Doc != nil {
						comment = d.Doc.Text()
					}
					issues := dl.checkGoDoc(s.Name.Name, comment, minLen)
					if len(issues) == 0 {
						documented++
						commentLengths = append(commentLengths, len(comment))
					}
					for i := range issues {
						issues[i].Line = line
					}
					result.Issues = append(result.Issues, issues...)

				case *ast.ValueSpec:
					for _, name := range s.Names {
						if !name.IsExported() {
							continue
						}
						if !requireExported {
							continue
						}
						totalExported++
						line := fset.Position(name.Pos()).Line
						comment := ""
						if d.Doc != nil {
							comment = d.Doc.Text()
						} else if s.Doc != nil {
							comment = s.Doc.Text()
						}
						issues := dl.checkGoDoc(name.Name, comment, minLen)
						if len(issues) == 0 {
							documented++
							commentLengths = append(commentLengths, len(comment))
						}
						for i := range issues {
							issues[i].Line = line
						}
						result.Issues = append(result.Issues, issues...)
					}
				}
			}

		case *ast.FuncDecl:
			name := d.Name.Name
			if !ast.IsExported(name) {
				continue
			}
			if !requireExported {
				continue
			}
			totalExported++
			line := fset.Position(d.Pos()).Line
			comment := ""
			if d.Doc != nil {
				comment = d.Doc.Text()
			}

			issues := dl.checkGoDoc(name, comment, minLen)
			if len(issues) == 0 {
				documented++
				commentLengths = append(commentLengths, len(comment))
			}
			for i := range issues {
				issues[i].Line = line
			}
			result.Issues = append(result.Issues, issues...)
		}
	}

	// Compute stats
	var avgLen int
	if len(commentLengths) > 0 {
		total := 0
		for _, l := range commentLengths {
			total += l
		}
		avgLen = total / len(commentLengths)
	}

	var coverage float64
	if totalExported > 0 {
		coverage = float64(documented) / float64(totalExported) * 100
	}

	missingCount := 0
	for _, issue := range result.Issues {
		if issue.Type == "missing" {
			missingCount++
		}
	}

	result.Stats = DocStats{
		TotalExported: totalExported,
		Documented:    documented,
		Coverage:      coverage,
		AvgLength:     avgLen,
		MissingCount:  missingCount,
	}

	// Compute score (0-100)
	result.Score = dl.computeScore(result)

	return result, nil
}

// LintDirectory walks all Go files in a directory and lints each one.
func (dl *DocLinter) LintDirectory(dir string) ([]*DocLintResult, error) {
	var results []*DocLintResult

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip hidden directories and vendor
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && path != dir {
				return filepath.SkipDir
			}
			if base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		result, lintErr := dl.LintFile(path, string(content))
		if lintErr != nil {
			return nil
		}
		results = append(results, result)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return results, nil
}

// CheckGoDoc inspects a doc comment for an exported symbol and returns issues.
func (dl *DocLinter) CheckGoDoc(name, comment string) []DocIssue {
	dl.mu.RLock()
	minLen := dl.MinCommentLength
	dl.mu.RUnlock()
	return dl.checkGoDoc(name, comment, minLen)
}

// checkGoDoc is the internal implementation for checking doc comments.
func (dl *DocLinter) checkGoDoc(name, comment string, minLen int) []DocIssue {
	var issues []DocIssue

	comment = strings.TrimSpace(comment)

	// Check: missing comment
	if comment == "" {
		issues = append(issues, DocIssue{
			Symbol:     name,
			Type:       "missing",
			Message:    fmt.Sprintf("%s — missing doc comment", name),
			Severity:   "error",
			Suggestion: fmt.Sprintf("// %s ...", name),
		})
		return issues
	}

	// Check: bad prefix (Go convention: comment starts with symbol name)
	firstLine := strings.SplitN(comment, "\n", 2)[0]
	if !strings.HasPrefix(firstLine, name+" ") && !strings.HasPrefix(firstLine, name+".") {
		issues = append(issues, DocIssue{
			Symbol:   name,
			Type:     "incomplete",
			Message:  fmt.Sprintf("%s — comment does not start with symbol name", name),
			Severity: "warning",
			Suggestion: fmt.Sprintf("// %s %s", name,
				strings.TrimPrefix(firstLine, "// ")),
		})
	}

	// Check: too short
	if len(comment) < minLen {
		issues = append(issues, DocIssue{
			Symbol:     name,
			Type:       "incomplete",
			Message:    fmt.Sprintf("%s — comment too short (%d chars, min %d)", name, len(comment), minLen),
			Severity:   "warning",
			Suggestion: fmt.Sprintf("// %s provides ... (add more detail)", name),
		})
	}

	// Check: trivial/restates name
	if isTrivialComment(name, comment) {
		issues = append(issues, DocIssue{
			Symbol:     name,
			Type:       "unclear",
			Message:    fmt.Sprintf("%s — trivial comment that restates the name", name),
			Severity:   "warning",
			Suggestion: fmt.Sprintf("// %s ... (describe behavior, not just restate name)", name),
		})
	}

	return issues
}

// isTrivialComment detects comments that just restate the symbol name.
// e.g., "Handler handles" or "Process processes the input"
func isTrivialComment(name, comment string) bool {
	comment = strings.TrimSpace(comment)
	lower := strings.ToLower(comment)
	nameLower := strings.ToLower(name)

	// Pattern: "X is a x" / "X is an x" / "X is the x"
	trivialIdentity := []string{
		nameLower + " is a " + nameLower,
		nameLower + " is an " + nameLower,
		nameLower + " is the " + nameLower,
	}
	for _, pattern := range trivialIdentity {
		if strings.HasPrefix(lower, pattern) {
			return true
		}
	}

	// Derive possible verbs from the name
	verbs := deriveVerbs(name)

	// Check if comment is just "Name <verb>" or "Name <verb>s ..."
	words := strings.Fields(lower)
	if len(words) >= 2 && words[0] == nameLower {
		secondWord := words[1]
		for _, v := range verbs {
			if secondWord == v || secondWord == v+"s" || secondWord == v+"es" || secondWord == v+"ed" {
				// Trivial if it's just two words, or very short
				if len(words) <= 3 {
					return true
				}
			}
		}
	}

	return false
}

// deriveVerbs extracts possible verb forms from a symbol name.
// e.g., "Handler" -> ["handle"], "Process" -> ["process"], "Validator" -> ["validate"]
func deriveVerbs(name string) []string {
	parts := splitCamelCase(name)
	if len(parts) == 0 {
		return []string{strings.ToLower(name)}
	}

	first := strings.ToLower(parts[0])
	var verbs []string

	// Common noun->verb transformations
	switch {
	case strings.HasSuffix(first, "handler"):
		verbs = append(verbs, strings.TrimSuffix(first, "r"))
	case strings.HasSuffix(first, "er"):
		base := strings.TrimSuffix(first, "er")
		verbs = append(verbs, base)
		// "handler" -> "handle" (add e)
		verbs = append(verbs, base+"e")
	case strings.HasSuffix(first, "or"):
		base := strings.TrimSuffix(first, "or")
		verbs = append(verbs, base)
		verbs = append(verbs, base+"e")
	case strings.HasSuffix(first, "ion"):
		base := strings.TrimSuffix(first, "ion")
		verbs = append(verbs, base)
		verbs = append(verbs, base+"e")
	default:
		verbs = append(verbs, first)
	}

	return verbs
}

var camelSplitRe = regexp.MustCompile(`[A-Z][^A-Z]*`)

// splitCamelCase splits a CamelCase identifier into words.
func splitCamelCase(s string) []string {
	matches := camelSplitRe.FindAllString(s, -1)
	if len(matches) == 0 {
		return []string{s}
	}
	return matches
}

// SuggestDocComment generates a suggested doc comment for a given symbol.
func (dl *DocLinter) SuggestDocComment(name, kind, signature string) string {
	switch kind {
	case "func", "method":
		return dl.suggestFuncDoc(name, signature)
	case "type":
		return dl.suggestTypeDoc(name, signature)
	case "var", "const":
		return fmt.Sprintf("// %s defines ...", name)
	default:
		return fmt.Sprintf("// %s ...", name)
	}
}

// suggestFuncDoc generates a doc comment suggestion for a function.
func (dl *DocLinter) suggestFuncDoc(name, signature string) string {
	verb := inferVerb(name)
	object := inferObject(name)

	// Parse return types from signature for richer suggestions
	returns := extractReturns(signature)

	var sb strings.Builder
	sb.WriteString("// ")
	sb.WriteString(name)
	sb.WriteString(" ")
	sb.WriteString(verb)
	sb.WriteString("s")

	if object != "" {
		sb.WriteString(" the given ")
		sb.WriteString(object)
	}

	if returns != "" {
		sb.WriteString(" and returns ")
		sb.WriteString(returns)
	}

	sb.WriteString(".")
	return sb.String()
}

// suggestTypeDoc generates a doc comment suggestion for a type.
func (dl *DocLinter) suggestTypeDoc(name, signature string) string {
	if strings.Contains(signature, "interface") {
		return fmt.Sprintf("// %s defines the interface for ...", name)
	}
	if strings.Contains(signature, "struct") {
		return fmt.Sprintf("// %s represents ...", name)
	}
	return fmt.Sprintf("// %s defines ...", name)
}

// inferVerb extracts the verb portion from a function name.
func inferVerb(name string) string {
	parts := splitCamelCase(name)
	if len(parts) == 0 {
		return "process"
	}
	verb := strings.ToLower(parts[0])
	// Remove trailing 's' if present (e.g., "Gets" -> "get")
	verb = strings.TrimSuffix(verb, "s")
	return verb
}

// inferObject extracts the object portion from a function name.
func inferObject(name string) string {
	parts := splitCamelCase(name)
	if len(parts) <= 1 {
		return ""
	}
	// Join remaining parts as lowercase
	objParts := make([]string, len(parts)-1)
	for i, p := range parts[1:] {
		objParts[i] = strings.ToLower(p)
	}
	return strings.Join(objParts, " ")
}

// extractReturns parses a function signature to extract return type info.
func extractReturns(signature string) string {
	// Find the parameter list end by matching parens
	// We need to find the closing paren that matches the opening paren after "func Name"
	paramStart := strings.Index(signature, "(")
	if paramStart < 0 {
		return ""
	}

	// Find matching closing paren
	depth := 0
	paramEnd := -1
scanLoop:
	for i := paramStart; i < len(signature); i++ {
		switch signature[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				paramEnd = i
				break scanLoop
			}
		}
	}
	if paramEnd < 0 || paramEnd >= len(signature)-1 {
		return ""
	}

	ret := strings.TrimSpace(signature[paramEnd+1:])
	if ret == "" {
		return ""
	}
	// Clean up parens for multi-return
	ret = strings.TrimPrefix(ret, "(")
	ret = strings.TrimSuffix(ret, ")")
	ret = strings.TrimSpace(ret)

	if ret == "error" {
		return "an error if the operation fails"
	}
	if strings.Contains(ret, "error") {
		parts := strings.Split(ret, ",")
		nonErr := []string{}
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "error" {
				nonErr = append(nonErr, p)
			}
		}
		if len(nonErr) > 0 {
			return strings.Join(nonErr, ", ")
		}
	}
	return ret
}

// computeScore calculates a documentation quality score from 0 to 100.
func (dl *DocLinter) computeScore(result *DocLintResult) float64 {
	if result.Stats.TotalExported == 0 {
		return 100.0 // No exported symbols, nothing to document
	}

	// Base score from coverage (70% weight)
	coverageScore := result.Stats.Coverage * 0.7

	// Penalty for issues (30% weight)
	issuesPenalty := 0.0
	for _, issue := range result.Issues {
		switch issue.Severity {
		case "error":
			issuesPenalty += 10
		case "warning":
			issuesPenalty += 5
		case "info":
			issuesPenalty += 2
		}
	}

	// Cap penalty at 30
	if issuesPenalty > 30 {
		issuesPenalty = 30
	}

	score := coverageScore + (30 - issuesPenalty)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// FormatReport produces a human-readable documentation lint report.
func (dl *DocLinter) FormatReport(results []*DocLintResult) string {
	if len(results) == 0 {
		return "Documentation Lint: No files analyzed."
	}

	var sb strings.Builder
	sb.WriteString("Documentation Lint:\n")
	sb.WriteString("═══════════════════════════════\n\n")

	// Sort by score ascending (worst first)
	sorted := make([]*DocLintResult, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score < sorted[j].Score
	})

	totalMissing := 0
	totalTrivial := 0
	totalScore := 0.0

	for _, r := range sorted {
		totalScore += r.Score
		sb.WriteString(fmt.Sprintf("%s (Score: %.0f/100)\n", r.File, r.Score))

		for _, issue := range r.Issues {
			icon := icons.Alert()
			switch issue.Severity {
			case "error":
				icon = icons.CloseThick()
			case "info":
				icon = icons.CheckBold()
			}
			sb.WriteString(fmt.Sprintf("  %s L%d: %s\n", icon, issue.Line, issue.Message))

			switch issue.Type {
			case "missing":
				totalMissing++
			case "unclear":
				totalTrivial++
			}
		}

		if len(r.Issues) == 0 {
			sb.WriteString("  " + icons.CheckBold() + " All exported symbols documented\n")
		}
		sb.WriteString("\n")
	}

	avgScore := totalScore / float64(len(results))
	sb.WriteString(fmt.Sprintf("Summary: %d files, avg score %.0f/100, %d missing, %d trivial\n",
		len(results), avgScore, totalMissing, totalTrivial))

	return sb.String()
}

// CoverageReport produces an overall documentation coverage summary.
func (dl *DocLinter) CoverageReport(results []*DocLintResult) string {
	if len(results) == 0 {
		return "Documentation Coverage: No files analyzed."
	}

	var sb strings.Builder
	sb.WriteString("Documentation Coverage Report\n")
	sb.WriteString("─────────────────────────────\n\n")

	totalExported := 0
	totalDocumented := 0

	type fileEntry struct {
		path     string
		coverage float64
		exported int
		docced   int
	}
	var entries []fileEntry

	for _, r := range results {
		totalExported += r.Stats.TotalExported
		totalDocumented += r.Stats.Documented
		entries = append(entries, fileEntry{
			path:     r.File,
			coverage: r.Stats.Coverage,
			exported: r.Stats.TotalExported,
			docced:   r.Stats.Documented,
		})
	}

	// Sort by coverage ascending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].coverage < entries[j].coverage
	})

	for _, e := range entries {
		bar := coverageBar(e.coverage)
		sb.WriteString(fmt.Sprintf("  %s %5.1f%% %s (%d/%d)\n",
			bar, e.coverage, e.path, e.docced, e.exported))
	}

	sb.WriteString("\n")
	overallCoverage := 0.0
	if totalExported > 0 {
		overallCoverage = float64(totalDocumented) / float64(totalExported) * 100
	}
	sb.WriteString(fmt.Sprintf("Overall: %.1f%% (%d/%d exported symbols documented)\n",
		overallCoverage, totalDocumented, totalExported))

	return sb.String()
}

// coverageBar produces a visual bar for coverage percentage.
func coverageBar(pct float64) string {
	const width = 20
	filled := int(pct / 100 * width)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}
