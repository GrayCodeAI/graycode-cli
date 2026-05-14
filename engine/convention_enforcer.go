package engine

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// Convention represents a single coding convention that can be enforced on generated code.
type Convention struct {
	Name        string
	Description string
	Pattern     *regexp.Regexp
	AntiPattern *regexp.Regexp
	Language    string
	Category    string  // "naming", "structure", "error_handling", "testing", "style"
	Example     string
	Confidence  float64 // 0.0 to 1.0 — how confident we are this convention applies
}

// ConventionSet holds a collection of conventions learned from a project.
type ConventionSet struct {
	Conventions []Convention
	ProjectDir  string
	mu          sync.RWMutex
}

// Violation records a single convention violation in generated code.
type Violation struct {
	Convention string
	File       string
	Line       int
	Code       string
	Expected   string
	Got        string
}

// NewConventionSet creates a new ConventionSet for the given project directory.
func NewConventionSet(projectDir string) *ConventionSet {
	return &ConventionSet{
		Conventions: make([]Convention, 0),
		ProjectDir:  projectDir,
	}
}

// LearnConventions scans existing code in projectDir to infer conventions:
// naming style, error handling, testing patterns, structure, and imports.
func (cs *ConventionSet) LearnConventions(projectDir string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return fmt.Errorf("project directory does not exist: %s", projectDir)
	}

	cs.ProjectDir = projectDir

	// Gather Go source files for analysis.
	var goFiles []string
	_ = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == "node_modules" || base == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			goFiles = append(goFiles, path)
		}
		return nil
	})

	if len(goFiles) == 0 {
		return nil
	}

	cs.learnNamingConventions(goFiles)
	cs.learnErrorHandling(goFiles)
	cs.learnTestStyle(goFiles)
	cs.learnStructure(goFiles)
	cs.learnImportStyle(goFiles)

	return nil
}

// learnNamingConventions detects naming patterns: camelCase vs snake_case, prefix patterns, etc.
func (cs *ConventionSet) learnNamingConventions(files []string) {
	var exportedFuncs int
	var unexportedFuncs int
	var snakeCaseVars int
	var camelCaseVars int

	funcDeclRe := regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?([A-Za-z_][A-Za-z0-9_]*)`)
	varDeclRe := regexp.MustCompile(`^\s*(?:var\s+)?([a-z][a-zA-Z0-9_]*)\s*(?::=|=)`)
	snakeRe := regexp.MustCompile(`[a-z]+_[a-z]+`)

	for _, f := range files {
		lines := readConventionFileLines(f)
		for _, line := range lines {
			if m := funcDeclRe.FindStringSubmatch(line); len(m) > 1 {
				name := m[1]
				if len(name) > 0 && unicode.IsUpper(rune(name[0])) {
					exportedFuncs++
				} else {
					unexportedFuncs++
				}
			}
			if m := varDeclRe.FindStringSubmatch(line); len(m) > 1 {
				name := m[1]
				if snakeRe.MatchString(name) {
					snakeCaseVars++
				} else {
					camelCaseVars++
				}
			}
		}
	}

	// If project strongly prefers camelCase for variables, add convention.
	if camelCaseVars > snakeCaseVars*3 {
		cs.Conventions = append(cs.Conventions, Convention{
			Name:        "camelCase variables",
			Description: "Local variables use camelCase, not snake_case",
			Pattern:     regexp.MustCompile(`[a-z][a-zA-Z0-9]*\s*:=`),
			AntiPattern: regexp.MustCompile(`[a-z]+_[a-z]+\s*:=`),
			Language:    "go",
			Category:    "naming",
			Example:     "userName := getValue() // not user_name",
			Confidence:  conventionConfidence(camelCaseVars, snakeCaseVars),
		})
	}

	// Exported functions use PascalCase.
	if exportedFuncs > 0 {
		cs.Conventions = append(cs.Conventions, Convention{
			Name:        "PascalCase exported functions",
			Description: "Exported functions and methods use PascalCase",
			Pattern:     regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?[A-Z][a-zA-Z0-9]*`),
			AntiPattern: regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?[a-z][a-zA-Z0-9]*.*\{`),
			Language:    "go",
			Category:    "naming",
			Example:     "func GetUser() // not getUser()",
			Confidence:  0.95,
		})
	}
}

// learnErrorHandling detects error handling patterns: wrapping style, sentinel errors.
func (cs *ConventionSet) learnErrorHandling(files []string) {
	var wrapCount int
	var bareCount int
	var sentinelCount int

	wrapRe := regexp.MustCompile(`fmt\.Errorf\(.+%w`)
	bareReturnRe := regexp.MustCompile(`return\s+err\s*$`)
	sentinelRe := regexp.MustCompile(`^var\s+Err[A-Z]\w+\s*=\s*(?:errors\.New|fmt\.Errorf)`)

	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		lines := readConventionFileLines(f)
		for _, line := range lines {
			if wrapRe.MatchString(line) {
				wrapCount++
			}
			if bareReturnRe.MatchString(line) {
				bareCount++
			}
			if sentinelRe.MatchString(line) {
				sentinelCount++
			}
		}
	}

	// If project uses error wrapping predominantly.
	if wrapCount > bareCount*2 {
		cs.Conventions = append(cs.Conventions, Convention{
			Name:        "error wrapping with %w",
			Description: "Errors are wrapped with fmt.Errorf and %w, not returned bare",
			Pattern:     regexp.MustCompile(`fmt\.Errorf\(.+%w`),
			AntiPattern: regexp.MustCompile(`return\s+err\s*$`),
			Language:    "go",
			Category:    "error_handling",
			Example:     `return fmt.Errorf("getting user: %w", err)`,
			Confidence:  conventionConfidence(wrapCount, bareCount),
		})
	}

	// If project uses sentinel errors.
	if sentinelCount >= 3 {
		cs.Conventions = append(cs.Conventions, Convention{
			Name:        "sentinel errors",
			Description: "Package-level sentinel errors use var ErrXxx = errors.New(...)",
			Pattern:     regexp.MustCompile(`^var\s+Err[A-Z]\w+\s*=`),
			AntiPattern: nil,
			Language:    "go",
			Category:    "error_handling",
			Example:     `var ErrNotFound = errors.New("not found")`,
			Confidence:  0.8,
		})
	}
}

// learnTestStyle detects testing patterns: table-driven, testify, or plain.
func (cs *ConventionSet) learnTestStyle(files []string) {
	var tableDriven int
	var subtests int
	var testifyAssert int
	var plainTests int

	tableRe := regexp.MustCompile(`(?:tests|cases|tt|tc)\s*:=\s*\[\](?:struct|test)`)
	subtestRe := regexp.MustCompile(`t\.Run\(`)
	testifyRe := regexp.MustCompile(`(?:assert|require)\.\w+\(`)
	funcTestRe := regexp.MustCompile(`^func\s+Test\w+`)

	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			continue
		}
		lines := readConventionFileLines(f)
		content := strings.Join(lines, "\n")
		tableDriven += len(tableRe.FindAllString(content, -1))
		subtests += len(subtestRe.FindAllString(content, -1))
		testifyAssert += len(testifyRe.FindAllString(content, -1))
		plainTests += len(funcTestRe.FindAllString(content, -1))
	}

	if tableDriven > 0 && tableDriven >= plainTests/3 {
		cs.Conventions = append(cs.Conventions, Convention{
			Name:        "table-driven tests",
			Description: "Tests use table-driven pattern with subtests",
			Pattern:     regexp.MustCompile(`(?:tests|cases)\s*:=\s*\[\]`),
			AntiPattern: nil,
			Language:    "go",
			Category:    "testing",
			Example:     "tests := []struct{ name string; input string; want string }{...}",
			Confidence:  conventionConfidence(tableDriven, plainTests-tableDriven),
		})
	}

	if subtests > plainTests/2 {
		cs.Conventions = append(cs.Conventions, Convention{
			Name:        "subtests with t.Run",
			Description: "Tests use t.Run for subtests",
			Pattern:     regexp.MustCompile(`t\.Run\(`),
			AntiPattern: nil,
			Language:    "go",
			Category:    "testing",
			Example:     `t.Run(tc.name, func(t *testing.T) { ... })`,
			Confidence:  conventionConfidence(subtests, plainTests),
		})
	}

	if testifyAssert == 0 && plainTests > 0 {
		cs.Conventions = append(cs.Conventions, Convention{
			Name:        "stdlib testing only",
			Description: "Tests use only stdlib testing package, no testify",
			Pattern:     regexp.MustCompile(`"testing"`),
			AntiPattern: regexp.MustCompile(`"github\.com/stretchr/testify`),
			Language:    "go",
			Category:    "testing",
			Example:     `if got != want { t.Errorf("got %v, want %v", got, want) }`,
			Confidence:  0.9,
		})
	}
}

// learnStructure detects file and package organization patterns.
func (cs *ConventionSet) learnStructure(files []string) {
	var hasTestSuffix int
	var hasDocFile int

	for _, f := range files {
		base := filepath.Base(f)
		if strings.HasSuffix(base, "_test.go") {
			hasTestSuffix++
		}
		if base == "doc.go" {
			hasDocFile++
		}
	}

	if hasTestSuffix > 0 {
		cs.Conventions = append(cs.Conventions, Convention{
			Name:        "test file co-location",
			Description: "Test files are co-located with source files using _test.go suffix",
			Pattern:     regexp.MustCompile(`_test\.go$`),
			AntiPattern: nil,
			Language:    "go",
			Category:    "structure",
			Example:     "foo.go and foo_test.go in the same directory",
			Confidence:  0.95,
		})
	}

	// Detect if files use snake_case naming.
	var snakeFiles int
	var totalFiles int
	for _, f := range files {
		base := filepath.Base(f)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}
		totalFiles++
		name := strings.TrimSuffix(base, ".go")
		if strings.Contains(name, "_") {
			snakeFiles++
		}
	}

	if snakeFiles > totalFiles/2 && totalFiles > 3 {
		cs.Conventions = append(cs.Conventions, Convention{
			Name:        "snake_case file names",
			Description: "Source files use snake_case naming (e.g., my_module.go)",
			Pattern:     regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*\.go$`),
			AntiPattern: regexp.MustCompile(`[A-Z]`),
			Language:    "go",
			Category:    "structure",
			Example:     "convention_enforcer.go, not conventionEnforcer.go",
			Confidence:  conventionConfidence(snakeFiles, totalFiles-snakeFiles),
		})
	}

	_ = hasDocFile
}

// learnImportStyle detects import grouping and alias patterns.
func (cs *ConventionSet) learnImportStyle(files []string) {
	var groupedImports int
	var flatImports int

	for _, f := range files {
		lines := readConventionFileLines(f)
		inImport := false
		hasBlankInImport := false
		importLines := 0
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "import (" {
				inImport = true
				importLines = 0
				hasBlankInImport = false
				continue
			}
			if inImport {
				if trimmed == ")" {
					inImport = false
					if hasBlankInImport && importLines > 1 {
						groupedImports++
					} else if importLines > 1 {
						flatImports++
					}
					continue
				}
				if trimmed == "" {
					hasBlankInImport = true
				} else {
					importLines++
				}
			}
		}
	}

	if groupedImports > flatImports*2 {
		cs.Conventions = append(cs.Conventions, Convention{
			Name:        "grouped imports",
			Description: "Imports are grouped: stdlib, then external, separated by blank line",
			Pattern:     regexp.MustCompile(`import \(`),
			AntiPattern: nil,
			Language:    "go",
			Category:    "style",
			Example:     "import (\n\t\"fmt\"\n\n\t\"github.com/pkg/errors\"\n)",
			Confidence:  conventionConfidence(groupedImports, flatImports),
		})
	}
}

// Check validates generated code against all learned conventions and returns violations.
func (cs *ConventionSet) Check(code, file string) []Violation {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	var violations []Violation
	violations = append(violations, cs.CheckNaming(code)...)
	violations = append(violations, cs.CheckErrorHandling(code)...)
	violations = append(violations, cs.CheckTestStyle(code)...)

	// Check additional style and structure conventions.
	lines := strings.Split(code, "\n")
	for _, conv := range cs.Conventions {
		if conv.Category == "naming" || conv.Category == "error_handling" || conv.Category == "testing" {
			continue // Already handled by specific checkers.
		}
		if conv.AntiPattern == nil {
			continue
		}
		for i, line := range lines {
			if conv.AntiPattern.MatchString(line) {
				violations = append(violations, Violation{
					Convention: conv.Name,
					File:       file,
					Line:       i + 1,
					Code:       strings.TrimSpace(line),
					Expected:   conv.Example,
					Got:        strings.TrimSpace(line),
				})
			}
		}
	}

	// Set file on all violations.
	for i := range violations {
		if violations[i].File == "" {
			violations[i].File = file
		}
	}

	return violations
}

// CheckNaming verifies names follow the project's naming conventions.
func (cs *ConventionSet) CheckNaming(code string) []Violation {
	var violations []Violation
	lines := strings.Split(code, "\n")

	for _, conv := range cs.Conventions {
		if conv.Category != "naming" {
			continue
		}
		if conv.AntiPattern == nil {
			continue
		}
		for i, line := range lines {
			if conv.AntiPattern.MatchString(line) {
				// Extract the offending name.
				got := strings.TrimSpace(line)
				violations = append(violations, Violation{
					Convention: conv.Name,
					Line:       i + 1,
					Code:       got,
					Expected:   conv.Example,
					Got:        got,
				})
			}
		}
	}

	return violations
}

// CheckErrorHandling verifies error handling matches the project's style.
func (cs *ConventionSet) CheckErrorHandling(code string) []Violation {
	var violations []Violation
	lines := strings.Split(code, "\n")

	for _, conv := range cs.Conventions {
		if conv.Category != "error_handling" {
			continue
		}
		if conv.AntiPattern == nil {
			continue
		}
		for i, line := range lines {
			if conv.AntiPattern.MatchString(line) {
				violations = append(violations, Violation{
					Convention: conv.Name,
					Line:       i + 1,
					Code:       strings.TrimSpace(line),
					Expected:   conv.Example,
					Got:        strings.TrimSpace(line),
				})
			}
		}
	}

	return violations
}

// CheckTestStyle verifies tests match the project's testing patterns.
func (cs *ConventionSet) CheckTestStyle(code string) []Violation {
	var violations []Violation

	// Only check if code looks like a test file.
	if !strings.Contains(code, "func Test") {
		return violations
	}

	for _, conv := range cs.Conventions {
		if conv.Category != "testing" {
			continue
		}

		// Check for antipattern presence.
		if conv.AntiPattern != nil && conv.AntiPattern.MatchString(code) {
			lines := strings.Split(code, "\n")
			for i, line := range lines {
				if conv.AntiPattern.MatchString(line) {
					violations = append(violations, Violation{
						Convention: conv.Name,
						Line:       i + 1,
						Code:       strings.TrimSpace(line),
						Expected:   conv.Example,
						Got:        strings.TrimSpace(line),
					})
				}
			}
		}

		// Check that pattern is present if it's expected.
		if conv.Name == "table-driven tests" && conv.Pattern != nil {
			if !conv.Pattern.MatchString(code) {
				// Count how many test functions exist.
				testFuncRe := regexp.MustCompile(`func\s+Test\w+`)
				matches := testFuncRe.FindAllStringIndex(code, -1)
				if len(matches) > 0 {
					violations = append(violations, Violation{
						Convention: conv.Name,
						Line:       1,
						Code:       "test file without table-driven pattern",
						Expected:   conv.Example,
						Got:        "individual test assertions",
					})
				}
			}
		}
	}

	return violations
}

// Enforce auto-fixes violations where possible and returns fixed code plus remaining violations.
func (cs *ConventionSet) Enforce(code string) (string, []Violation) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	fixed := code
	var remaining []Violation

	// Fix bare error returns by wrapping with %w.
	for _, conv := range cs.Conventions {
		if conv.Category == "error_handling" && conv.Name == "error wrapping with %w" {
			fixed = fixBareErrorReturns(fixed)
		}
	}

	// Fix snake_case variables to camelCase.
	for _, conv := range cs.Conventions {
		if conv.Category == "naming" && conv.Name == "camelCase variables" {
			fixed = fixSnakeCaseVars(fixed)
		}
	}

	// Re-check for remaining violations.
	remaining = cs.checkWithoutLock(fixed, "")

	return fixed, remaining
}

// checkWithoutLock is an internal helper that checks conventions without acquiring the lock.
func (cs *ConventionSet) checkWithoutLock(code, file string) []Violation {
	var violations []Violation
	lines := strings.Split(code, "\n")

	for _, conv := range cs.Conventions {
		if conv.AntiPattern == nil {
			continue
		}
		if conv.Category == "testing" {
			continue // Test style checks need special logic.
		}
		for i, line := range lines {
			if conv.AntiPattern.MatchString(line) {
				violations = append(violations, Violation{
					Convention: conv.Name,
					File:       file,
					Line:       i + 1,
					Code:       strings.TrimSpace(line),
					Expected:   conv.Example,
					Got:        strings.TrimSpace(line),
				})
			}
		}
	}

	// Check test conventions.
	if strings.Contains(code, "func Test") {
		for _, conv := range cs.Conventions {
			if conv.Category != "testing" {
				continue
			}
			if conv.AntiPattern != nil && conv.AntiPattern.MatchString(code) {
				violations = append(violations, Violation{
					Convention: conv.Name,
					File:       file,
					Line:       1,
					Code:       "test style violation",
					Expected:   conv.Example,
					Got:        "non-conforming test pattern",
				})
			}
		}
	}

	return violations
}

// AddConvention adds a new convention to the set.
func (cs *ConventionSet) AddConvention(conv Convention) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.Conventions = append(cs.Conventions, conv)
}

// FormatViolations formats a list of violations for display.
func FormatViolations(violations []Violation) string {
	if len(violations) == 0 {
		return "No convention violations found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Convention Violations (%d):\n", len(violations)))
	sb.WriteString(strings.Repeat("─", 25))
	sb.WriteString("\n")

	for _, v := range violations {
		location := ""
		if v.File != "" {
			location = fmt.Sprintf(" [%s:%d]", v.File, v.Line)
		} else if v.Line > 0 {
			location = fmt.Sprintf(" [line %d]", v.Line)
		}

		desc := v.Convention
		detail := ""
		if v.Expected != "" && v.Got != "" && v.Expected != v.Got {
			detail = fmt.Sprintf(": %q should follow %q", v.Got, v.Expected)
		} else if v.Code != "" {
			detail = fmt.Sprintf(": %s", v.Code)
		}

		sb.WriteString(fmt.Sprintf("⚠ %s%s%s\n", desc, detail, location))
	}

	return sb.String()
}

// FormatConventions lists all learned conventions in a human-readable format.
func (cs *ConventionSet) FormatConventions() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if len(cs.Conventions) == 0 {
		return "No conventions learned yet."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project Conventions (%d):\n", len(cs.Conventions)))
	sb.WriteString(strings.Repeat("─", 30))
	sb.WriteString("\n\n")

	categories := map[string][]Convention{
		"naming":         {},
		"error_handling": {},
		"testing":        {},
		"structure":      {},
		"style":          {},
	}

	for _, conv := range cs.Conventions {
		categories[conv.Category] = append(categories[conv.Category], conv)
	}

	categoryOrder := []string{"naming", "error_handling", "testing", "structure", "style"}
	categoryLabels := map[string]string{
		"naming":         "Naming",
		"error_handling": "Error Handling",
		"testing":        "Testing",
		"structure":      "Structure",
		"style":          "Style",
	}

	for _, cat := range categoryOrder {
		convs := categories[cat]
		if len(convs) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("[%s]\n", categoryLabels[cat]))
		for _, c := range convs {
			sb.WriteString(fmt.Sprintf("  - %s (confidence: %.0f%%)\n", c.Description, c.Confidence*100))
			if c.Example != "" {
				sb.WriteString(fmt.Sprintf("    Example: %s\n", c.Example))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// fixBareErrorReturns rewrites bare "return err" to a wrapped version.
func fixBareErrorReturns(code string) string {
	lines := strings.Split(code, "\n")
	bareRe := regexp.MustCompile(`^(\s*)return\s+err\s*$`)

	// Try to find the enclosing function name for context.
	funcNameRe := regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?(\w+)`)

	currentFunc := "operation"
	for i, line := range lines {
		if m := funcNameRe.FindStringSubmatch(line); len(m) > 1 {
			currentFunc = camelToWords(m[1])
		}
		if m := bareRe.FindStringSubmatch(line); len(m) > 0 {
			indent := m[1]
			lines[i] = fmt.Sprintf(`%sreturn fmt.Errorf("%s: %%w", err)`, indent, currentFunc)
		}
	}

	return strings.Join(lines, "\n")
}

// fixSnakeCaseVars converts snake_case variable declarations to camelCase.
func fixSnakeCaseVars(code string) string {
	lines := strings.Split(code, "\n")
	snakeVarRe := regexp.MustCompile(`^(\s*)([a-z]+(?:_[a-z]+)+)(\s*:=)`)

	for i, line := range lines {
		if m := snakeVarRe.FindStringSubmatch(line); len(m) > 0 {
			camel := snakeToCamel(m[2])
			lines[i] = m[1] + camel + m[3] + line[len(m[0]):]
		}
	}

	return strings.Join(lines, "\n")
}

// snakeToCamel converts a snake_case string to camelCase.
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) <= 1 {
		return s
	}
	result := parts[0]
	for _, p := range parts[1:] {
		if len(p) > 0 {
			result += strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return result
}

// camelToWords converts a camelCase/PascalCase name to a lowercase space-separated phrase.
func camelToWords(s string) string {
	if s == "" {
		return s
	}
	var words []string
	current := strings.Builder{}
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			current.WriteRune(unicode.ToLower(r))
		} else {
			current.WriteRune(unicode.ToLower(r))
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return strings.Join(words, " ")
}

// conventionConfidence returns a confidence score based on the ratio of matching vs non-matching.
func conventionConfidence(matching, nonMatching int) float64 {
	total := matching + nonMatching
	if total == 0 {
		return 0.0
	}
	conf := float64(matching) / float64(total)
	if conf > 0.95 {
		return 0.95
	}
	return conf
}

// readConventionFileLines reads a file and returns its lines for convention analysis.
func readConventionFileLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}
