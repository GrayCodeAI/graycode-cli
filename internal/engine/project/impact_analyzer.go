package project

import (
	"bufio"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ImpactAnalysis holds the results of analyzing the blast radius of code changes.
type ImpactAnalysis struct {
	ChangedFiles         []string
	DirectlyAffected     []string
	TransitivelyAffected []string
	RiskScore            float64
	TestCoverage         float64
	Suggestions          []string
}

// ImpactAnalyzer predicts the blast radius of code changes before they're applied.
type ImpactAnalyzer struct {
	ProjectDir  string
	ImportGraph map[string][]string // reverse dependency: pkg -> list of packages that import it
	TestMapping map[string][]string // pkg -> list of test files for that pkg
	mu          sync.RWMutex
}

// NewImpactAnalyzer creates a new ImpactAnalyzer for the given project directory.
func NewImpactAnalyzer(projectDir string) *ImpactAnalyzer {
	return &ImpactAnalyzer{
		ProjectDir:  projectDir,
		ImportGraph: make(map[string][]string),
		TestMapping: make(map[string][]string),
	}
}

// Analyze performs a full impact analysis for the given changed files.
func (ia *ImpactAnalyzer) Analyze(changedFiles []string) (*ImpactAnalysis, error) {
	ia.mu.Lock()
	ia.ImportGraph = BuildImportGraph(ia.ProjectDir)
	ia.buildTestMapping()
	ia.mu.Unlock()

	analysis := &ImpactAnalysis{
		ChangedFiles: changedFiles,
	}

	// Collect unique affected packages from changed files.
	changedPkgs := make(map[string]bool)
	for _, f := range changedFiles {
		pkg := impactFileToPackage(f, ia.ProjectDir)
		changedPkgs[pkg] = true
	}

	// Find direct dependents.
	directSet := make(map[string]bool)
	for pkg := range changedPkgs {
		for _, dep := range ia.FindDirectDependents(pkg) {
			if !changedPkgs[dep] {
				directSet[dep] = true
			}
		}
	}
	for pkg := range directSet {
		analysis.DirectlyAffected = append(analysis.DirectlyAffected, pkg)
	}
	sort.Strings(analysis.DirectlyAffected)

	// Find transitive dependents.
	transitiveSet := make(map[string]bool)
	for pkg := range changedPkgs {
		for _, dep := range ia.FindTransitiveDependents(pkg, 10) {
			if !changedPkgs[dep] && !directSet[dep] {
				transitiveSet[dep] = true
			}
		}
	}
	for pkg := range transitiveSet {
		analysis.TransitivelyAffected = append(analysis.TransitivelyAffected, pkg)
	}
	sort.Strings(analysis.TransitivelyAffected)

	// Score risk and find test coverage.
	analysis.RiskScore = ia.ScoreRisk(analysis)
	allAffected := append(append([]string{}, analysis.DirectlyAffected...), analysis.TransitivelyAffected...)
	analysis.TestCoverage = ia.FindTestCoverage(allAffected)

	// Generate suggestions.
	analysis.Suggestions = ia.GenerateSuggestions(analysis)

	return analysis, nil
}

// BuildImportGraph parses Go source files to build a reverse dependency graph.
// Package A imports B means B's map entry includes A.
func BuildImportGraph(projectDir string) map[string][]string {
	graph := make(map[string][]string)
	fset := token.NewFileSet()

	// Detect the module path from go.mod.
	modulePath := detectModulePath(projectDir)

	_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip vendor and hidden directories.
		if d.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
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

		f, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return nil
		}

		// Determine this file's package path.
		dir := filepath.Dir(path)
		relDir, relErr := filepath.Rel(projectDir, dir)
		if relErr != nil {
			return nil
		}
		var thisPkg string
		if relDir == "." {
			thisPkg = modulePath
		} else {
			thisPkg = modulePath + "/" + filepath.ToSlash(relDir)
		}

		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			// Only track internal imports (within the same module).
			if strings.HasPrefix(importPath, modulePath) {
				// importPath is imported by thisPkg.
				// Reverse graph: importPath -> thisPkg depends on it.
				graph[importPath] = appendUnique(graph[importPath], thisPkg)
			}
		}
		return nil
	})

	return graph
}

// FindDirectDependents returns packages that directly import the given package.
func (ia *ImpactAnalyzer) FindDirectDependents(pkg string) []string {
	ia.mu.RLock()
	defer ia.mu.RUnlock()

	deps := ia.ImportGraph[pkg]
	result := make([]string, len(deps))
	copy(result, deps)
	return result
}

// FindTransitiveDependents performs BFS up to depth levels of reverse dependencies.
func (ia *ImpactAnalyzer) FindTransitiveDependents(pkg string, depth int) []string {
	ia.mu.RLock()
	defer ia.mu.RUnlock()

	visited := make(map[string]bool)
	visited[pkg] = true
	queue := []string{pkg}
	var result []string

	for level := 0; level < depth && len(queue) > 0; level++ {
		var next []string
		for _, current := range queue {
			for _, dep := range ia.ImportGraph[current] {
				if !visited[dep] {
					visited[dep] = true
					next = append(next, dep)
					result = append(result, dep)
				}
			}
		}
		queue = next
	}

	return result
}

// ScoreRisk calculates a risk score from 0.0 to 1.0 based on various factors.
func (ia *ImpactAnalyzer) ScoreRisk(analysis *ImpactAnalysis) float64 {
	score := 0.0

	// Factor 1: Number of affected packages (max contribution 0.4).
	totalAffected := len(analysis.DirectlyAffected) + len(analysis.TransitivelyAffected)
	affectedScore := float64(totalAffected) / 20.0
	if affectedScore > 0.4 {
		affectedScore = 0.4
	}
	score += affectedScore

	// Factor 2: Presence of cmd/main packages (contribution 0.25).
	allAffected := append(append([]string{}, analysis.DirectlyAffected...), analysis.TransitivelyAffected...)
	for _, pkg := range allAffected {
		if strings.Contains(pkg, "/cmd/") || strings.HasSuffix(pkg, "/cmd") || strings.Contains(pkg, "main") {
			score += 0.25
			break
		}
	}

	// Factor 3: Number of changed files (max contribution 0.2).
	changedScore := float64(len(analysis.ChangedFiles)) / 10.0
	if changedScore > 0.2 {
		changedScore = 0.2
	}
	score += changedScore

	// Factor 4: Test coverage penalty — less coverage means higher risk (max 0.15).
	allPkgs := append(append([]string{}, analysis.DirectlyAffected...), analysis.TransitivelyAffected...)
	coverage := ia.FindTestCoverage(allPkgs)
	coveragePenalty := (1.0 - coverage) * 0.15
	score += coveragePenalty

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// FindTestCoverage checks which affected packages have test files and returns
// the percentage of packages with at least one test file.
func (ia *ImpactAnalyzer) FindTestCoverage(packages []string) float64 {
	if len(packages) == 0 {
		return 1.0
	}

	ia.mu.RLock()
	defer ia.mu.RUnlock()

	withTests := 0
	for _, pkg := range packages {
		if tests, ok := ia.TestMapping[pkg]; ok && len(tests) > 0 {
			withTests++
		} else {
			// Fallback: check if the directory has test files.
			pkgDir := ia.packageToDir(pkg)
			if hasTestFiles(pkgDir) {
				withTests++
			}
		}
	}
	return float64(withTests) / float64(len(packages))
}

// GenerateSuggestions produces actionable suggestions based on the analysis.
func (ia *ImpactAnalyzer) GenerateSuggestions(analysis *ImpactAnalysis) []string {
	var suggestions []string

	// Suggest running tests for directly affected packages.
	if len(analysis.DirectlyAffected) > 0 {
		testPaths := make([]string, 0, len(analysis.DirectlyAffected))
		for _, pkg := range analysis.DirectlyAffected {
			short := ia.shortPkg(pkg)
			testPaths = append(testPaths, "./"+short+"/...")
		}
		suggestions = append(suggestions, fmt.Sprintf("Run: go test %s", strings.Join(testPaths, " ")))
	}

	// Suggest review for cmd packages.
	allAffected := append(append([]string{}, analysis.DirectlyAffected...), analysis.TransitivelyAffected...)
	for _, pkg := range allAffected {
		if strings.Contains(pkg, "/cmd/") || strings.HasSuffix(pkg, "/cmd") {
			short := ia.shortPkg(pkg)
			suggestions = append(suggestions, fmt.Sprintf("Review %s for integration impact", short))
			break
		}
	}

	// High risk warning.
	totalAffected := len(analysis.DirectlyAffected) + len(analysis.TransitivelyAffected)
	if totalAffected > 8 {
		suggestions = append(suggestions, fmt.Sprintf("High risk: %d packages affected transitively", totalAffected))
	}

	// Suggest adding tests for uncovered packages.
	for _, pkg := range allAffected {
		pkgDir := ia.packageToDir(pkg)
		if !hasTestFiles(pkgDir) {
			short := ia.shortPkg(pkg)
			suggestions = append(suggestions, fmt.Sprintf("Consider adding tests for %s", short))
		}
	}

	// Suggest integration tests if server/daemon is affected.
	for _, pkg := range allAffected {
		if strings.Contains(pkg, "server") || strings.Contains(pkg, "daemon") {
			suggestions = append(suggestions, "Consider integration tests: server/daemon depends on changed code")
			break
		}
	}

	return suggestions
}

// FormatImpact produces a human-readable formatted report of the impact analysis.
func FormatImpact(analysis *ImpactAnalysis) string {
	var b strings.Builder

	b.WriteString("Change Impact Analysis:\n")
	b.WriteString("═══════════════════════════════════\n")

	// Changed files.
	shortFiles := make([]string, len(analysis.ChangedFiles))
	copy(shortFiles, analysis.ChangedFiles)
	b.WriteString(fmt.Sprintf("Changed: %s\n", strings.Join(shortFiles, ", ")))
	b.WriteString("\n")

	// Risk level.
	riskLabel := "LOW"
	if analysis.RiskScore >= 0.6 {
		riskLabel = "HIGH"
	} else if analysis.RiskScore >= 0.3 {
		riskLabel = "MEDIUM"
	}
	b.WriteString(fmt.Sprintf("Risk: %s (%.2f)\n", riskLabel, analysis.RiskScore))
	b.WriteString("\n")

	// Direct dependents.
	if len(analysis.DirectlyAffected) > 0 {
		b.WriteString(fmt.Sprintf("Direct dependents (%d):\n", len(analysis.DirectlyAffected)))
		for _, pkg := range analysis.DirectlyAffected {
			b.WriteString(fmt.Sprintf("  %s\n", pkg))
		}
		b.WriteString("\n")
	}

	// Transitive dependents.
	if len(analysis.TransitivelyAffected) > 0 {
		b.WriteString(fmt.Sprintf("Transitive dependents (%d):\n", len(analysis.TransitivelyAffected)))
		maxShow := 5
		for i, pkg := range analysis.TransitivelyAffected {
			if i >= maxShow {
				b.WriteString(fmt.Sprintf("  ... and %d more\n", len(analysis.TransitivelyAffected)-maxShow))
				break
			}
			b.WriteString(fmt.Sprintf("  %s\n", pkg))
		}
		b.WriteString("\n")
	}

	// Test coverage.
	allAffected := len(analysis.DirectlyAffected) + len(analysis.TransitivelyAffected)
	withTests := int(analysis.TestCoverage * float64(allAffected))
	b.WriteString(fmt.Sprintf("Test coverage: %.0f%% (%d/%d affected packages have tests)\n",
		analysis.TestCoverage*100, withTests, allAffected))
	b.WriteString("\n")

	// Suggestions.
	if len(analysis.Suggestions) > 0 {
		b.WriteString("Suggestions:\n")
		for _, s := range analysis.Suggestions {
			b.WriteString(fmt.Sprintf("  • %s\n", s))
		}
	}

	return b.String()
}

// QuickImpact provides a fast single-file impact summary (one line).
func (ia *ImpactAnalyzer) QuickImpact(file string) string {
	ia.mu.Lock()
	if len(ia.ImportGraph) == 0 {
		ia.ImportGraph = BuildImportGraph(ia.ProjectDir)
	}
	ia.mu.Unlock()

	pkg := impactFileToPackage(file, ia.ProjectDir)
	direct := ia.FindDirectDependents(pkg)
	transitive := ia.FindTransitiveDependents(pkg, 10)

	// Deduplicate: remove direct from transitive count.
	directSet := make(map[string]bool)
	for _, d := range direct {
		directSet[d] = true
	}
	transitiveOnly := 0
	for _, t := range transitive {
		if !directSet[t] {
			transitiveOnly++
		}
	}

	shortPkg := ia.shortPkg(pkg)
	return fmt.Sprintf("%s: %d direct, %d transitive dependents", shortPkg, len(direct), transitiveOnly)
}

// --- Helper functions ---

// impactFileToPackage converts a file path to its package import path.
func impactFileToPackage(file string, projectDir string) string {
	modulePath := detectModulePath(projectDir)

	// Get directory of the file.
	dir := filepath.Dir(file)

	// If the file path is relative, join with project dir.
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(projectDir, dir)
	}

	relDir, err := filepath.Rel(projectDir, dir)
	if err != nil {
		return modulePath
	}
	if relDir == "." {
		return modulePath
	}
	return modulePath + "/" + filepath.ToSlash(relDir)
}

// buildTestMapping scans the project to map packages to their test files.
func (ia *ImpactAnalyzer) buildTestMapping() {
	modulePath := detectModulePath(ia.ProjectDir)
	ia.TestMapping = make(map[string][]string)

	_ = filepath.WalkDir(ia.ProjectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			dir := filepath.Dir(path)
			relDir, relErr := filepath.Rel(ia.ProjectDir, dir)
			if relErr != nil {
				return nil
			}
			var pkg string
			if relDir == "." {
				pkg = modulePath
			} else {
				pkg = modulePath + "/" + filepath.ToSlash(relDir)
			}
			ia.TestMapping[pkg] = append(ia.TestMapping[pkg], path)
		}
		return nil
	})
}

// packageToDir converts a package import path to a filesystem directory.
func (ia *ImpactAnalyzer) packageToDir(pkg string) string {
	modulePath := detectModulePath(ia.ProjectDir)
	if pkg == modulePath {
		return ia.ProjectDir
	}
	rel := strings.TrimPrefix(pkg, modulePath+"/")
	return filepath.Join(ia.ProjectDir, filepath.FromSlash(rel))
}

// shortPkg extracts a short relative package path from a full import path.
func (ia *ImpactAnalyzer) shortPkg(pkg string) string {
	modulePath := detectModulePath(ia.ProjectDir)
	if pkg == modulePath {
		return "."
	}
	return strings.TrimPrefix(pkg, modulePath+"/")
}

// hasTestFiles checks if a directory contains any _test.go files.
func hasTestFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			return true
		}
	}
	return false
}

// appendUnique appends s to slice only if not already present.
func appendUnique(slice []string, s string) []string {
	for _, existing := range slice {
		if existing == s {
			return slice
		}
	}
	return append(slice, s)
}

// detectModulePath reads go.mod to find the module path.
func detectModulePath(projectDir string) string {
	modFile := filepath.Join(projectDir, "go.mod")
	f, err := os.Open(modFile) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
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
