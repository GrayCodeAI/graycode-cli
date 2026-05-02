package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ShadowWorkspace provides a temporary directory where file edits can be
// validated (e.g. via `go vet`, `tsc`, `pylint`) without touching the
// original source tree.
type ShadowWorkspace struct {
	tempDir string
}

// NewShadowWorkspace creates a new temporary directory for shadow validation.
func NewShadowWorkspace() (*ShadowWorkspace, error) {
	dir, err := os.MkdirTemp("", "hawk-shadow-*")
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

	if err := os.WriteFile(tmpFile, []byte(newContent), 0o644); err != nil {
		return []ValidationError{{File: originalPath, Message: fmt.Sprintf("shadow write: %v", err)}}
	}
	defer os.Remove(tmpFile)

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
		os.RemoveAll(sw.tempDir)
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
		os.WriteFile(modPath, []byte("module shadowcheck\n\ngo 1.21\n"), 0o644)
		defer os.Remove(modPath)
	}

	cmd := exec.Command("go", "vet", "./...")
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
	cmd := exec.Command("python3", "-c",
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
	cmd := exec.Command("npx", "tsc", "--noEmit", "--allowJs", tmpPath)
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
