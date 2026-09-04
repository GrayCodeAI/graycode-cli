package branching

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ValidationError represents a single validation issue.
type ValidationError struct {
	File    string
	Line    int
	Column  int
	Message string
}

// ShadowWorkspace provides a temporary directory where file edits can be
// validated (e.g. via `go vet`, `tsc`, `pylint`) without touching the
// original source tree.
type ShadowWorkspace struct {
	tempDir string
}

// NewShadowWorkspace creates a new temporary directory for shadow validation.
func NewShadowWorkspace() (*ShadowWorkspace, error) {
	dir, err := os.MkdirTemp("", "graycode-shadow-*")
	if err != nil {
		return nil, fmt.Errorf("shadow workspace: create temp dir: %w", err)
	}
	return &ShadowWorkspace{tempDir: dir}, nil
}

// TempDir returns the path to the shadow workspace temp directory.
func (sw *ShadowWorkspace) TempDir() string {
	return sw.tempDir
}

// ValidateEdit copies a file into the shadow workspace, writes newContent to
// the copy, runs the language-appropriate validation tool, and returns any
// errors found. The temp copy is cleaned up before returning.
func (sw *ShadowWorkspace) ValidateEdit(originalPath, newContent string) []ValidationError {
	ext := strings.ToLower(filepath.Ext(originalPath))
	base := filepath.Base(originalPath)
	tmpFile := filepath.Join(sw.tempDir, base)

	if err := os.WriteFile(tmpFile, []byte(newContent), 0o600); err != nil {
		return []ValidationError{{File: originalPath, Message: fmt.Sprintf("shadow write: %v", err)}}
	}
	defer func() { _ = os.Remove(tmpFile) }()

	runner := shadowValidator(ext)
	if runner == nil {
		return nil // no validator for this language — assume valid
	}

	return runner(tmpFile, originalPath)
}

// ValidateMultipleEdits validates several files at once and returns a map of
// file path to validation errors.
func (sw *ShadowWorkspace) ValidateMultipleEdits(edits map[string]string) map[string][]ValidationError {
	results := make(map[string][]ValidationError, len(edits))
	for path, content := range edits {
		errs := sw.ValidateEdit(path, content)
		if len(errs) > 0 {
			results[path] = errs
		}
	}
	return results
}

// Close removes the shadow workspace temp directory and all its contents.
func (sw *ShadowWorkspace) Close() {
	if sw.tempDir != "" {
		_ = os.RemoveAll(sw.tempDir)
	}
}

// shadowValidator returns a validation function for the given file extension.
func shadowValidator(ext string) func(tmpPath, origPath string) []ValidationError {
	switch ext {
	case ".go":
		return shadowValidateGo
	case ".py":
		return shadowValidatePython
	case ".ts", ".tsx":
		return shadowValidateTS
	default:
		return nil
	}
}

// shadowValidateGo runs `go vet` on the temp file directory.
func shadowValidateGo(tmpPath, origPath string) []ValidationError {
	dir := filepath.Dir(tmpPath)

	// Ensure a go.mod exists so `go vet` can operate.
	modPath := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		_ = os.WriteFile(modPath, []byte("module shadowcheck\n\ngo 1.21\n"), 0o600)
		defer func() { _ = os.Remove(modPath) }()
	}

	cmd := exec.CommandContext(context.Background(), "go", "vet", "./...") // #nosec G204 -- fixed Go executable
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	parsed := parseGoErrors(string(output))
	if len(parsed) == 0 && len(output) > 0 {
		return []ValidationError{{File: origPath, Message: strings.TrimSpace(string(output))}}
	}
	// Rewrite file references to point to the original path.
	for i := range parsed {
		parsed[i].File = origPath
	}
	return parsed
}

// shadowValidatePython runs `python3 -c "import py_compile; ..."` on the temp file.
func shadowValidatePython(tmpPath, origPath string) []ValidationError {
	cmd := exec.CommandContext(context.Background(), "python3", "-c", // #nosec G204 -- debugger/interpreter invocation with file path or expression from tool params
		fmt.Sprintf("import py_compile; py_compile.compile('%s', doraise=True)", tmpPath))
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	parsed := parsePythonErrors(string(output), origPath)
	if len(parsed) == 0 && len(output) > 0 {
		return []ValidationError{{File: origPath, Message: strings.TrimSpace(string(output))}}
	}
	return parsed
}

// shadowValidateTS runs `npx tsc --noEmit` on the temp file.
func shadowValidateTS(tmpPath, origPath string) []ValidationError {
	cmd := exec.CommandContext(context.Background(), "npx", "tsc", "--noEmit", "--allowJs", tmpPath) // #nosec G204 -- fixed TypeScript compiler executable
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	// If tsc is not available, assume valid.
	if strings.Contains(string(output), "not found") || strings.Contains(string(output), "ERR!") {
		return nil
	}

	parsed := parseTSErrors(string(output), origPath)
	if len(parsed) == 0 && len(output) > 0 {
		return []ValidationError{{File: origPath, Message: strings.TrimSpace(string(output))}}
	}
	return parsed
}

// --- parser helpers (moved inline from validate.go for independence) ---

var goErrorRe = regexp.MustCompile(`([^:]+\.go):(\d+):(\d+):\s*(.+)`)

func parseGoErrors(output string) []ValidationError {
	var errors []ValidationError
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if matches := goErrorRe.FindStringSubmatch(line); matches != nil {
			lineNum, _ := strconv.Atoi(matches[2])
			colNum, _ := strconv.Atoi(matches[3])
			errors = append(errors, ValidationError{
				File:    matches[1],
				Line:    lineNum,
				Column:  colNum,
				Message: matches[4],
			})
		}
	}
	return errors
}

var (
	pythonLineRe  = regexp.MustCompile(`File "([^"]+)", line (\d+)`)
	pythonErrorRe = regexp.MustCompile(`(SyntaxError|IndentationError|TabError):\s*(.+)`)
)

func parsePythonErrors(output, path string) []ValidationError {
	var errors []ValidationError
	lines := strings.Split(output, "\n")
	lineNum := 0

	for _, line := range lines {
		if matches := pythonLineRe.FindStringSubmatch(line); matches != nil {
			lineNum, _ = strconv.Atoi(matches[2])
		}
		if matches := pythonErrorRe.FindStringSubmatch(line); matches != nil {
			errors = append(errors, ValidationError{
				File:    path,
				Line:    lineNum,
				Message: matches[1] + ": " + matches[2],
			})
		}
	}
	return errors
}

var tsErrorRe = regexp.MustCompile(`\((\d+),(\d+)\):\s*error\s+\w+:\s*(.+)`)

func parseTSErrors(output, path string) []ValidationError {
	var errors []ValidationError
	for _, line := range strings.Split(output, "\n") {
		if matches := tsErrorRe.FindStringSubmatch(line); matches != nil {
			lineNum, _ := strconv.Atoi(matches[1])
			colNum, _ := strconv.Atoi(matches[2])
			errors = append(errors, ValidationError{
				File:    path,
				Line:    lineNum,
				Column:  colNum,
				Message: matches[3],
			})
		}
	}
	return errors
}
