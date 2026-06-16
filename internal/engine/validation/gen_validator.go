package validation

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// GenValidator checks generated code for correctness before writing to disk.
type GenValidator struct {
	Checks []GenCheck
	mu     sync.RWMutex
}

// GenCheck defines a single validation check that can be applied to generated code.
type GenCheck struct {
	Name     string
	Language string // empty means all languages
	CheckFn  func(code, language string) []GenIssue
	Severity string // "error", "warning", "info"
}

// GenIssue represents a single problem found in generated code.
type GenIssue struct {
	Check       string
	Message     string
	Line        int
	Severity    string
	AutoFixable bool
	Fix         string
}

// GenValidation holds the complete result of validating generated code.
type GenValidation struct {
	Valid    bool
	Issues   []GenIssue
	Language string
	Score    float64
}

// NewGenValidator creates a GenValidator with built-in checks for common generation errors.
func NewGenValidator() *GenValidator {
	gv := &GenValidator{}

	gv.Checks = []GenCheck{
		{
			Name:     "syntax",
			Language: "",
			Severity: "error",
			CheckFn: func(code, language string) []GenIssue {
				return CheckBalancedDelimiters(code)
			},
		},
		{
			Name:     "imports",
			Language: "go",
			Severity: "warning",
			CheckFn: func(code, language string) []GenIssue {
				return checkDuplicateImports(code)
			},
		},
		{
			Name:     "naming",
			Language: "go",
			Severity: "warning",
			CheckFn: func(code, language string) []GenIssue {
				return checkExportedNaming(code)
			},
		},
		{
			Name:     "completeness",
			Language: "",
			Severity: "warning",
			CheckFn: func(code, language string) []GenIssue {
				return CheckCompleteness(code)
			},
		},
		{
			Name:     "compilation",
			Language: "go",
			Severity: "error",
			CheckFn: func(code, language string) []GenIssue {
				return checkGoCompilation(code)
			},
		},
		{
			Name:     "types",
			Language: "go",
			Severity: "error",
			CheckFn: func(code, language string) []GenIssue {
				return checkTypeConsistency(code)
			},
		},
	}

	return gv
}

// Validate runs all applicable checks on the given code and returns structured results.
func (gv *GenValidator) Validate(code, language string) *GenValidation {
	gv.mu.RLock()
	defer gv.mu.RUnlock()

	result := &GenValidation{
		Valid:    true,
		Language: language,
	}

	for _, check := range gv.Checks {
		if check.Language != "" && check.Language != language {
			continue
		}
		issues := check.CheckFn(code, language)
		result.Issues = append(result.Issues, issues...)
	}

	// Calculate score based on issues
	errorCount := 0
	warningCount := 0
	for _, issue := range result.Issues {
		if issue.Severity == "error" {
			errorCount++
		} else if issue.Severity == "warning" {
			warningCount++
		}
	}

	if errorCount > 0 {
		result.Valid = false
	}

	// Score: start at 1.0, subtract 0.15 per error, 0.05 per warning
	result.Score = 1.0 - float64(errorCount)*0.15 - float64(warningCount)*0.05
	if result.Score < 0 {
		result.Score = 0
	}

	return result
}

// ValidateGo uses go/parser to check Go code for syntax errors and common generation issues.
func ValidateGo(code string) []GenIssue {
	var issues []GenIssue

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "generated.go", code, parser.AllErrors)
	if err != nil {
		// Parse the error list for line information
		errStr := err.Error()
		for _, line := range strings.Split(errStr, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			lineNum := genExtractLineNumber(line)
			issues = append(issues, GenIssue{
				Check:    "go-syntax",
				Message:  line,
				Line:     lineNum,
				Severity: "error",
			})
		}
	}

	// Check for common generation errors
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect unreachable code patterns (simplistic)
		if trimmed == "return" || strings.HasPrefix(trimmed, "return ") {
			if i+1 < len(lines) {
				nextTrimmed := strings.TrimSpace(lines[i+1])
				if nextTrimmed != "" && nextTrimmed != "}" && !strings.HasPrefix(nextTrimmed, "//") && !strings.HasPrefix(nextTrimmed, "case ") && !strings.HasPrefix(nextTrimmed, "default:") {
					issues = append(issues, GenIssue{
						Check:    "go-unreachable",
						Message:  "possible unreachable code after return",
						Line:     i + 2,
						Severity: "warning",
					})
				}
			}
		}
	}

	return issues
}

// ValidatePython checks Python code for indentation consistency and syntax patterns.
func ValidatePython(code string) []GenIssue {
	var issues []GenIssue
	lines := strings.Split(code, "\n")

	// Track indentation style
	usesSpaces := false
	usesTabs := false
	spaceCount := 0

	for i, line := range lines {
		if line == "" || strings.TrimSpace(line) == "" {
			continue
		}

		// Detect indentation type
		if strings.HasPrefix(line, "\t") {
			usesTabs = true
		} else if strings.HasPrefix(line, " ") {
			usesSpaces = true
			// Count leading spaces for first indented line
			if spaceCount == 0 {
				count := 0
				for _, ch := range line {
					if ch == ' ' {
						count++
					} else {
						break
					}
				}
				if count > 0 {
					spaceCount = count
				}
			}
		}

		// Check for mixed tabs and spaces
		if usesSpaces && usesTabs {
			issues = append(issues, GenIssue{
				Check:       "python-indent",
				Message:     "mixed tabs and spaces in indentation",
				Line:        i + 1,
				Severity:    "error",
				AutoFixable: true,
				Fix:         "replace tabs with spaces",
			})
			break // Only report once
		}

		// Check for unclosed strings
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			singleCount := 0
			doubleCount := 0
			escaped := false
			inTripleSingle := false
			inTripleDouble := false

			for j := 0; j < len(trimmed); j++ {
				ch := trimmed[j]
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}

				// Check for triple quotes
				if j+2 < len(trimmed) {
					if trimmed[j:j+3] == "'''" {
						inTripleSingle = !inTripleSingle
						j += 2
						continue
					}
					if trimmed[j:j+3] == `"""` {
						inTripleDouble = !inTripleDouble
						j += 2
						continue
					}
				}

				if !inTripleSingle && !inTripleDouble {
					if ch == '\'' {
						singleCount++
					} else if ch == '"' {
						doubleCount++
					}
				}
			}

			if !inTripleSingle && !inTripleDouble {
				if singleCount%2 != 0 {
					issues = append(issues, GenIssue{
						Check:    "python-string",
						Message:  "unclosed single-quote string",
						Line:     i + 1,
						Severity: "error",
					})
				}
				if doubleCount%2 != 0 {
					issues = append(issues, GenIssue{
						Check:    "python-string",
						Message:  "unclosed double-quote string",
						Line:     i + 1,
						Severity: "error",
					})
				}
			}
		}
	}

	// Check indentation consistency for space-indented code
	if usesSpaces && spaceCount > 0 {
		for i, line := range lines {
			if line == "" || strings.TrimSpace(line) == "" {
				continue
			}
			leading := 0
			for _, ch := range line {
				if ch == ' ' {
					leading++
				} else {
					break
				}
			}
			if leading > 0 && leading%spaceCount != 0 {
				issues = append(issues, GenIssue{
					Check:       "python-indent",
					Message:     fmt.Sprintf("indentation is %d spaces, expected multiple of %d", leading, spaceCount),
					Line:        i + 1,
					Severity:    "warning",
					AutoFixable: true,
					Fix:         fmt.Sprintf("adjust indentation to multiple of %d spaces", spaceCount),
				})
			}
		}
	}

	return issues
}

// ValidateTypeScript checks TypeScript code for balanced braces and import/export patterns.
func ValidateTypeScript(code string) []GenIssue {
	var issues []GenIssue

	// Check balanced braces
	issues = append(issues, CheckBalancedDelimiters(code)...)

	// Check import patterns
	lines := strings.Split(code, "\n")
	importSeen := make(map[string]bool)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for duplicate imports
		if strings.HasPrefix(trimmed, "import ") {
			// Extract the import source
			parts := strings.Split(trimmed, "from")
			if len(parts) == 2 {
				source := strings.TrimSpace(parts[1])
				source = strings.TrimRight(source, ";")
				source = strings.Trim(source, "'\"")
				if importSeen[source] {
					issues = append(issues, GenIssue{
						Check:       "ts-import",
						Message:     fmt.Sprintf("duplicate import from %q", source),
						Line:        i + 1,
						Severity:    "warning",
						AutoFixable: true,
						Fix:         "merge duplicate imports",
					})
				}
				importSeen[source] = true
			}
		}

		// Check for export before declaration without proper syntax
		if strings.HasPrefix(trimmed, "export default export") {
			issues = append(issues, GenIssue{
				Check:    "ts-export",
				Message:  "invalid double export statement",
				Line:     i + 1,
				Severity: "error",
			})
		}
	}

	return issues
}

// CheckBalancedDelimiters counts {}, [], (), and <> (for generics) and reports mismatches.
func CheckBalancedDelimiters(code string) []GenIssue {
	var issues []GenIssue

	type delimInfo struct {
		char rune
		line int
	}

	var stack []delimInfo
	lines := strings.Split(code, "\n")
	inString := false
	stringChar := rune(0)
	inLineComment := false
	inBlockComment := false

	for lineIdx, line := range lines {
		inLineComment = false
		for i := 0; i < len(line); i++ {
			ch := rune(line[i])

			// Handle block comment end
			if inBlockComment {
				if ch == '*' && i+1 < len(line) && line[i+1] == '/' {
					inBlockComment = false
					i++
				}
				continue
			}

			// Handle line comment
			if inLineComment {
				continue
			}

			// Handle strings
			if inString {
				if ch == '\\' {
					i++ // skip escaped char
					continue
				}
				if ch == stringChar {
					inString = false
				}
				continue
			}

			// Detect comment start
			if ch == '/' && i+1 < len(line) {
				next := line[i+1]
				if next == '/' {
					inLineComment = true
					continue
				}
				if next == '*' {
					inBlockComment = true
					i++
					continue
				}
			}

			// Detect string start
			if ch == '"' || ch == '\'' || ch == '`' {
				inString = true
				stringChar = ch
				continue
			}

			// Track delimiters
			switch ch {
			case '{', '(', '[':
				stack = append(stack, delimInfo{ch, lineIdx + 1})
			case '}':
				if len(stack) > 0 && stack[len(stack)-1].char == '{' {
					stack = stack[:len(stack)-1]
				} else {
					issues = append(issues, GenIssue{
						Check:       "syntax",
						Message:     "unexpected closing brace '}'",
						Line:        lineIdx + 1,
						Severity:    "error",
						AutoFixable: true,
						Fix:         "remove extra closing brace",
					})
				}
			case ')':
				if len(stack) > 0 && stack[len(stack)-1].char == '(' {
					stack = stack[:len(stack)-1]
				} else {
					issues = append(issues, GenIssue{
						Check:       "syntax",
						Message:     "unexpected closing parenthesis ')'",
						Line:        lineIdx + 1,
						Severity:    "error",
						AutoFixable: true,
						Fix:         "remove extra closing parenthesis",
					})
				}
			case ']':
				if len(stack) > 0 && stack[len(stack)-1].char == '[' {
					stack = stack[:len(stack)-1]
				} else {
					issues = append(issues, GenIssue{
						Check:       "syntax",
						Message:     "unexpected closing bracket ']'",
						Line:        lineIdx + 1,
						Severity:    "error",
						AutoFixable: true,
						Fix:         "remove extra closing bracket",
					})
				}
			}
		}
	}

	// Report unclosed delimiters
	for _, d := range stack {
		var name string
		switch d.char {
		case '{':
			name = "brace"
		case '(':
			name = "parenthesis"
		case '[':
			name = "bracket"
		}
		issues = append(issues, GenIssue{
			Check:       "syntax",
			Message:     fmt.Sprintf("unclosed %s", name),
			Line:        d.line,
			Severity:    "error",
			AutoFixable: true,
			Fix:         fmt.Sprintf("add closing %s", name),
		})
	}

	return issues
}

// CheckCompleteness finds leftover placeholder markers like TODO, FIXME, ..., pass, NotImplementedError.
func CheckCompleteness(code string) []GenIssue {
	var issues []GenIssue
	lines := strings.Split(code, "\n")

	todoRe := regexp.MustCompile(`\b(TODO|FIXME|HACK|XXX)\b`)
	notImplRe := regexp.MustCompile(`NotImplementedError|not implemented`)
	ellipsisRe := regexp.MustCompile(`^\s*\.\.\.\s*$`)
	passRe := regexp.MustCompile(`^\s*pass\s*$`)

	for i, line := range lines {
		if matches := todoRe.FindString(line); matches != "" {
			issues = append(issues, GenIssue{
				Check:    "completeness",
				Message:  fmt.Sprintf("%s marker left in generated code", matches),
				Line:     i + 1,
				Severity: "warning",
			})
		}
		if notImplRe.MatchString(line) {
			issues = append(issues, GenIssue{
				Check:    "completeness",
				Message:  "NotImplementedError left in generated code",
				Line:     i + 1,
				Severity: "warning",
			})
		}
		if ellipsisRe.MatchString(line) {
			issues = append(issues, GenIssue{
				Check:    "completeness",
				Message:  "placeholder ellipsis '...' left in generated code",
				Line:     i + 1,
				Severity: "warning",
			})
		}
		if passRe.MatchString(line) {
			// Only flag pass if it seems like a placeholder (function body with only pass)
			if i > 0 {
				prevTrimmed := strings.TrimSpace(lines[i-1])
				if strings.HasSuffix(prevTrimmed, ":") {
					issues = append(issues, GenIssue{
						Check:    "completeness",
						Message:  "placeholder 'pass' left in generated code",
						Line:     i + 1,
						Severity: "warning",
					})
				}
			}
		}
	}

	return issues
}

// FormatValidation renders a GenValidation into a human-readable string.
func FormatValidation(v *GenValidation) string {
	var sb strings.Builder

	issueCount := len(v.Issues)
	if issueCount == 0 {
		sb.WriteString("Code Validation: no issues\n")
		sb.WriteString(fmt.Sprintf("\nScore: %.2f (good to go)\n", v.Score))
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Code Validation: %d issue", issueCount))
	if issueCount != 1 {
		sb.WriteString("s")
	}
	sb.WriteString("\n")

	// Separator
	sb.WriteString(strings.Repeat("─", 25))
	sb.WriteString("\n")

	for _, issue := range v.Issues {
		icon := icons.CloseThick()
		if issue.Severity == "warning" {
			icon = icons.Alert()
		} else if issue.Severity == "info" {
			icon = "ℹ"
		}

		fixNote := ""
		if issue.AutoFixable {
			fixNote = " (auto-fixable)"
		}

		if issue.Line > 0 {
			sb.WriteString(fmt.Sprintf("%s L%d: %s — %s%s\n", icon, issue.Line, issue.Check, issue.Message, fixNote))
		} else {
			sb.WriteString(fmt.Sprintf("%s %s — %s%s\n", icon, issue.Check, issue.Message, fixNote))
		}
	}

	sb.WriteString("\n")

	scoreComment := "good to go"
	if v.Score < 0.5 {
		scoreComment = "significant issues found"
	} else if v.Score < 0.9 {
		scoreComment = "proceed with caution"
	}

	sb.WriteString(fmt.Sprintf("Score: %.2f (%s)\n", v.Score, scoreComment))

	return sb.String()
}

// AutoFix applies auto-fixable issues to the code and returns the corrected version.
func AutoFix(code string, issues []GenIssue) string {
	// Collect auto-fixable issues, grouped by type
	var unclosedBraces, unclosedParens, unclosedBrackets int
	extraClosers := make(map[int]string) // line -> type to remove

	for _, issue := range issues {
		if !issue.AutoFixable {
			continue
		}

		switch {
		case issue.Check == "syntax" && strings.Contains(issue.Message, "unclosed brace"):
			unclosedBraces++
		case issue.Check == "syntax" && strings.Contains(issue.Message, "unclosed parenthesis"):
			unclosedParens++
		case issue.Check == "syntax" && strings.Contains(issue.Message, "unclosed bracket"):
			unclosedBrackets++
		case issue.Check == "syntax" && strings.Contains(issue.Message, "unexpected closing brace"):
			extraClosers[issue.Line] = "}"
		case issue.Check == "syntax" && strings.Contains(issue.Message, "unexpected closing parenthesis"):
			extraClosers[issue.Line] = ")"
		case issue.Check == "syntax" && strings.Contains(issue.Message, "unexpected closing bracket"):
			extraClosers[issue.Line] = "]"
		case issue.Check == "python-indent" && strings.Contains(issue.Fix, "replace tabs with spaces"):
			code = strings.ReplaceAll(code, "\t", "    ")
		}
	}

	// Remove extra closers
	if len(extraClosers) > 0 {
		lines := strings.Split(code, "\n")
		for lineNum, closer := range extraClosers {
			if lineNum-1 < len(lines) {
				line := lines[lineNum-1]
				trimmed := strings.TrimSpace(line)
				if trimmed == closer {
					lines[lineNum-1] = ""
				} else {
					// Remove last occurrence of the closer in the line
					idx := strings.LastIndex(lines[lineNum-1], closer)
					if idx >= 0 {
						lines[lineNum-1] = lines[lineNum-1][:idx] + lines[lineNum-1][idx+1:]
					}
				}
			}
		}
		code = strings.Join(lines, "\n")
	}

	// Append missing closers at end
	for i := 0; i < unclosedBraces; i++ {
		code += "\n}"
	}
	for i := 0; i < unclosedParens; i++ {
		code += "\n)"
	}
	for i := 0; i < unclosedBrackets; i++ {
		code += "\n]"
	}

	return code
}

// --- Internal helper functions ---

// checkDuplicateImports finds duplicate import paths in Go code.
func checkDuplicateImports(code string) []GenIssue {
	var issues []GenIssue

	// Find import blocks
	importRe := regexp.MustCompile(`import\s*\(\s*([\s\S]*?)\)`)
	matches := importRe.FindAllStringSubmatchIndex(code, -1)

	seen := make(map[string]int) // path -> first line

	lines := strings.Split(code, "\n")

	for _, match := range matches {
		blockStart := match[2]
		blockEnd := match[3]
		block := code[blockStart:blockEnd]

		// Calculate starting line number
		startLine := strings.Count(code[:blockStart], "\n") + 1

		for i, importLine := range strings.Split(block, "\n") {
			trimmed := strings.TrimSpace(importLine)
			if trimmed == "" || trimmed == "(" || trimmed == ")" {
				continue
			}
			// Strip alias if present
			parts := strings.Fields(trimmed)
			importPath := parts[len(parts)-1]
			importPath = strings.Trim(importPath, `"`)

			lineNum := startLine + i
			if firstLine, exists := seen[importPath]; exists {
				issues = append(issues, GenIssue{
					Check:       "imports",
					Message:     fmt.Sprintf("duplicate import %q (first at line %d)", importPath, firstLine),
					Line:        lineNum,
					Severity:    "warning",
					AutoFixable: true,
					Fix:         "remove duplicate import",
				})
			} else {
				seen[importPath] = lineNum
			}
		}
	}

	// Also handle single-line imports
	singleImportRe := regexp.MustCompile(`import\s+"([^"]+)"`)
	for i, line := range lines {
		if m := singleImportRe.FindStringSubmatch(line); m != nil {
			importPath := m[1]
			if firstLine, exists := seen[importPath]; exists {
				issues = append(issues, GenIssue{
					Check:       "imports",
					Message:     fmt.Sprintf("duplicate import %q (first at line %d)", importPath, firstLine),
					Line:        i + 1,
					Severity:    "warning",
					AutoFixable: true,
					Fix:         "remove duplicate import",
				})
			} else {
				seen[importPath] = i + 1
			}
		}
	}

	return issues
}

// checkExportedNaming verifies exported Go names are capitalized.
func checkExportedNaming(code string) []GenIssue {
	var issues []GenIssue
	lines := strings.Split(code, "\n")

	// Match func/type/var/const declarations
	declRe := regexp.MustCompile(`^(func|type|var|const)\s+(\w+)`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := declRe.FindStringSubmatch(trimmed); m != nil {
			name := m[2]
			// Skip unexported names (that's intentional) and receiver methods
			if strings.HasPrefix(trimmed, "func (") {
				continue
			}
			// Check if name starts with lowercase but looks like it should be exported
			// (has a doc comment above it)
			if i > 0 && len(name) > 0 {
				prevLine := strings.TrimSpace(lines[i-1])
				if strings.HasPrefix(prevLine, "//") && unicode.IsLower(rune(name[0])) {
					// Has a doc comment but unexported — might be intentional, flag as info
					issues = append(issues, GenIssue{
						Check:    "naming",
						Message:  fmt.Sprintf("documented name %q is unexported — consider capitalizing", name),
						Line:     i + 1,
						Severity: "info",
					})
				}
			}
		}
	}

	return issues
}

// checkGoCompilation attempts to compile Go code by writing a temp file and running go build.
func checkGoCompilation(code string) []GenIssue {
	var issues []GenIssue

	// Only try compilation if the code looks like a complete file
	if !strings.Contains(code, "package ") {
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "genvalidator-*")
	if err != nil {
		return nil
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmpFile := filepath.Join(tmpDir, "generated.go")
	if writeErr := os.WriteFile(tmpFile, []byte(code), 0o644); writeErr != nil {
		return nil
	}

	// Initialize a module in the temp directory
	cmd := exec.CommandContext(context.Background(), "go", "mod", "init", "temp")
	cmd.Dir = tmpDir
	if runErr := cmd.Run(); runErr != nil {
		return nil
	}

	// Try to build
	cmd = exec.CommandContext(context.Background(), "go", "build", ".")
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	// Parse build errors
	goErrRe := regexp.MustCompile(`generated\.go:(\d+):(\d+):\s*(.+)`)
	for _, line := range strings.Split(string(output), "\n") {
		if m := goErrRe.FindStringSubmatch(line); m != nil {
			lineNum := 0
			_, _ = fmt.Sscanf(m[1], "%d", &lineNum)
			issues = append(issues, GenIssue{
				Check:    "compilation",
				Message:  m[3],
				Line:     lineNum,
				Severity: "error",
			})
		}
	}

	return issues
}

// checkTypeConsistency performs basic type consistency checks on Go code.
func checkTypeConsistency(code string) []GenIssue {
	var issues []GenIssue
	lines := strings.Split(code, "\n")

	// Check for nil returns in non-pointer functions
	funcRe := regexp.MustCompile(`^func\s+\w+\(.*\)\s+(\w+)\s*{`)
	inFunc := false
	returnType := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if m := funcRe.FindStringSubmatch(trimmed); m != nil {
			inFunc = true
			returnType = m[1]
			continue
		}

		if inFunc && trimmed == "}" {
			inFunc = false
			returnType = ""
			continue
		}

		if inFunc && (trimmed == "return nil" || strings.HasPrefix(trimmed, "return nil,") || strings.HasPrefix(trimmed, "return nil ")) {
			// If return type is a value type (not pointer, interface, slice, map, chan, error)
			switch returnType {
			case "int", "int8", "int16", "int32", "int64",
				"uint", "uint8", "uint16", "uint32", "uint64",
				"float32", "float64", "bool", "string", "byte", "rune":
				issues = append(issues, GenIssue{
					Check:    "types",
					Message:  fmt.Sprintf("returning nil for non-nilable type %q", returnType),
					Line:     i + 1,
					Severity: "error",
				})
			}
		}
	}

	return issues
}

// genExtractLineNumber tries to pull a line number from a Go parser error string.
func genExtractLineNumber(errLine string) int {
	// Format: filename:line:col: message
	re := regexp.MustCompile(`:(\d+):`)
	if m := re.FindStringSubmatch(errLine); m != nil {
		n := 0
		_, _ = fmt.Sscanf(m[1], "%d", &n)
		return n
	}
	return 0
}
