// health_score.go rolls the individual signals
// (complexity, smells, dead code, doc coverage, ownership, duplication)
// into a single weighted HealthScore with per-dimension breakdowns,
// issue lists, and a letter grade. CompareScores produces a "before /
// after" diff suitable for commit messages or PR descriptions.
package repomap

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
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

// The per-dimension scorer methods (ScoreTestCoverage, ScoreDocumentation,
// ScoreComplexity, ScoreDependencies, ScoreCodeQuality, ScoreMaintainability,
// ScoreSecurity) live in health_score_dimensions.go.

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
			icon := icons.Alert()
			if issue.Severity == "error" {
				icon = icons.CloseThick()
			}
			sb.WriteString(fmt.Sprintf("  %s %s: %s\n", icon, issue.Dimension, issue.Description))
		}
	}

	if len(score.Strengths) > 0 {
		sb.WriteString("\nStrengths:\n")
		for _, s := range score.Strengths {
			sb.WriteString(fmt.Sprintf("  "+icons.CheckBold()+" %s\n", s))
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
			sb.WriteString(fmt.Sprintf("  "+icons.Alert()+" %s: %s\n", issue.Dimension, issue.Description))
		}
	}

	// Resolved issues
	resolved := findNewIssues(after, before)
	if len(resolved) > 0 {
		sb.WriteString(fmt.Sprintf("\nResolved Issues (%d):\n", len(resolved)))
		for _, issue := range resolved {
			sb.WriteString(fmt.Sprintf("  "+icons.CheckBold()+" %s: %s\n", issue.Dimension, issue.Description))
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
		switch n := n.(type) {
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
			if n.Op.String() == "&&" || n.Op.String() == "||" {
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
