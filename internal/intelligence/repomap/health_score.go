package repomap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// HealthScore represents the overall health assessment of a project.
type HealthScore struct {
	Overall    float64
	Dimensions map[string]float64
	Issues     []HealthIssue
	Strengths  []string
	Grade      string
}

// HealthIssue represents a specific health concern in a project.
type HealthIssue struct {
	Dimension   string
	Description string
	Severity    string
	File        string
	Suggestion  string
}

// HealthScorer evaluates project health across multiple dimensions.
type HealthScorer struct {
	Weights map[string]float64
	mu      sync.Mutex
}

// NewHealthScorer creates a HealthScorer with default dimension weights.
func NewHealthScorer() *HealthScorer {
	return &HealthScorer{
		Weights: map[string]float64{
			"test_coverage":   0.20,
			"documentation":   0.15,
			"complexity":      0.15,
			"dependencies":    0.10,
			"code_quality":    0.15,
			"maintainability": 0.10,
			"security":        0.15,
		},
	}
}

// Score evaluates the overall health of a project directory.
func (hs *HealthScorer) Score(projectDir string) (*HealthScore, error) {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	info, err := os.Stat(projectDir)
	if err != nil {
		return nil, fmt.Errorf("cannot access project directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", projectDir)
	}

	result := &HealthScore{
		Dimensions: make(map[string]float64),
		Issues:     []HealthIssue{},
		Strengths:  []string{},
	}

	// Run all dimension scorers
	type dimResult struct {
		name   string
		score  float64
		issues []HealthIssue
	}

	scorers := []struct {
		name string
		fn   func(string) (float64, []HealthIssue)
	}{
		{"test_coverage", hs.ScoreTestCoverage},
		{"documentation", hs.ScoreDocumentation},
		{"complexity", hs.ScoreComplexity},
		{"dependencies", hs.ScoreDependencies},
		{"code_quality", hs.ScoreCodeQuality},
		{"maintainability", hs.ScoreMaintainability},
		{"security", hs.ScoreSecurity},
	}

	var wg sync.WaitGroup
	results := make([]dimResult, len(scorers))

	for i, s := range scorers {
		wg.Add(1)
		go func(idx int, name string, fn func(string) (float64, []HealthIssue)) {
			defer wg.Done()
			score, issues := fn(projectDir)
			results[idx] = dimResult{name: name, score: score, issues: issues}
		}(i, s.name, s.fn)
	}
	wg.Wait()

	// Aggregate results
	weightedSum := 0.0
	for _, dr := range results {
		result.Dimensions[dr.name] = dr.score
		result.Issues = append(result.Issues, dr.issues...)
		weight := hs.Weights[dr.name]
		weightedSum += dr.score * weight
	}

	result.Overall = weightedSum

	// Assign grade
	result.Grade = assignGrade(result.Overall)

	// Identify strengths
	result.Strengths = identifyStrengths(result.Dimensions)

	return result, nil
}

// ScoreTestCoverage evaluates testing practices and coverage.
func (hs *HealthScorer) ScoreTestCoverage(dir string) (float64, []HealthIssue) {
	var issues []HealthIssue
	sourceFiles := 0
	testFiles := 0
	dirsWithSource := make(map[string]bool)
	dirsWithTests := make(map[string]bool)

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		relDir := filepath.Dir(path)
		if strings.HasSuffix(path, "_test.go") {
			testFiles++
			dirsWithTests[relDir] = true
		} else {
			sourceFiles++
			dirsWithSource[relDir] = true
		}
		return nil
	})

	if sourceFiles == 0 {
		return 100.0, issues
	}

	// Calculate test-to-source ratio
	ratio := float64(testFiles) / float64(sourceFiles)
	ratioScore := ratio * 100.0
	if ratioScore > 100.0 {
		ratioScore = 100.0
	}

	// Check directories without tests
	for d := range dirsWithSource {
		if !dirsWithTests[d] {
			rel, _ := filepath.Rel(dir, d)
			if rel == "" {
				rel = d
			}
			issues = append(issues, HealthIssue{
				Dimension:   "test_coverage",
				Description: fmt.Sprintf("%s has no tests", rel),
				Severity:    "warning",
				File:        d,
				Suggestion:  fmt.Sprintf("Add test files to %s", rel),
			})
		}
	}

	// Penalize for directories without tests
	if len(dirsWithSource) > 0 {
		coverageRatio := float64(len(dirsWithTests)) / float64(len(dirsWithSource))
		score := (ratioScore*0.6 + coverageRatio*100.0*0.4)
		if score > 100.0 {
			score = 100.0
		}
		return score, issues
	}

	return ratioScore, issues
}

// ScoreDocumentation evaluates documentation quality.
func (hs *HealthScorer) ScoreDocumentation(dir string) (float64, []HealthIssue) {
	var issues []HealthIssue
	score := 0.0
	checks := 0.0

	// Check for README
	readmeExists := false
	readmeNames := []string{"README.md", "README", "README.txt", "readme.md"}
	for _, name := range readmeNames {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			readmeExists = true
			break
		}
	}
	checks++
	if readmeExists {
		score += 100.0
	} else {
		issues = append(issues, HealthIssue{
			Dimension:   "documentation",
			Description: "No README file found",
			Severity:    "warning",
			File:        dir,
			Suggestion:  "Add a README.md with project overview and usage instructions",
		})
	}

	// Analyze exported function documentation
	totalExported := 0
	documentedExported := 0

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return nil
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if !fn.Name.IsExported() {
				continue
			}
			totalExported++
			if fn.Doc != nil && len(fn.Doc.List) > 0 {
				documentedExported++
			}
		}
		return nil
	})

	if totalExported > 0 {
		checks++
		docRatio := float64(documentedExported) / float64(totalExported) * 100.0
		score += docRatio

		if docRatio < 50.0 {
			issues = append(issues, HealthIssue{
				Dimension:   "documentation",
				Description: fmt.Sprintf("Only %.0f%% of exported functions are documented", docRatio),
				Severity:    "warning",
				File:        dir,
				Suggestion:  "Add doc comments to exported functions following Go conventions",
			})
		}
	}

	if checks == 0 {
		return 100.0, issues
	}
	return score / checks, issues
}

// ScoreComplexity evaluates code complexity across the project.
func (hs *HealthScorer) ScoreComplexity(dir string) (float64, []HealthIssue) {
	var issues []HealthIssue
	var complexities []int
	highComplexityCount := 0
	threshold := 10

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return nil
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			cc := calculateCyclomaticComplexity(fn)
			complexities = append(complexities, cc)
			if cc > threshold {
				highComplexityCount++
				rel, _ := filepath.Rel(dir, path)
				if rel == "" {
					rel = path
				}
				issues = append(issues, HealthIssue{
					Dimension:   "complexity",
					Description: fmt.Sprintf("Function %s has cyclomatic complexity %d (threshold: %d)", fn.Name.Name, cc, threshold),
					Severity:    severityForComplexity(cc),
					File:        rel,
					Suggestion:  fmt.Sprintf("Refactor %s to reduce branching", fn.Name.Name),
				})
			}
		}
		return nil
	})

	if len(complexities) == 0 {
		return 100.0, issues
	}

	// Calculate average complexity
	total := 0
	for _, c := range complexities {
		total += c
	}
	avg := float64(total) / float64(len(complexities))

	// Score based on average and high-complexity function ratio
	avgScore := 100.0 - (avg-1.0)*10.0
	if avgScore < 0 {
		avgScore = 0
	}
	if avgScore > 100.0 {
		avgScore = 100.0
	}

	highRatio := float64(highComplexityCount) / float64(len(complexities))
	ratioScore := (1.0 - highRatio) * 100.0

	score := avgScore*0.6 + ratioScore*0.4
	if score > 100.0 {
		score = 100.0
	}
	if score < 0 {
		score = 0
	}

	return score, issues
}

// ScoreDependencies evaluates dependency health.
func (hs *HealthScorer) ScoreDependencies(dir string) (float64, []HealthIssue) {
	var issues []HealthIssue
	score := 100.0

	// Check for go.mod
	goModPath := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		// No go.mod — might be a simple project or not Go
		return 80.0, issues
	}

	lines := strings.Split(string(data), "\n")
	depCount := 0
	inRequire := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "require (" {
			inRequire = true
			continue
		}
		if inRequire && trimmed == ")" {
			inRequire = false
			continue
		}
		if inRequire && trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			depCount++
		}
		// Single-line require
		if strings.HasPrefix(trimmed, "require ") && !strings.Contains(trimmed, "(") {
			depCount++
		}
	}

	// Penalize for excessive dependencies
	if depCount > 50 {
		penalty := float64(depCount-50) * 0.5
		if penalty > 30 {
			penalty = 30
		}
		score -= penalty
		issues = append(issues, HealthIssue{
			Dimension:   "dependencies",
			Description: fmt.Sprintf("High dependency count: %d direct dependencies", depCount),
			Severity:    "warning",
			File:        "go.mod",
			Suggestion:  "Review dependencies for unused or replaceable modules",
		})
	}

	// Check for replace directives (might indicate instability)
	replaceCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "replace ") {
			replaceCount++
		}
	}
	if replaceCount > 3 {
		score -= float64(replaceCount) * 2
		issues = append(issues, HealthIssue{
			Dimension:   "dependencies",
			Description: fmt.Sprintf("%d replace directives found", replaceCount),
			Severity:    "info",
			File:        "go.mod",
			Suggestion:  "Replace directives may indicate unstable dependencies",
		})
	}

	if score < 0 {
		score = 0
	}
	return score, issues
}

// ScoreCodeQuality evaluates overall code quality signals.
func (hs *HealthScorer) ScoreCodeQuality(dir string) (float64, []HealthIssue) {
	var issues []HealthIssue
	totalFiles := 0
	filesWithIssues := 0

	var longFiles []string
	var deadCodeFiles []string

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		totalFiles++
		hasIssue := false

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")

		// Check file length
		if len(lines) > 500 {
			hasIssue = true
			rel, _ := filepath.Rel(dir, path)
			if rel == "" {
				rel = path
			}
			longFiles = append(longFiles, rel)
		}

		// Check for potential dead code (commented-out functions)
		commentedFuncCount := 0
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "// func ") || strings.HasPrefix(trimmed, "//func ") {
				commentedFuncCount++
			}
		}
		if commentedFuncCount > 2 {
			hasIssue = true
			rel, _ := filepath.Rel(dir, path)
			if rel == "" {
				rel = path
			}
			deadCodeFiles = append(deadCodeFiles, rel)
		}

		if hasIssue {
			filesWithIssues++
		}
		return nil
	})

	if len(longFiles) > 0 {
		issues = append(issues, HealthIssue{
			Dimension:   "code_quality",
			Description: fmt.Sprintf("%d files exceed 500 lines", len(longFiles)),
			Severity:    "warning",
			File:        longFiles[0],
			Suggestion:  "Consider splitting large files into smaller, focused modules",
		})
	}

	if len(deadCodeFiles) > 0 {
		issues = append(issues, HealthIssue{
			Dimension:   "code_quality",
			Description: fmt.Sprintf("%d files contain commented-out code", len(deadCodeFiles)),
			Severity:    "info",
			File:        deadCodeFiles[0],
			Suggestion:  "Remove dead code; use version control for history",
		})
	}

	if totalFiles == 0 {
		return 100.0, issues
	}

	qualityRatio := 1.0 - float64(filesWithIssues)/float64(totalFiles)
	score := qualityRatio * 100.0
	if score < 0 {
		score = 0
	}
	return score, issues
}

// ScoreMaintainability evaluates how maintainable the codebase is.
func (hs *HealthScorer) ScoreMaintainability(dir string) (float64, []HealthIssue) {
	var issues []HealthIssue
	score := 100.0

	// Check package organization
	pkgCount := 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == ".git" || name == "vendor" || name == "node_modules" {
			return filepath.SkipDir
		}
		// Check if directory contains Go files
		entries, _ := os.ReadDir(path)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
				pkgCount++
				break
			}
		}
		return nil
	})

	// Check naming consistency
	inconsistentNames := 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		base := filepath.Base(path)
		// Check for mixed naming conventions (camelCase vs snake_case)
		if strings.Contains(base, "-") {
			inconsistentNames++
		}
		return nil
	})

	if inconsistentNames > 0 {
		score -= float64(inconsistentNames) * 5
		issues = append(issues, HealthIssue{
			Dimension:   "maintainability",
			Description: fmt.Sprintf("%d files use non-standard naming", inconsistentNames),
			Severity:    "info",
			File:        dir,
			Suggestion:  "Use snake_case for Go file names",
		})
	}

	// Check for consistent error handling patterns
	errPatterns := checkErrorPatterns(dir)
	if errPatterns < 0.7 {
		score -= 15
		issues = append(issues, HealthIssue{
			Dimension:   "maintainability",
			Description: "Inconsistent error handling patterns detected",
			Severity:    "warning",
			File:        dir,
			Suggestion:  "Standardize error handling with wrapped errors using fmt.Errorf with %w",
		})
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score, issues
}

// ScoreSecurity evaluates basic security signals in the codebase.
func (hs *HealthScorer) ScoreSecurity(dir string) (float64, []HealthIssue) {
	var issues []HealthIssue
	score := 100.0

	dangerousPatterns := []struct {
		pattern     string
		description string
		severity    string
		suggestion  string
	}{
		{"exec.Command", "Use of exec.Command may allow command injection", "warning", "Validate and sanitize all inputs to exec.Command"},
		{"os.Exec", "Direct OS exec calls detected", "warning", "Ensure executed commands are properly validated"},
		{"net/http", "HTTP usage without explicit TLS configuration", "info", "Consider enforcing HTTPS for external connections"},
		{"crypto/md5", "MD5 is cryptographically broken", "error", "Replace MD5 with SHA-256 or stronger"},
		{"crypto/sha1", "SHA-1 is deprecated for security purposes", "warning", "Replace SHA-1 with SHA-256 or stronger"},
		{"unsafe.Pointer", "Use of unsafe package bypasses type safety", "warning", "Avoid unsafe unless absolutely necessary"},
	}

	foundPatterns := make(map[string][]string)

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(data)
		rel, _ := filepath.Rel(dir, path)
		if rel == "" {
			rel = path
		}

		for _, dp := range dangerousPatterns {
			if strings.Contains(content, dp.pattern) {
				foundPatterns[dp.pattern] = append(foundPatterns[dp.pattern], rel)
			}
		}
		return nil
	})

	for _, dp := range dangerousPatterns {
		files := foundPatterns[dp.pattern]
		if len(files) > 0 {
			var penalty float64
			switch dp.severity {
			case "error":
				penalty = 15
			case "warning":
				penalty = 8
			case "info":
				penalty = 3
			}
			score -= penalty
			issues = append(issues, HealthIssue{
				Dimension:   "security",
				Description: fmt.Sprintf("%s (found in %d files)", dp.description, len(files)),
				Severity:    dp.severity,
				File:        files[0],
				Suggestion:  dp.suggestion,
			})
		}
	}

	if score < 0 {
		score = 0
	}
	return score, issues
}

// FormatScore produces a human-readable health report.
func FormatScore(score *HealthScore) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Project Health: %s (%.0f/100)\n", score.Grade, score.Overall))
	sb.WriteString(strings.Repeat("═", 31))
	sb.WriteString("\n\n")

	sb.WriteString("Dimensions:\n")

	// Sort dimensions for consistent output
	dimNames := []string{
		"test_coverage",
		"documentation",
		"complexity",
		"dependencies",
		"code_quality",
		"maintainability",
		"security",
	}

	displayNames := map[string]string{
		"test_coverage":   "Test Coverage",
		"documentation":   "Documentation",
		"complexity":      "Complexity",
		"dependencies":    "Dependencies",
		"code_quality":    "Code Quality",
		"maintainability": "Maintainability",
		"security":        "Security",
	}

	for _, name := range dimNames {
		val, ok := score.Dimensions[name]
		if !ok {
			continue
		}
		display := displayNames[name]
		bar := renderBar(val, 20)
		sb.WriteString(fmt.Sprintf("  %-16s %3.0f%%  %s\n", display+":", val, bar))
	}

	if len(score.Issues) > 0 {
		sb.WriteString(fmt.Sprintf("\nIssues (%d):\n", len(score.Issues)))
		for _, issue := range score.Issues {
			icon := "⚠"
			if issue.Severity == "error" {
				icon = "✗"
			}
			sb.WriteString(fmt.Sprintf("  %s %s: %s\n", icon, issue.Dimension, issue.Description))
		}
	}

	if len(score.Strengths) > 0 {
		sb.WriteString("\nStrengths:\n")
		for _, s := range score.Strengths {
			sb.WriteString(fmt.Sprintf("  ✓ %s\n", s))
		}
	}

	return sb.String()
}

// CompareScores produces a comparison between two health scores.
func CompareScores(before, after *HealthScore) string {
	var sb strings.Builder

	sb.WriteString("Health Score Comparison\n")
	sb.WriteString(strings.Repeat("═", 31))
	sb.WriteString("\n\n")

	// Overall change
	diff := after.Overall - before.Overall
	arrow := "↑"
	if diff < 0 {
		arrow = "↓"
	} else if diff == 0 {
		arrow = "→"
	}
	sb.WriteString(fmt.Sprintf("Overall: %s %s (%.0f) -> %s (%.0f)  %+.1f\n\n",
		arrow, before.Grade, before.Overall, after.Grade, after.Overall, diff))

	sb.WriteString("Dimensions:\n")

	dimNames := []string{
		"test_coverage",
		"documentation",
		"complexity",
		"dependencies",
		"code_quality",
		"maintainability",
		"security",
	}

	displayNames := map[string]string{
		"test_coverage":   "Test Coverage",
		"documentation":   "Documentation",
		"complexity":      "Complexity",
		"dependencies":    "Dependencies",
		"code_quality":    "Code Quality",
		"maintainability": "Maintainability",
		"security":        "Security",
	}

	for _, name := range dimNames {
		beforeVal := before.Dimensions[name]
		afterVal := after.Dimensions[name]
		d := afterVal - beforeVal
		a := " "
		if d > 0 {
			a = "↑"
		} else if d < 0 {
			a = "↓"
		}
		display := displayNames[name]
		sb.WriteString(fmt.Sprintf("  %-16s %3.0f%% -> %3.0f%%  %s %+.1f\n",
			display+":", beforeVal, afterVal, a, d))
	}

	// New issues
	newIssues := findNewIssues(before, after)
	if len(newIssues) > 0 {
		sb.WriteString(fmt.Sprintf("\nNew Issues (%d):\n", len(newIssues)))
		for _, issue := range newIssues {
			sb.WriteString(fmt.Sprintf("  ⚠ %s: %s\n", issue.Dimension, issue.Description))
		}
	}

	// Resolved issues
	resolved := findNewIssues(after, before)
	if len(resolved) > 0 {
		sb.WriteString(fmt.Sprintf("\nResolved Issues (%d):\n", len(resolved)))
		for _, issue := range resolved {
			sb.WriteString(fmt.Sprintf("  ✓ %s: %s\n", issue.Dimension, issue.Description))
		}
	}

	return sb.String()
}

// --- Internal helpers ---

func assignGrade(score float64) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}

func identifyStrengths(dimensions map[string]float64) []string {
	var strengths []string

	if v, ok := dimensions["documentation"]; ok && v >= 85 {
		strengths = append(strengths, "Well-documented public API")
	}
	if v, ok := dimensions["dependencies"]; ok && v >= 90 {
		strengths = append(strengths, "Few external dependencies")
	}
	if v, ok := dimensions["code_quality"]; ok && v >= 85 {
		strengths = append(strengths, "Consistent code style")
	}
	if v, ok := dimensions["test_coverage"]; ok && v >= 85 {
		strengths = append(strengths, "Strong test coverage")
	}
	if v, ok := dimensions["complexity"]; ok && v >= 85 {
		strengths = append(strengths, "Low code complexity")
	}
	if v, ok := dimensions["security"]; ok && v >= 90 {
		strengths = append(strengths, "Good security practices")
	}
	if v, ok := dimensions["maintainability"]; ok && v >= 85 {
		strengths = append(strengths, "Highly maintainable code")
	}

	return strengths
}

func renderBar(pct float64, width int) string {
	filled := int(pct / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func calculateCyclomaticComplexity(fn *ast.FuncDecl) int {
	if fn.Body == nil {
		return 1
	}
	complexity := 1
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt:
			complexity++
		case *ast.ForStmt:
			complexity++
		case *ast.RangeStmt:
			complexity++
		case *ast.CaseClause:
			complexity++
		case *ast.CommClause:
			complexity++
		case *ast.BinaryExpr:
			bin := n.(*ast.BinaryExpr)
			if bin.Op.String() == "&&" || bin.Op.String() == "||" {
				complexity++
			}
		}
		return true
	})
	return complexity
}

func severityForComplexity(cc int) string {
	switch {
	case cc > 20:
		return "error"
	case cc > 15:
		return "warning"
	default:
		return "info"
	}
}

func checkErrorPatterns(dir string) float64 {
	totalReturns := 0
	wrappedErrors := 0

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(data)
		lines := strings.Split(content, "\n")

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "return") && strings.Contains(trimmed, "err") {
				totalReturns++
				if strings.Contains(trimmed, "fmt.Errorf") && strings.Contains(trimmed, "%w") {
					wrappedErrors++
				}
			}
		}
		return nil
	})

	if totalReturns == 0 {
		return 1.0
	}
	return float64(wrappedErrors) / float64(totalReturns)
}

func findNewIssues(before, after *HealthScore) []HealthIssue {
	beforeSet := make(map[string]bool)
	for _, issue := range before.Issues {
		key := issue.Dimension + ":" + issue.Description
		beforeSet[key] = true
	}

	var newIssues []HealthIssue
	for _, issue := range after.Issues {
		key := issue.Dimension + ":" + issue.Description
		if !beforeSet[key] {
			newIssues = append(newIssues, issue)
		}
	}
	return newIssues
}

// sortIssuesBySeverity sorts issues with errors first, then warnings, then info.
func sortIssuesBySeverity(issues []HealthIssue) {
	severityOrder := map[string]int{"error": 0, "warning": 1, "info": 2}
	sort.Slice(issues, func(i, j int) bool {
		return severityOrder[issues[i].Severity] < severityOrder[issues[j].Severity]
	})
}
