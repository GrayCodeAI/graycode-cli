package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AffectedTests holds the result of affected test detection.
type AffectedTests struct {
	ChangedFiles  []string
	AffectedTests []string
	TestFileMap   map[string][]string // source file -> test files that import it
}

// DetectAffectedTests analyzes changed files and determines which tests are
// affected by tracing the import graph. For Go projects, it uses a simple
// heuristic: for each changed .go file, find test files in the same package
// and packages that import the changed file's package.
func DetectAffectedTests(changedFiles []string) AffectedTests {
	result := AffectedTests{
		ChangedFiles: changedFiles,
		TestFileMap:  make(map[string][]string),
	}

	seen := map[string]bool{}
	var affected []string

	for _, f := range changedFiles {
		tests := findTestsForFile(f)
		result.TestFileMap[f] = tests
		for _, t := range tests {
			if !seen[t] {
				seen[t] = true
				affected = append(affected, t)
			}
		}
	}

	result.AffectedTests = affected
	return result
}

// findTestsForFile finds test files that are related to the given source file.
// Strategy:
// 1. Same directory: *_test.go files in the same package
// 2. Parent/child: test files in subdirectories that import this package
// 3. Integration: test files with matching import paths
func findTestsForFile(sourceFile string) []string {
	dir := filepath.Dir(sourceFile)
	base := filepath.Base(sourceFile)

	// Skip non-Go files
	if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") {
		return nil
	}

	var tests []string

	// 1. Same directory test files
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasSuffix(name, "_test.go") {
				tests = append(tests, filepath.Join(dir, name))
			}
		}
	}

	// 2. Check one level up for integration tests
	parentDir := filepath.Dir(dir)
	if parentDir != dir {
		parentEntries, err := os.ReadDir(parentDir)
		if err == nil {
			for _, e := range parentEntries {
				name := e.Name()
				if strings.HasSuffix(name, "_test.go") {
					// Check if the test file references our package
					testPath := filepath.Join(parentDir, name)
					if testReferencesPackage(testPath, filepath.Base(dir)) {
						tests = append(tests, testPath)
					}
				}
			}
		}
	}

	// 3. Check testdata or *_test directories
	testDirs := []string{
		filepath.Join(dir, "testdata"),
		filepath.Join(dir, "test"),
		filepath.Join(dir, "tests"),
	}
	for _, td := range testDirs {
		entries, err := os.ReadDir(td)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".go") {
				tests = append(tests, filepath.Join(td, name))
			}
		}
	}

	return tests
}

// testReferencesPackage checks if a test file imports or references a package name.
func testReferencesPackage(testPath, pkgName string) bool {
	data, err := os.ReadFile(testPath)
	if err != nil {
		return false
	}
	content := string(data)
	// Check for import of the package
	return strings.Contains(content, fmt.Sprintf(`"%s`, pkgName)) ||
		strings.Contains(content, fmt.Sprintf(`/%s"`, pkgName))
}

// FormatAffectedTests returns a human-readable summary of affected tests.
func FormatAffectedTests(at AffectedTests) string {
	if len(at.ChangedFiles) == 0 {
		return "No changed files detected."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Changed files: %d\n", len(at.ChangedFiles)))
	for _, f := range at.ChangedFiles {
		b.WriteString(fmt.Sprintf("  %s\n", f))
	}

	if len(at.AffectedTests) == 0 {
		b.WriteString("\nNo affected tests detected.\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("\nAffected tests: %d\n", len(at.AffectedTests)))
	for _, t := range at.AffectedTests {
		b.WriteString(fmt.Sprintf("  %s\n", t))
	}

	return b.String()
}
