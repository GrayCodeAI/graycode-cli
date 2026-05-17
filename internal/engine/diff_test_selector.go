package engine

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// TestSelector provides diff-aware test selection, identifying which tests
// need to run based on changed files and dependency relationships.
type TestSelector struct {
	ProjectDir string
	Language   string
	DepGraph   map[string][]string
	mu         sync.RWMutex
}

// SelectedTests holds the result of a diff-aware test selection.
type SelectedTests struct {
	Tests    []string
	Packages []string
	Reason   map[string]string
	Coverage float64
}

// NewTestSelector creates a TestSelector for the given project directory.
// It auto-detects the project language and builds a dependency graph.
func NewTestSelector(projectDir string) *TestSelector {
	ts := &TestSelector{
		ProjectDir: projectDir,
		Language:   dtsDetectLanguage(projectDir),
		DepGraph:   make(map[string][]string),
	}
	ts.DepGraph = BuildDependencyGraph(projectDir)
	return ts
}

// SelectFromDiff parses a unified diff string, extracts changed file paths,
// and selects the related tests.
func (ts *TestSelector) SelectFromDiff(diff string) (*SelectedTests, error) {
	if diff == "" {
		return &SelectedTests{
			Tests:    []string{},
			Packages: []string{},
			Reason:   map[string]string{},
		}, nil
	}

	changedFiles := dtsParseDiffFiles(diff)
	if len(changedFiles) == 0 {
		return &SelectedTests{
			Tests:    []string{},
			Packages: []string{},
			Reason:   map[string]string{},
		}, nil
	}

	return ts.SelectFromFiles(changedFiles)
}

// SelectFromFiles determines which tests to run given a list of changed files.
// Strategy depends on the detected language.
func (ts *TestSelector) SelectFromFiles(changedFiles []string) (*SelectedTests, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	selected := &SelectedTests{
		Tests:    []string{},
		Packages: []string{},
		Reason:   make(map[string]string),
	}

	pkgSet := make(map[string]bool)

	for _, file := range changedFiles {
		// Skip test files themselves — we already know about them.
		if isTestFile(file, ts.Language) {
			selected.Tests = append(selected.Tests, file)
			pkg := filepath.Dir(file)
			if !pkgSet[pkg] {
				pkgSet[pkg] = true
				selected.Packages = append(selected.Packages, pkg)
			}
			selected.Reason[file] = "changed test file"
			continue
		}

		related := ts.FindRelatedTests(file)
		for _, t := range related {
			selected.Tests = append(selected.Tests, t)
			selected.Reason[t] = fmt.Sprintf("direct: tests %s", filepath.Base(file))
		}

		pkg := filepath.Dir(file)
		if !pkgSet[pkg] {
			pkgSet[pkg] = true
			selected.Packages = append(selected.Packages, pkg)
		}

		// Find dependent packages via the dependency graph.
		dependents := ts.DepGraph[pkg]
		for _, dep := range dependents {
			if !pkgSet[dep] {
				pkgSet[dep] = true
				selected.Packages = append(selected.Packages, dep)
				selected.Reason[dep] = fmt.Sprintf("dependent: imports %s", pkg)
			}
			// Find tests in dependent packages.
			depTests := findTestsInDir(filepath.Join(ts.ProjectDir, dep), ts.Language)
			for _, dt := range depTests {
				relPath := filepath.Join(dep, filepath.Base(dt))
				selected.Tests = append(selected.Tests, relPath)
				if _, exists := selected.Reason[relPath]; !exists {
					selected.Reason[relPath] = fmt.Sprintf("dependent: imports %s", pkg)
				}
			}
		}
	}

	// Deduplicate tests.
	selected.Tests = dtsDedup(selected.Tests)
	selected.Packages = dtsDedup(selected.Packages)

	// Estimate coverage ratio.
	totalTests := countAllTests(ts.ProjectDir, ts.Language)
	if totalTests > 0 {
		selected.Coverage = float64(len(selected.Tests)) / float64(totalTests)
	}

	return selected, nil
}

// BuildDependencyGraph scans the project directory and builds a reverse
// import graph mapping packages to their dependents.
func BuildDependencyGraph(projectDir string) map[string][]string {
	graph := make(map[string][]string)

	// Determine the module path for Go projects.
	modulePath := detectModulePath(projectDir)

	// Walk all Go files and parse imports.
	_ = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		relDir, _ := filepath.Rel(projectDir, filepath.Dir(path))
		if relDir == "." {
			relDir = ""
		}

		imports := parseGoImports(path)
		for _, imp := range imports {
			// Only track internal imports.
			if modulePath != "" && strings.HasPrefix(imp, modulePath) {
				depPkg := strings.TrimPrefix(imp, modulePath+"/")
				if depPkg == imp {
					depPkg = "."
				}
				// relDir depends on depPkg.
				graph[depPkg] = appendUnique(graph[depPkg], relDir)
			}
		}
		return nil
	})

	return graph
}

// FindRelatedTests finds test files related to the given source file.
func (ts *TestSelector) FindRelatedTests(file string) []string {
	var tests []string

	switch ts.Language {
	case "go":
		tests = ts.findGoRelatedTests(file)
	case "python":
		tests = ts.findPythonRelatedTests(file)
	case "javascript", "typescript":
		tests = ts.findJSRelatedTests(file)
	default:
		tests = ts.findGoRelatedTests(file)
	}

	return tests
}

// GenerateTestCommand produces a runnable test command for the selected tests.
func GenerateTestCommand(selected *SelectedTests, language string) string {
	if len(selected.Tests) == 0 && len(selected.Packages) == 0 {
		return ""
	}

	switch language {
	case "go":
		return generateGoTestCommand(selected)
	case "python":
		return generatePythonTestCommand(selected)
	case "javascript", "typescript":
		return generateJSTestCommand(selected)
	default:
		return generateGoTestCommand(selected)
	}
}

// FormatSelection produces a human-readable summary of the test selection.
func FormatSelection(selected *SelectedTests, changedFiles []string, language string, totalTests int) string {
	var b strings.Builder

	b.WriteString("Diff-Aware Test Selection:\n")

	// Changed files summary.
	if len(changedFiles) > 0 {
		fileList := strings.Join(changedFiles, ", ")
		b.WriteString(fmt.Sprintf("Changed: %d files (%s)\n", len(changedFiles), fileList))
	}

	b.WriteString("\nSelected tests:\n")

	// Group tests by package.
	pkgTests := make(map[string][]string)
	pkgReasons := make(map[string]string)
	for _, t := range selected.Tests {
		pkg := filepath.Dir(t)
		pkgTests[pkg] = append(pkgTests[pkg], filepath.Base(t))
		if reason, ok := selected.Reason[t]; ok {
			// Extract reason type.
			if strings.HasPrefix(reason, "direct") {
				pkgReasons[pkg] = "direct"
			} else if strings.HasPrefix(reason, "dependent") {
				if pkgReasons[pkg] != "direct" {
					pkgReasons[pkg] = "dependent"
				}
			}
		}
	}

	// Sort packages for stable output.
	var pkgs []string
	for p := range pkgTests {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)

	for _, pkg := range pkgs {
		files := pkgTests[pkg]
		reason := pkgReasons[pkg]
		var reasonDetail string
		if reason == "dependent" {
			if r, ok := selected.Reason[pkg]; ok {
				reasonDetail = r
			} else {
				reasonDetail = "dependent"
			}
			b.WriteString(fmt.Sprintf("  %s/ (%s: %s)\n", pkg, reason, strings.Join(files, ", ")))
			_ = reasonDetail
		} else {
			b.WriteString(fmt.Sprintf("  %s/ (direct: %s)\n", pkg, strings.Join(files, ", ")))
		}
	}

	// Command.
	cmd := GenerateTestCommand(selected, language)
	if cmd != "" {
		b.WriteString(fmt.Sprintf("\nCommand: %s\n", cmd))
	}

	// Estimate.
	selectedCount := len(selected.Tests)
	b.WriteString(fmt.Sprintf("Estimated: %d tests (vs %d+ in full suite)\n", selectedCount, totalTests))

	return b.String()
}

// EstimateTimeSaved returns a human-readable string estimating time savings.
func EstimateTimeSaved(totalTests, selectedTests int) string {
	if totalTests <= 0 || selectedTests <= 0 {
		return "No time estimate available"
	}
	if selectedTests >= totalTests {
		return "No time saved — running full suite"
	}

	// Assume average of 0.5s per test.
	totalTime := float64(totalTests) * 0.5
	selectedTime := float64(selectedTests) * 0.5
	saved := totalTime - selectedTime
	pct := (saved / totalTime) * 100

	if saved < 60 {
		return fmt.Sprintf("~%.0fs saved (%.0f%% faster, running %d/%d tests)", saved, pct, selectedTests, totalTests)
	}
	minutes := saved / 60
	return fmt.Sprintf("~%.1fm saved (%.0f%% faster, running %d/%d tests)", minutes, pct, selectedTests, totalTests)
}

// --- Internal helpers ---

// dtsParseDiffFiles extracts file paths from a unified diff.
func dtsParseDiffFiles(diff string) []string {
	var files []string
	seen := make(map[string]bool)

	diffFileRe := regexp.MustCompile(`^(?:diff --git a/(.+?) b/|[\+]{3} b/(.+)|--- a/(.+))`)
	scanner := bufio.NewScanner(strings.NewReader(diff))
	for scanner.Scan() {
		line := scanner.Text()
		matches := diffFileRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		for _, m := range matches[1:] {
			if m != "" && !seen[m] && m != "/dev/null" {
				seen[m] = true
				files = append(files, m)
			}
		}
	}

	return dtsDedup(files)
}

// dtsDetectLanguage determines the primary language of the project.
func dtsDetectLanguage(projectDir string) string {
	if _, err := os.Stat(filepath.Join(projectDir, "go.mod")); err == nil {
		return "go"
	}
	if _, err := os.Stat(filepath.Join(projectDir, "setup.py")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(projectDir, "pyproject.toml")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err == nil {
		return "javascript"
	}
	if _, err := os.Stat(filepath.Join(projectDir, "tsconfig.json")); err == nil {
		return "typescript"
	}
	return "go"
}

// detectModulePath reads the module path from go.mod.
func detectModulePath(projectDir string) string {
	modFile := filepath.Join(projectDir, "go.mod")
	f, err := os.Open(modFile)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

// parseGoImports extracts import paths from a Go source file.
func parseGoImports(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var imports []string
	inImportBlock := false

	scanner := bufio.NewScanner(f)
	importRe := regexp.MustCompile(`^\s*"(.+)"`)
	singleImportRe := regexp.MustCompile(`^import\s+"(.+)"`)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "import (") {
			inImportBlock = true
			continue
		}
		if inImportBlock {
			if trimmed == ")" {
				inImportBlock = false
				continue
			}
			matches := importRe.FindStringSubmatch(trimmed)
			if matches != nil {
				imports = append(imports, matches[1])
			}
			continue
		}
		if matches := singleImportRe.FindStringSubmatch(trimmed); matches != nil {
			imports = append(imports, matches[1])
		}
		// Stop scanning after the import section.
		if strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "type ") ||
			strings.HasPrefix(trimmed, "var ") || strings.HasPrefix(trimmed, "const ") {
			break
		}
	}

	return imports
}

// findGoRelatedTests finds Go test files related to the given source file.
func (ts *TestSelector) findGoRelatedTests(file string) []string {
	var tests []string

	dir := filepath.Dir(file)
	absDir := filepath.Join(ts.ProjectDir, dir)
	base := strings.TrimSuffix(filepath.Base(file), ".go")

	// Look for same-directory test files.
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return tests
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, "_test.go") {
			continue
		}
		// Direct test file for this source (e.g., foo.go -> foo_test.go).
		if name == base+"_test.go" {
			tests = append(tests, filepath.Join(dir, name))
			continue
		}
		// Also include other tests in the same package.
		tests = append(tests, filepath.Join(dir, name))
	}

	// Check for integration tests mentioning this file.
	integrationTests := findIntegrationTests(ts.ProjectDir, file)
	tests = append(tests, integrationTests...)

	return tests
}

// findPythonRelatedTests finds Python test files for the given module.
func (ts *TestSelector) findPythonRelatedTests(file string) []string {
	var tests []string

	dir := filepath.Dir(file)
	base := strings.TrimSuffix(filepath.Base(file), ".py")

	// Look for test_<module>.py in the same directory.
	testFile := filepath.Join(dir, "test_"+base+".py")
	absTestFile := filepath.Join(ts.ProjectDir, testFile)
	if _, err := os.Stat(absTestFile); err == nil {
		tests = append(tests, testFile)
	}

	// Look in tests/ subdirectory.
	testDir := filepath.Join(dir, "tests")
	testFile2 := filepath.Join(testDir, "test_"+base+".py")
	absTestFile2 := filepath.Join(ts.ProjectDir, testFile2)
	if _, err := os.Stat(absTestFile2); err == nil {
		tests = append(tests, testFile2)
	}

	// Look in a top-level tests/ directory.
	topTestFile := filepath.Join("tests", "test_"+base+".py")
	absTopTestFile := filepath.Join(ts.ProjectDir, topTestFile)
	if _, err := os.Stat(absTopTestFile); err == nil {
		tests = append(tests, topTestFile)
	}

	return tests
}

// findJSRelatedTests finds JavaScript/TypeScript test files for the given source.
func (ts *TestSelector) findJSRelatedTests(file string) []string {
	var tests []string

	dir := filepath.Dir(file)
	ext := filepath.Ext(file)
	base := strings.TrimSuffix(filepath.Base(file), ext)

	// Look for *.test.ts, *.spec.ts, *.test.js, *.spec.js.
	extensions := []string{".test.ts", ".spec.ts", ".test.js", ".spec.js", ".test.tsx", ".spec.tsx"}
	for _, testExt := range extensions {
		candidate := filepath.Join(dir, base+testExt)
		absCandidate := filepath.Join(ts.ProjectDir, candidate)
		if _, err := os.Stat(absCandidate); err == nil {
			tests = append(tests, candidate)
		}
	}

	// Also check __tests__ directory.
	testsDir := filepath.Join(dir, "__tests__")
	for _, testExt := range extensions {
		candidate := filepath.Join(testsDir, base+testExt)
		absCandidate := filepath.Join(ts.ProjectDir, candidate)
		if _, err := os.Stat(absCandidate); err == nil {
			tests = append(tests, candidate)
		}
	}

	return tests
}

// findIntegrationTests searches for integration test files that mention the given file.
func findIntegrationTests(projectDir, file string) []string {
	var results []string
	base := filepath.Base(file)
	pkg := filepath.Dir(file)

	// Check common integration test locations.
	integrationPaths := []string{
		"integration_test.go",
		filepath.Join(filepath.Dir(file), "integration_test.go"),
		filepath.Join("test", "integration_test.go"),
	}

	for _, ip := range integrationPaths {
		absPath := filepath.Join(projectDir, ip)
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		// Check if it mentions the changed file or package.
		if strings.Contains(string(content), base) || strings.Contains(string(content), pkg) {
			results = append(results, ip)
		}
	}

	return results
}

// findTestsInDir returns test files in a directory.
func findTestsInDir(dir, language string) []string {
	var tests []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return tests
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if isTestFile(e.Name(), language) {
			tests = append(tests, e.Name())
		}
	}
	return tests
}

// isTestFile checks if a file is a test file for the given language.
func isTestFile(file string, language string) bool {
	base := filepath.Base(file)
	switch language {
	case "go":
		return strings.HasSuffix(base, "_test.go")
	case "python":
		return strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py")
	case "javascript", "typescript":
		return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")
	default:
		return strings.HasSuffix(base, "_test.go")
	}
}

// countAllTests counts all test files in the project.
func countAllTests(projectDir, language string) int {
	count := 0
	_ = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if isTestFile(path, language) {
			count++
		}
		return nil
	})
	return count
}

// generateGoTestCommand creates a go test command for selected packages.
func generateGoTestCommand(selected *SelectedTests) string {
	if len(selected.Packages) == 0 {
		return ""
	}

	// Convert packages to go test paths.
	var pkgPaths []string
	for _, pkg := range selected.Packages {
		if pkg == "" || pkg == "." {
			pkgPaths = append(pkgPaths, "./...")
		} else {
			pkgPaths = append(pkgPaths, "./"+pkg+"/...")
		}
	}

	// Extract test function names for -run flag.
	testNames := extractTestFuncNames(selected.Tests)
	if len(testNames) > 0 && len(testNames) <= 10 {
		pattern := strings.Join(testNames, "|")
		return fmt.Sprintf("go test -run \"%s\" %s", pattern, strings.Join(pkgPaths, " "))
	}

	return fmt.Sprintf("go test %s", strings.Join(pkgPaths, " "))
}

// generatePythonTestCommand creates a pytest command for selected tests.
func generatePythonTestCommand(selected *SelectedTests) string {
	if len(selected.Tests) == 0 {
		return ""
	}
	return fmt.Sprintf("pytest %s", strings.Join(selected.Tests, " "))
}

// generateJSTestCommand creates a jest command for selected tests.
func generateJSTestCommand(selected *SelectedTests) string {
	if len(selected.Tests) == 0 {
		return ""
	}

	// Build a test path pattern.
	var patterns []string
	for _, t := range selected.Tests {
		base := filepath.Base(t)
		patterns = append(patterns, base)
	}
	pattern := strings.Join(patterns, "|")
	return fmt.Sprintf("jest --testPathPattern=\"%s\"", pattern)
}

// extractTestFuncNames parses test files to extract test function names.
func extractTestFuncNames(testFiles []string) []string {
	// For efficiency, return just the file base names as patterns.
	// A full implementation would parse the test files for func Test* names.
	var names []string
	seen := make(map[string]bool)
	funcRe := regexp.MustCompile(`^func (Test\w+)`)

	for _, tf := range testFiles {
		f, err := os.Open(tf)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			matches := funcRe.FindStringSubmatch(scanner.Text())
			if matches != nil && !seen[matches[1]] {
				seen[matches[1]] = true
				names = append(names, matches[1])
			}
		}
		_ = f.Close()
	}

	return names
}

// dedup removes duplicate strings from a slice while preserving order.
func dtsDedup(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// appendUnique appends val to slice only if it is not already present.
func appendUnique(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}
