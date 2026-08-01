package repomap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// This file holds the per-dimension HealthScorer methods (test coverage,
// documentation, complexity, dependencies, code quality, maintainability,
// security). The HealthScorer type, aggregation (Score), reporting
// (FormatScore/CompareScores), and shared helpers live in health_score.go.

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
	data, err := os.ReadFile(goModPath) // #nosec G304 -- goModPath is the go.mod of the repo directory being analyzed by this dev CLI
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

		data, readErr := os.ReadFile(path) // #nosec G304,G122 -- read-only repository analysis
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

		data, readErr := os.ReadFile(path) // #nosec G304,G122 -- read-only repository analysis
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
