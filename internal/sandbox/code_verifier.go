package sandbox

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"sync"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

// CodeVerifier performs static analysis of generated code before execution,
// blocking dangerous operations such as forbidden imports, destructive system
// calls, and unsafe patterns.
type CodeVerifier struct {
	BlockedModules   []string
	BlockedFunctions []string
	BlockedPatterns  []*regexp.Regexp
	AllowedPaths     []string
	mu               sync.RWMutex
}

// VerificationResult holds the outcome of a code verification pass.
type VerificationResult struct {
	Safe       bool
	Violations []Violation
	Warnings   []string
	Language   string
}

// Violation represents a single dangerous construct found in the code.
type Violation struct {
	Type     string // "blocked_module", "blocked_function", "dangerous_pattern", "file_access", "network_access", "system_call"
	Line     int
	Code     string
	Reason   string
	Severity string // "error", "warning"
}

// NewCodeVerifier returns a CodeVerifier pre-configured with sensible defaults
// for Python, Go, JavaScript/TypeScript, Ruby, and Bash analysis.
func NewCodeVerifier() *CodeVerifier {
	cv := &CodeVerifier{
		BlockedModules: []string{
			// Python dangerous modules/calls
			"os.system",
			"subprocess.call(shell=True)",
			"eval",
			"exec",
			"__import__",
			// Go dangerous packages
			"unsafe",
			"syscall",
			// JavaScript/TypeScript dangerous modules/calls
			"child_process",
			"vm.runInNewContext",
			"fs.unlinkSync",
			"fs.rmSync",
			// Ruby dangerous calls
			"Kernel#system",
			"Kernel#exec",
			"FileUtils.rm_rf",
		},
		BlockedFunctions: []string{
			"rm",
			"rmdir",
			"format",
			"kill",
		},
		AllowedPaths: []string{"/tmp", "/var/tmp"},
	}

	patterns := []string{
		// Python
		`os\.system\(`,
		`exec\(`,
		`eval\(`,
		`__import__\(`,
		`subprocess\.call.*shell=True`,
		// JavaScript/TypeScript
		`require\s*\(\s*["']child_process["']\s*\)`,
		`child_process`,
		`vm\.runInNewContext`,
		`new\s+Function\s*\(`,
		`fs\.unlinkSync`,
		`fs\.rmSync`,
		// Ruby
		`Kernel#(?:system|exec)`,
		`FileUtils\.rm_rf`,
		`eval\s*[(\x60]`,
	}
	for _, p := range patterns {
		cv.BlockedPatterns = append(cv.BlockedPatterns, regexp.MustCompile(p))
	}

	return cv
}

// ApplyConfig merges user-configured blocked modules and patterns on top of
// the defaults. Empty config is a no-op.
func (cv *CodeVerifier) ApplyConfig(cfg *CodeVerifierConfig) {
	if cfg == nil {
		return
	}
	cv.BlockedModules = append(cv.BlockedModules, cfg.BlockedModules...)
	for _, p := range cfg.BlockedPatterns {
		if p == "" {
			continue
		}
		if re, err := regexp.Compile(p); err == nil {
			cv.BlockedPatterns = append(cv.BlockedPatterns, re)
		}
	}
}

// Verify analyses code in the given language and returns a structured result.
// Supported languages: "go", "python", "bash".
func (cv *CodeVerifier) Verify(code, language string) *VerificationResult {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	result := &VerificationResult{
		Safe:     true,
		Language: language,
	}

	var violations []Violation

	switch strings.ToLower(language) {
	case "go", "golang":
		violations = cv.verifyGo(code)
	case "python", "py":
		violations = cv.verifyPython(code)
	case "bash", "sh", "shell":
		violations = cv.verifyBash(code)
	default:
		// For unknown languages, run generic pattern matching.
		violations = cv.verifyGeneric(code)
	}

	for i := range violations {
		if violations[i].Severity == "" {
			violations[i].Severity = "error"
		}
		if violations[i].Severity == "error" {
			result.Safe = false
		} else if violations[i].Severity == "warning" {
			result.Warnings = append(result.Warnings, violations[i].Reason)
		}
	}

	result.Violations = violations
	return result
}

// VerifyGo analyses Go source code using go/ast for imports and calls.
func (cv *CodeVerifier) VerifyGo(code string) []Violation {
	cv.mu.RLock()
	defer cv.mu.RUnlock()
	return cv.verifyGo(code)
}

func (cv *CodeVerifier) verifyGo(code string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "generated.go", code, parser.AllErrors)
	if err != nil {
		// If parsing fails, fall back to regex-based checks.
		return cv.verifyGeneric(code)
	}

	// Check imports.
	for _, imp := range f.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		line := fset.Position(imp.Pos()).Line

		if importPath == "unsafe" {
			violations = append(violations, Violation{
				Type:     "blocked_module",
				Line:     line,
				Code:     importPath,
				Reason:   `import of "unsafe" package is blocked`,
				Severity: "error",
			})
		}
		if importPath == "syscall" {
			violations = append(violations, Violation{
				Type:     "blocked_module",
				Line:     line,
				Code:     importPath,
				Reason:   `import of "syscall" package is blocked`,
				Severity: "error",
			})
		}
	}

	// Walk AST looking for dangerous calls.
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		line := fset.Position(call.Pos()).Line

		// Check for os.Exit in library code.
		if ident, ok := sel.X.(*ast.Ident); ok {
			pkgName := ident.Name
			fnName := sel.Sel.Name

			if pkgName == "os" && fnName == "Exit" {
				violations = append(violations, Violation{
					Type:     "system_call",
					Line:     line,
					Code:     "os.Exit",
					Reason:   "os.Exit in library code may cause unexpected termination",
					Severity: "warning",
				})
			}

			// Check for exec.Command with potential user input.
			if pkgName == "exec" && fnName == "Command" {
				violations = append(violations, Violation{
					Type:     "system_call",
					Line:     line,
					Code:     "exec.Command",
					Reason:   "exec.Command may execute arbitrary commands",
					Severity: "error",
				})
			}
		}

		return true
	})

	return violations
}

// VerifyPython analyses Python source code using regex-based pattern matching.
func (cv *CodeVerifier) VerifyPython(code string) []Violation {
	cv.mu.RLock()
	defer cv.mu.RUnlock()
	return cv.verifyPython(code)
}

func (cv *CodeVerifier) verifyPython(code string) []Violation {
	var violations []Violation

	lines := strings.Split(code, "\n")

	// Python-specific patterns.
	dangerousPatterns := []struct {
		pattern *regexp.Regexp
		typ     string
		reason  string
	}{
		{regexp.MustCompile(`\beval\s*\(`), "blocked_function", "eval() can execute arbitrary code"},
		{regexp.MustCompile(`\bexec\s*\(`), "blocked_function", "exec() can execute arbitrary code"},
		{regexp.MustCompile(`\bos\.system\s*\(`), "blocked_module", `os.system() executes shell commands`},
		{regexp.MustCompile(`\bsubprocess\.call\s*\(.*shell\s*=\s*True`), "blocked_module", `subprocess.call with shell=True is dangerous`},
		{regexp.MustCompile(`\bsubprocess\.Popen\s*\(.*shell\s*=\s*True`), "blocked_module", `subprocess.Popen with shell=True is dangerous`},
		{regexp.MustCompile(`\b__import__\s*\(`), "blocked_module", "__import__() can import arbitrary modules"},
		{regexp.MustCompile(`\bpickle\.loads?\s*\(`), "blocked_function", "pickle.load(s) can execute arbitrary code during deserialization"},
		{regexp.MustCompile(`\bos\.remove\s*\(`), "file_access", "os.remove() deletes files"},
		{regexp.MustCompile(`\bos\.rmdir\s*\(`), "file_access", "os.rmdir() deletes directories"},
		{regexp.MustCompile(`\bshutil\.rmtree\s*\(`), "file_access", "shutil.rmtree() recursively deletes directories"},
		{regexp.MustCompile(`\bsocket\.socket\s*\(`), "network_access", "direct socket creation detected"},
	}

	for lineNum, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip comments.
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		for _, dp := range dangerousPatterns {
			if dp.pattern.MatchString(line) {
				violations = append(violations, Violation{
					Type:     dp.typ,
					Line:     lineNum + 1,
					Code:     strings.TrimSpace(line),
					Reason:   dp.reason,
					Severity: "error",
				})
			}
		}
	}

	// Also run blocklist patterns.
	violations = append(violations, cv.matchBlockedPatterns(code)...)

	return violations
}

// VerifyBash analyses shell script code for dangerous commands.
func (cv *CodeVerifier) VerifyBash(code string) []Violation {
	cv.mu.RLock()
	defer cv.mu.RUnlock()
	return cv.verifyBash(code)
}

func (cv *CodeVerifier) verifyBash(code string) []Violation {
	var violations []Violation

	lines := strings.Split(code, "\n")

	dangerousPatterns := []struct {
		pattern *regexp.Regexp
		typ     string
		reason  string
	}{
		{regexp.MustCompile(`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|(-[a-zA-Z]*f[a-zA-Z]*r))\s+/\s*$`), "system_call", "rm -rf / will destroy the entire filesystem"},
		{regexp.MustCompile(`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|(-[a-zA-Z]*f[a-zA-Z]*r))\s+/[^/\s]`), "system_call", "recursive forced deletion of system path"},
		{regexp.MustCompile(`\bsudo\b`), "system_call", "sudo escalates privileges"},
		{regexp.MustCompile(`\bchmod\s+777\b`), "file_access", "chmod 777 makes files world-readable/writable/executable"},
		{regexp.MustCompile(`\bcurl\b.*\|\s*(ba)?sh`), "system_call", "curl piped to shell executes remote code"},
		{regexp.MustCompile(`\bwget\b.*\|\s*(ba)?sh`), "system_call", "wget piped to shell executes remote code"},
		{regexp.MustCompile(`\bdd\b.*\bof=/dev/`), "system_call", "dd to device can destroy data"},
		{regexp.MustCompile(`\bmkfs\b`), "system_call", "mkfs formats a disk partition"},
		{regexp.MustCompile(`\b:\(\)\s*\{\s*:\|\s*:&\s*\}`), "system_call", "fork bomb detected"},
		{regexp.MustCompile(`>\s*/dev/sd[a-z]`), "system_call", "writing directly to block device"},
		{regexp.MustCompile(`\brm\s+-rf\s+/\s*$`), "system_call", "rm -rf / will destroy the entire filesystem"},
	}

	for lineNum, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip comments.
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		for _, dp := range dangerousPatterns {
			if dp.pattern.MatchString(line) {
				violations = append(violations, Violation{
					Type:     dp.typ,
					Line:     lineNum + 1,
					Code:     strings.TrimSpace(line),
					Reason:   dp.reason,
					Severity: "error",
				})
			}
		}
	}

	return violations
}

// AddBlockedModule adds a module to the blocked list.
func (cv *CodeVerifier) AddBlockedModule(module string) {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	cv.BlockedModules = append(cv.BlockedModules, module)
}

// AddBlockedFunction adds a function to the blocked list.
func (cv *CodeVerifier) AddBlockedFunction(fn string) {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	cv.BlockedFunctions = append(cv.BlockedFunctions, fn)
}

// AddBlockedPattern compiles and adds a regex pattern to the blocked list.
func (cv *CodeVerifier) AddBlockedPattern(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}
	cv.mu.Lock()
	defer cv.mu.Unlock()
	cv.BlockedPatterns = append(cv.BlockedPatterns, re)
	return nil
}

// FormatResult produces a human-readable summary of verification results.
func FormatResult(result *VerificationResult) string {
	var sb strings.Builder

	if result.Safe {
		sb.WriteString("Code Verification: SAFE\n")
		sb.WriteString(strings.Repeat("─", 25))
		sb.WriteString("\n")
		sb.WriteString("No violations detected.\n")
		return sb.String()
	}

	sb.WriteString("Code Verification: UNSAFE\n")
	sb.WriteString(strings.Repeat("─", 25))
	sb.WriteString("\n")

	errorCount := 0
	warningCount := 0

	for _, v := range result.Violations {
		if v.Severity == "warning" {
			warningCount++
			sb.WriteString(fmt.Sprintf("%s L%d: %s\n", icons.Alert(), v.Line, v.Reason))
		} else {
			errorCount++
			sb.WriteString(fmt.Sprintf("%s L%d: %s\n", icons.CloseThick(), v.Line, v.Reason))
		}
	}

	sb.WriteString("\n")
	parts := []string{}
	if errorCount > 0 {
		parts = append(parts, fmt.Sprintf("%d violation%s", errorCount, pluralS(errorCount)))
	}
	if warningCount > 0 {
		parts = append(parts, fmt.Sprintf("%d warning%s", warningCount, pluralS(warningCount)))
	}
	sb.WriteString(strings.Join(parts, ", "))

	if errorCount > 0 {
		sb.WriteString(" — execution blocked")
	}
	sb.WriteString("\n")

	return sb.String()
}

// verifyGeneric runs the blocklist patterns against any code.
func (cv *CodeVerifier) verifyGeneric(code string) []Violation {
	return cv.matchBlockedPatterns(code)
}

// matchBlockedPatterns checks all configured blocked patterns against the code.
func (cv *CodeVerifier) matchBlockedPatterns(code string) []Violation {
	var violations []Violation
	lines := strings.Split(code, "\n")

	for _, pattern := range cv.BlockedPatterns {
		for lineNum, line := range lines {
			if pattern.MatchString(line) {
				violations = append(violations, Violation{
					Type:     "dangerous_pattern",
					Line:     lineNum + 1,
					Code:     strings.TrimSpace(line),
					Reason:   fmt.Sprintf("dangerous pattern: %s", pattern.String()),
					Severity: "error",
				})
			}
		}
	}

	return violations
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
