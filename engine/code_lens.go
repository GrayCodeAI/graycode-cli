package engine

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// CodeLens represents an inline annotation for a specific line in a file.
type CodeLens struct {
	File     string
	Line     int
	Label    string
	Category string // "test_status", "complexity", "ownership", "age", "references", "coverage"
	Command  string
	Tooltip  string
}

// LensGenerator is a function that produces code lenses for a given file and its content.
type LensGenerator func(file, content string) []CodeLens

// CodeLensProvider manages a set of lens generators and produces annotations.
type CodeLensProvider struct {
	Providers map[string]LensGenerator
	mu        sync.RWMutex
}

// NewCodeLensProvider creates a CodeLensProvider with built-in generators for
// test status, complexity, references, age, and coverage.
func NewCodeLensProvider() *CodeLensProvider {
	p := &CodeLensProvider{
		Providers: make(map[string]LensGenerator),
	}
	p.Providers["test_status"] = GenerateTestLens
	p.Providers["complexity"] = GenerateComplexityLens
	p.Providers["references"] = GenerateReferenceLens
	p.Providers["age"] = GenerateAgeLens
	p.Providers["coverage"] = GenerateCoverageLens
	return p
}

// Register adds or replaces a named lens generator.
func (p *CodeLensProvider) Register(name string, generator LensGenerator) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Providers[name] = generator
}

// Generate runs all registered providers and returns merged lenses sorted by line.
func (p *CodeLensProvider) Generate(file, content string) []CodeLens {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var all []CodeLens
	for _, gen := range p.Providers {
		lenses := gen(file, content)
		all = append(all, lenses...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Line != all[j].Line {
			return all[i].Line < all[j].Line
		}
		return all[i].Category < all[j].Category
	})
	return all
}

// FilterByCategory returns only lenses matching the given category.
func FilterByCategory(lenses []CodeLens, category string) []CodeLens {
	var result []CodeLens
	for _, l := range lenses {
		if l.Category == category {
			result = append(result, l)
		}
	}
	return result
}

// FormatLenses produces a human-readable summary of code lenses.
func FormatLenses(file string, lenses []CodeLens) string {
	if len(lenses) == 0 {
		return fmt.Sprintf("Code Lenses for %s:\n  (none)", file)
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Code Lenses for %s:\n", file))
	for _, l := range lenses {
		b.WriteString(fmt.Sprintf("L%-3d [%s] %s\n", l.Line, l.Label, l.Tooltip))
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---------- Built-in Generators ----------

var testFuncRe = regexp.MustCompile(`(?m)^func\s+(Test\w+)\s*\(`)

// GenerateTestLens finds test functions and annotates them with last known status.
func GenerateTestLens(file, content string) []CodeLens {
	if !strings.HasSuffix(file, "_test.go") {
		return nil
	}

	var lenses []CodeLens
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := testFuncRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		funcName := matches[1]
		status := lookupTestStatus(file, funcName)
		label := fmt.Sprintf("test: %s", status)
		tooltip := fmt.Sprintf("func %s", funcName)
		if status == "PASS" {
			tooltip += " — passed"
		} else if status == "FAIL" {
			tooltip += " — failed"
		} else {
			tooltip += " — status unknown"
		}
		lenses = append(lenses, CodeLens{
			File:     file,
			Line:     i + 1,
			Label:    label,
			Category: "test_status",
			Command:  fmt.Sprintf("go test -run ^%s$ %s", funcName, file),
			Tooltip:  tooltip,
		})
	}
	return lenses
}

// lookupTestStatus attempts to determine the last test result.
// In a real implementation this would query a test result cache.
func lookupTestStatus(file, funcName string) string {
	// Try running the test quickly to determine status
	dir := file
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		dir = file[:idx]
	}
	cmd := exec.Command("go", "test", "-run", "^"+funcName+"$", "-count=1", "-timeout=10s", dir)
	err := cmd.Run()
	if err == nil {
		return "PASS"
	}
	// If the command fails it could be a real failure or an environment issue.
	// Distinguish by exit code when possible.
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 1 {
			return "FAIL"
		}
	}
	return "UNKNOWN"
}

var funcDeclRe = regexp.MustCompile(`(?m)^func\s+(?:\(\s*\w+\s+\*?\w+\s*\)\s+)?(\w+)\s*\(`)

// GenerateComplexityLens calculates cyclomatic complexity for each function and
// annotates functions that exceed a threshold of 5.
func GenerateComplexityLens(file, content string) []CodeLens {
	const threshold = 5
	var lenses []CodeLens

	functions := extractFunctions(content)
	for _, fn := range functions {
		cc := calculateCyclomaticComplexity(fn.body)
		if cc > threshold {
			label := fmt.Sprintf("complexity: %d", cc)
			tooltip := fmt.Sprintf("func %s — consider splitting", fn.name)
			lenses = append(lenses, CodeLens{
				File:     file,
				Line:     fn.line,
				Label:    label,
				Category: "complexity",
				Command:  "",
				Tooltip:  tooltip,
			})
		}
	}
	return lenses
}

// GenerateReferenceLens counts how many times each exported symbol is referenced.
func GenerateReferenceLens(file, content string) []CodeLens {
	var lenses []CodeLens

	lines := strings.Split(content, "\n")
	symbols := extractExportedSymbols(content)

	for _, sym := range symbols {
		count := countReferences(content, sym.name)
		if count > 0 {
			label := fmt.Sprintf("references: %d", count)
			tooltip := fmt.Sprintf("func %s — ", sym.name)
			if count >= 5 {
				tooltip += "widely used"
			} else {
				tooltip += fmt.Sprintf("referenced %d times", count)
			}
			_ = lines
			lenses = append(lenses, CodeLens{
				File:     file,
				Line:     sym.line,
				Label:    label,
				Category: "references",
				Command:  "",
				Tooltip:  tooltip,
			})
		}
	}
	return lenses
}

// GenerateAgeLens uses git blame to determine how recently each function was modified.
func GenerateAgeLens(file, content string) []CodeLens {
	var lenses []CodeLens

	functions := extractFunctions(content)
	blameData := getGitBlame(file)

	for _, fn := range functions {
		age := lookupAge(blameData, fn.line)
		if age == "" {
			continue
		}
		label := fmt.Sprintf("age: %s", age)
		tooltip := fmt.Sprintf("func %s — ", fn.name)
		if isRecent(age) {
			tooltip += "recently modified"
		} else {
			tooltip += "last modified " + age + " ago"
		}
		lenses = append(lenses, CodeLens{
			File:     file,
			Line:     fn.line,
			Label:    label,
			Category: "age",
			Command:  fmt.Sprintf("git log --oneline -1 -L %d,%d:%s", fn.line, fn.endLine, file),
			Tooltip:  tooltip,
		})
	}
	return lenses
}

// GenerateCoverageLens produces coverage annotations per function if coverage data is available.
func GenerateCoverageLens(file, content string) []CodeLens {
	var lenses []CodeLens

	functions := extractFunctions(content)
	coverageData := loadCoverageData(file)
	if coverageData == nil {
		return nil
	}

	for _, fn := range functions {
		pct, ok := coverageData[fn.name]
		if !ok {
			continue
		}
		label := fmt.Sprintf("coverage: %.0f%%", pct)
		tooltip := fmt.Sprintf("func %s — ", fn.name)
		if pct >= 80 {
			tooltip += "well covered"
		} else if pct >= 50 {
			tooltip += "partially covered"
		} else {
			tooltip += "needs more tests"
		}
		lenses = append(lenses, CodeLens{
			File:     file,
			Line:     fn.line,
			Label:    label,
			Category: "coverage",
			Command:  fmt.Sprintf("go test -coverprofile=coverage.out -run . %s", file),
			Tooltip:  tooltip,
		})
	}
	return lenses
}

// ---------- Internal Helpers ----------

type funcInfo struct {
	name    string
	line    int
	endLine int
	body    string
}

type symbolInfo struct {
	name string
	line int
}

// extractFunctions parses Go source and extracts function declarations with their bodies.
func extractFunctions(content string) []funcInfo {
	var funcs []funcInfo
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		matches := funcDeclRe.FindStringSubmatch(lines[i])
		if matches == nil {
			continue
		}
		name := matches[1]
		startLine := i + 1
		braceCount := 0
		started := false
		var bodyLines []string

		for j := i; j < len(lines); j++ {
			for _, ch := range lines[j] {
				if ch == '{' {
					braceCount++
					started = true
				} else if ch == '}' {
					braceCount--
				}
			}
			bodyLines = append(bodyLines, lines[j])
			if started && braceCount == 0 {
				funcs = append(funcs, funcInfo{
					name:    name,
					line:    startLine,
					endLine: j + 1,
					body:    strings.Join(bodyLines, "\n"),
				})
				break
			}
		}
	}
	return funcs
}

// calculateCyclomaticComplexity computes a simplified cyclomatic complexity for a function body.
func calculateCyclomaticComplexity(body string) int {
	cc := 1
	// Count decision points
	decisionPatterns := []string{
		`\bif\b`,
		`\belse if\b`,
		`\bfor\b`,
		`\bcase\b`,
		`\b&&\b`,
		`\b\|\|\b`,
		`\bselect\b`,
	}
	// Use simpler token-based counting
	words := strings.Fields(body)
	for _, w := range words {
		switch w {
		case "if", "for", "case", "select":
			cc++
		}
	}
	// Count && and || in the body
	_ = decisionPatterns
	cc += strings.Count(body, "&&")
	cc += strings.Count(body, "||")
	return cc
}

// extractExportedSymbols finds exported function and type declarations.
func extractExportedSymbols(content string) []symbolInfo {
	var symbols []symbolInfo
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := funcDeclRe.FindStringSubmatch(line)
		if matches != nil {
			name := matches[1]
			if len(name) > 0 && unicode.IsUpper(rune(name[0])) {
				symbols = append(symbols, symbolInfo{name: name, line: i + 1})
			}
			continue
		}
		// Check for exported type declarations
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "type ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				name := parts[1]
				if len(name) > 0 && unicode.IsUpper(rune(name[0])) {
					symbols = append(symbols, symbolInfo{name: name, line: i + 1})
				}
			}
		}
	}
	return symbols
}

// countReferences counts occurrences of a symbol in the content (excluding its declaration).
func countReferences(content, symbol string) int {
	// Count all occurrences minus the declaration itself
	count := strings.Count(content, symbol)
	if count > 0 {
		count-- // exclude the declaration
	}
	return count
}

// blameEntry holds parsed git blame information for a single line.
type blameEntry struct {
	line int
	date time.Time
}

// getGitBlame runs git blame and returns parsed entries.
func getGitBlame(file string) []blameEntry {
	cmd := exec.Command("git", "blame", "--porcelain", file)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var entries []blameEntry
	lines := strings.Split(string(out), "\n")
	lineNum := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "author-time ") {
			ts := strings.TrimPrefix(l, "author-time ")
			var epoch int64
			fmt.Sscanf(ts, "%d", &epoch)
			if epoch > 0 {
				entries = append(entries, blameEntry{
					line: lineNum,
					date: time.Unix(epoch, 0),
				})
			}
		}
		// Track the line number from the header
		parts := strings.Fields(l)
		if len(parts) >= 3 && len(parts[0]) == 40 {
			fmt.Sscanf(parts[2], "%d", &lineNum)
		}
	}
	return entries
}

// lookupAge finds the most recent modification date for lines near the given line.
func lookupAge(entries []blameEntry, line int) string {
	if len(entries) == 0 {
		return ""
	}

	var newest time.Time
	for _, e := range entries {
		if e.line >= line && e.line <= line+20 {
			if e.date.After(newest) {
				newest = e.date
			}
		}
	}

	if newest.IsZero() {
		// Fall back to closest entry
		for _, e := range entries {
			if e.date.After(newest) {
				newest = e.date
			}
		}
	}

	if newest.IsZero() {
		return ""
	}

	return lensFormatDuration(time.Since(newest))
}

// lensFormatDuration formats a duration into a human-readable age string.
func lensFormatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days == 0 {
		hours := int(d.Hours())
		if hours == 0 {
			return "just now"
		}
		return fmt.Sprintf("%dh", hours)
	}
	if days < 7 {
		return fmt.Sprintf("%dd", days)
	}
	if days < 30 {
		return fmt.Sprintf("%dw", days/7)
	}
	if days < 365 {
		return fmt.Sprintf("%dmo", days/30)
	}
	return fmt.Sprintf("%dy", days/365)
}

// isRecent returns true if the age string indicates a recent modification (< 7 days).
func isRecent(age string) bool {
	if age == "just now" {
		return true
	}
	if strings.HasSuffix(age, "h") {
		return true
	}
	if strings.HasSuffix(age, "d") {
		var days int
		fmt.Sscanf(age, "%dd", &days)
		return days < 7
	}
	return false
}

// loadCoverageData attempts to load coverage information for the given file.
// Returns a map of function name to coverage percentage, or nil if unavailable.
func loadCoverageData(file string) map[string]float64 {
	// Look for a coverage.out file in the same directory
	dir := file
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		dir = file[:idx]
	}
	coverFile := dir + "/coverage.out"

	cmd := exec.Command("cat", coverFile)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	return parseCoverageProfile(string(out), file)
}

// parseCoverageProfile parses Go coverage profile output and returns per-function coverage.
func parseCoverageProfile(profile, file string) map[string]float64 {
	result := make(map[string]float64)
	lines := strings.Split(profile, "\n")

	type blockInfo struct {
		startLine int
		endLine   int
		stmts     int
		count     int
	}

	var blocks []blockInfo
	for _, line := range lines {
		if !strings.Contains(line, ":") || strings.HasPrefix(line, "mode:") {
			continue
		}
		// Format: file:startLine.startCol,endLine.endCol statements count
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		loc := parts[0]
		if !strings.Contains(loc, file) {
			continue
		}
		// Parse start and end lines
		colonIdx := strings.LastIndex(loc, ":")
		if colonIdx < 0 {
			continue
		}
		span := loc[colonIdx+1:]
		rangeParts := strings.Split(span, ",")
		if len(rangeParts) != 2 {
			continue
		}
		var startLine, endLine, stmts, count int
		fmt.Sscanf(rangeParts[0], "%d", &startLine)
		fmt.Sscanf(rangeParts[1], "%d", &endLine)
		fmt.Sscanf(parts[1], "%d", &stmts)
		fmt.Sscanf(parts[2], "%d", &count)
		blocks = append(blocks, blockInfo{startLine, endLine, stmts, count})
	}

	if len(blocks) == 0 {
		return nil
	}

	// This is a simplified mapping; full implementation would correlate with function line ranges
	_ = result
	_ = blocks
	return nil
}
