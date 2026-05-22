package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewHealthScorer(t *testing.T) {
	hs := NewHealthScorer()
	if hs == nil {
		t.Fatal("NewHealthScorer returned nil")
	}

	expectedWeights := map[string]float64{
		"test_coverage":   0.20,
		"documentation":   0.15,
		"complexity":      0.15,
		"dependencies":    0.10,
		"code_quality":    0.15,
		"maintainability": 0.10,
		"security":        0.15,
	}

	for dim, expected := range expectedWeights {
		got, ok := hs.Weights[dim]
		if !ok {
			t.Errorf("missing weight for dimension %q", dim)
			continue
		}
		if got != expected {
			t.Errorf("weight for %q: got %f, want %f", dim, got, expected)
		}
	}

	// Verify weights sum to 1.0
	sum := 0.0
	for _, w := range hs.Weights {
		sum += w
	}
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("weights sum to %f, expected 1.0", sum)
	}
}

func TestScore_InvalidDir(t *testing.T) {
	hs := NewHealthScorer()
	_, err := hs.Score("/nonexistent/path/xyz123")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestScore_NotADir(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "health_test_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	hs := NewHealthScorer()
	_, err = hs.Score(tmpFile.Name())
	if err == nil {
		t.Error("expected error for file path")
	}
}

func TestScore_EmptyDir(t *testing.T) {
	dir, err := os.MkdirTemp("", "health_test_empty_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	hs := NewHealthScorer()
	score, err := hs.Score(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score == nil {
		t.Fatal("score is nil")
	}
	if score.Grade == "" {
		t.Error("grade should not be empty")
	}
}

func TestScore_WellStructuredProject(t *testing.T) {
	dir := setupHealthTestProject(t, true)
	defer os.RemoveAll(dir)

	hs := NewHealthScorer()
	score, err := hs.Score(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if score.Overall < 0 || score.Overall > 100 {
		t.Errorf("overall score out of range: %f", score.Overall)
	}

	validGrades := map[string]bool{"A": true, "B": true, "C": true, "D": true, "F": true}
	if !validGrades[score.Grade] {
		t.Errorf("invalid grade: %q", score.Grade)
	}

	// Should have all dimensions
	expectedDims := []string{"test_coverage", "documentation", "complexity", "dependencies", "code_quality", "maintainability", "security"}
	for _, dim := range expectedDims {
		if _, ok := score.Dimensions[dim]; !ok {
			t.Errorf("missing dimension: %q", dim)
		}
	}
}

func TestScoreTestCoverage(t *testing.T) {
	dir := setupHealthTestProject(t, true)
	defer os.RemoveAll(dir)

	hs := NewHealthScorer()
	score, issues := hs.ScoreTestCoverage(dir)

	if score < 0 || score > 100 {
		t.Errorf("test coverage score out of range: %f", score)
	}
	// Project has test files, should score reasonably
	if score < 30 {
		t.Errorf("test coverage too low for project with tests: %f", score)
	}

	for _, issue := range issues {
		if issue.Dimension != "test_coverage" {
			t.Errorf("issue dimension should be test_coverage, got %q", issue.Dimension)
		}
	}
}

func TestScoreTestCoverage_NoTests(t *testing.T) {
	dir, err := os.MkdirTemp("", "health_test_notests_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Create source file without test
	writeHealthFile(t, filepath.Join(dir, "main.go"), `package main

func main() {}
func helper() {}
`)

	hs := NewHealthScorer()
	score, issues := hs.ScoreTestCoverage(dir)

	if score >= 50 {
		t.Errorf("score too high for project without tests: %f", score)
	}
	if len(issues) == 0 {
		t.Error("expected issues for project without tests")
	}
}

func TestScoreDocumentation(t *testing.T) {
	dir := setupHealthTestProject(t, true)
	defer os.RemoveAll(dir)

	hs := NewHealthScorer()
	score, issues := hs.ScoreDocumentation(dir)

	if score < 0 || score > 100 {
		t.Errorf("documentation score out of range: %f", score)
	}
	// Has README, should score well
	if score < 50 {
		t.Errorf("documentation score too low for project with README: %f", score)
	}

	for _, issue := range issues {
		if issue.Dimension != "documentation" {
			t.Errorf("issue dimension should be documentation, got %q", issue.Dimension)
		}
	}
}

func TestScoreDocumentation_NoReadme(t *testing.T) {
	dir, err := os.MkdirTemp("", "health_test_noreadme_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	writeHealthFile(t, filepath.Join(dir, "main.go"), `package main

// Run executes the main logic.
func Run() {}
`)

	hs := NewHealthScorer()
	_, issues := hs.ScoreDocumentation(dir)

	hasReadmeIssue := false
	for _, issue := range issues {
		if strings.Contains(issue.Description, "README") {
			hasReadmeIssue = true
		}
	}
	if !hasReadmeIssue {
		t.Error("expected README issue")
	}
}

func TestScoreComplexity(t *testing.T) {
	dir, err := os.MkdirTemp("", "health_test_complexity_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Simple function
	writeHealthFile(t, filepath.Join(dir, "simple.go"), `package pkg

func Simple(x int) int {
	return x + 1
}
`)

	hs := NewHealthScorer()
	score, issues := hs.ScoreComplexity(dir)

	if score < 80 {
		t.Errorf("simple function should have high complexity score: %f", score)
	}
	if len(issues) > 0 {
		t.Errorf("simple function should have no complexity issues, got %d", len(issues))
	}
}

func TestScoreComplexity_HighComplexity(t *testing.T) {
	dir, err := os.MkdirTemp("", "health_test_highcc_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Complex function with many branches
	writeHealthFile(t, filepath.Join(dir, "complex.go"), `package pkg

func Complex(a, b, c, d int) int {
	if a > 0 {
		if b > 0 {
			if c > 0 {
				if d > 0 {
					return 1
				} else if d < -1 {
					return 2
				}
			} else if c < -1 {
				return 3
			} else if c == 0 {
				return 4
			}
		} else if b < -1 {
			for i := 0; i < a; i++ {
				if i%2 == 0 {
					continue
				}
			}
			return 5
		}
	} else if a < 0 {
		switch {
		case b > 0:
			return 6
		case b < 0:
			return 7
		case c > 0:
			return 8
		}
	}
	return 0
}
`)

	hs := NewHealthScorer()
	score, issues := hs.ScoreComplexity(dir)

	if score > 80 {
		t.Errorf("complex function should have lower score: %f", score)
	}
	if len(issues) == 0 {
		t.Error("expected complexity issues for high-CC function")
	}
}

func TestScoreDependencies(t *testing.T) {
	dir, err := os.MkdirTemp("", "health_test_deps_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	writeHealthFile(t, filepath.Join(dir, "go.mod"), `module example.com/test

go 1.21

require (
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.8.4
)
`)

	hs := NewHealthScorer()
	score, issues := hs.ScoreDependencies(dir)

	if score < 90 {
		t.Errorf("few deps should score high: %f", score)
	}
	_ = issues
}

func TestScoreDependencies_ManyDeps(t *testing.T) {
	dir, err := os.MkdirTemp("", "health_test_manydeps_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	var deps strings.Builder
	deps.WriteString("module example.com/test\n\ngo 1.21\n\nrequire (\n")
	for i := 0; i < 60; i++ {
		deps.WriteString("\tgithub.com/example/dep")
		deps.WriteString(strings.Repeat("x", i%10))
		deps.WriteString(" v1.0.0\n")
	}
	deps.WriteString(")\n")

	writeHealthFile(t, filepath.Join(dir, "go.mod"), deps.String())

	hs := NewHealthScorer()
	score, issues := hs.ScoreDependencies(dir)

	if score >= 100 {
		t.Errorf("many deps should reduce score: %f", score)
	}
	if len(issues) == 0 {
		t.Error("expected issues for many dependencies")
	}
}

func TestScoreCodeQuality(t *testing.T) {
	dir := setupHealthTestProject(t, true)
	defer os.RemoveAll(dir)

	hs := NewHealthScorer()
	score, issues := hs.ScoreCodeQuality(dir)

	if score < 0 || score > 100 {
		t.Errorf("code quality score out of range: %f", score)
	}

	for _, issue := range issues {
		if issue.Dimension != "code_quality" {
			t.Errorf("issue dimension should be code_quality, got %q", issue.Dimension)
		}
	}
}

func TestScoreSecurity(t *testing.T) {
	dir, err := os.MkdirTemp("", "health_test_security_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	writeHealthFile(t, filepath.Join(dir, "safe.go"), `package pkg

import "fmt"

func Safe() {
	fmt.Println("hello")
}
`)

	hs := NewHealthScorer()
	score, _ := hs.ScoreSecurity(dir)

	if score < 90 {
		t.Errorf("safe code should have high security score: %f", score)
	}
}

func TestScoreSecurity_DangerousPatterns(t *testing.T) {
	dir, err := os.MkdirTemp("", "health_test_unsafe_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	writeHealthFile(t, filepath.Join(dir, "risky.go"), `package pkg

import (
	"crypto/md5"
	"os/exec"
)

func Risky(cmd string) {
	_ = exec.CommandContext(context.Background(), cmd)
	_ = md5.Sum([]byte("data"))
}
`)

	hs := NewHealthScorer()
	score, issues := hs.ScoreSecurity(dir)

	if score >= 90 {
		t.Errorf("risky code should have lower security score: %f", score)
	}
	if len(issues) == 0 {
		t.Error("expected security issues for dangerous patterns")
	}
}

func TestFormatScore(t *testing.T) {
	score := &HealthScore{
		Overall: 82,
		Grade:   "B",
		Dimensions: map[string]float64{
			"test_coverage":   75,
			"documentation":   90,
			"complexity":      70,
			"dependencies":    95,
			"code_quality":    80,
			"maintainability": 85,
			"security":        78,
		},
		Issues: []HealthIssue{
			{Dimension: "complexity", Description: "5 functions exceed CC threshold", Severity: "warning"},
			{Dimension: "test_coverage", Description: "pkg/handler/ has no tests", Severity: "warning"},
		},
		Strengths: []string{
			"Well-documented public API",
			"Few external dependencies",
		},
	}

	output := FormatScore(score)

	if !strings.Contains(output, "Project Health: B (82/100)") {
		t.Error("output should contain grade and score")
	}
	if !strings.Contains(output, "Test Coverage:") {
		t.Error("output should contain dimension names")
	}
	if !strings.Contains(output, "Issues (2)") {
		t.Error("output should contain issue count")
	}
	if !strings.Contains(output, "Strengths:") {
		t.Error("output should contain strengths section")
	}
	if !strings.Contains(output, "█") {
		t.Error("output should contain bar chart characters")
	}
	if !strings.Contains(output, "Well-documented public API") {
		t.Error("output should contain strength text")
	}
}

func TestFormatScore_NoIssues(t *testing.T) {
	score := &HealthScore{
		Overall: 95,
		Grade:   "A",
		Dimensions: map[string]float64{
			"test_coverage":   95,
			"documentation":   92,
			"complexity":      96,
			"dependencies":    98,
			"code_quality":    94,
			"maintainability": 93,
			"security":        97,
		},
		Issues:    []HealthIssue{},
		Strengths: []string{"Excellent overall quality"},
	}

	output := FormatScore(score)
	if strings.Contains(output, "Issues (") {
		t.Error("should not show issues section when empty")
	}
}

func TestCompareScores(t *testing.T) {
	before := &HealthScore{
		Overall: 72,
		Grade:   "C",
		Dimensions: map[string]float64{
			"test_coverage":   60,
			"documentation":   80,
			"complexity":      70,
			"dependencies":    90,
			"code_quality":    65,
			"maintainability": 75,
			"security":        70,
		},
		Issues: []HealthIssue{
			{Dimension: "test_coverage", Description: "low coverage"},
		},
	}

	after := &HealthScore{
		Overall: 85,
		Grade:   "B",
		Dimensions: map[string]float64{
			"test_coverage":   80,
			"documentation":   85,
			"complexity":      75,
			"dependencies":    90,
			"code_quality":    85,
			"maintainability": 80,
			"security":        85,
		},
		Issues: []HealthIssue{
			{Dimension: "security", Description: "new warning"},
		},
	}

	output := CompareScores(before, after)

	if !strings.Contains(output, "Health Score Comparison") {
		t.Error("output should contain title")
	}
	if !strings.Contains(output, "C") && !strings.Contains(output, "B") {
		t.Error("output should contain grades")
	}
	if !strings.Contains(output, "↑") {
		t.Error("output should contain improvement arrows")
	}
	if !strings.Contains(output, "New Issues") {
		t.Error("output should show new issues")
	}
	if !strings.Contains(output, "Resolved Issues") {
		t.Error("output should show resolved issues")
	}
}

func TestAssignGrade(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{95, "A"},
		{90, "A"},
		{89, "B"},
		{80, "B"},
		{79, "C"},
		{70, "C"},
		{69, "D"},
		{60, "D"},
		{59, "F"},
		{0, "F"},
	}

	for _, tt := range tests {
		got := assignGrade(tt.score)
		if got != tt.want {
			t.Errorf("assignGrade(%f) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestRenderBar(t *testing.T) {
	tests := []struct {
		pct   float64
		width int
		want  int // expected filled chars
	}{
		{100, 20, 20},
		{50, 20, 10},
		{0, 20, 0},
		{75, 20, 15},
	}

	for _, tt := range tests {
		bar := renderBar(tt.pct, tt.width)
		if len([]rune(bar)) != tt.width {
			t.Errorf("renderBar(%f, %d) length = %d, want %d", tt.pct, tt.width, len([]rune(bar)), tt.width)
		}
		filled := strings.Count(bar, "█")
		if filled != tt.want {
			t.Errorf("renderBar(%f, %d) filled = %d, want %d", tt.pct, tt.width, filled, tt.want)
		}
	}
}

func TestIdentifyStrengths(t *testing.T) {
	dims := map[string]float64{
		"documentation": 90,
		"dependencies":  95,
		"code_quality":  90,
		"test_coverage": 50,
		"complexity":    60,
		"security":      95,
	}

	strengths := identifyStrengths(dims)
	if len(strengths) == 0 {
		t.Error("expected strengths for high-scoring dimensions")
	}

	// Should include docs strength
	found := false
	for _, s := range strengths {
		if strings.Contains(s, "document") || strings.Contains(s, "Document") {
			found = true
		}
	}
	if !found {
		t.Error("expected documentation strength")
	}
}

func TestSortIssuesBySeverity(t *testing.T) {
	issues := []HealthIssue{
		{Severity: "info", Description: "info issue"},
		{Severity: "error", Description: "error issue"},
		{Severity: "warning", Description: "warning issue"},
	}

	sortIssuesBySeverity(issues)

	if issues[0].Severity != "error" {
		t.Errorf("first issue should be error, got %q", issues[0].Severity)
	}
	if issues[1].Severity != "warning" {
		t.Errorf("second issue should be warning, got %q", issues[1].Severity)
	}
	if issues[2].Severity != "info" {
		t.Errorf("third issue should be info, got %q", issues[2].Severity)
	}
}

func TestScoreMaintainability(t *testing.T) {
	dir := setupHealthTestProject(t, true)
	defer os.RemoveAll(dir)

	hs := NewHealthScorer()
	score, _ := hs.ScoreMaintainability(dir)

	if score < 0 || score > 100 {
		t.Errorf("maintainability score out of range: %f", score)
	}
}

func TestConcurrentScoring(t *testing.T) {
	dir := setupHealthTestProject(t, true)
	defer os.RemoveAll(dir)

	hs := NewHealthScorer()

	// Run multiple scores concurrently to test thread safety
	done := make(chan struct{}, 5)
	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			score, err := hs.Score(dir)
			if err != nil {
				t.Errorf("concurrent score failed: %v", err)
				return
			}
			if score == nil {
				t.Error("concurrent score returned nil")
			}
		}()
	}

	for i := 0; i < 5; i++ {
		<-done
	}
}

// --- Test helpers ---

func setupHealthTestProject(t *testing.T, withTests bool) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "health_test_project_*")
	if err != nil {
		t.Fatal(err)
	}

	// README
	writeHealthFile(t, filepath.Join(dir, "README.md"), "# Test Project\n\nA test project for health scoring.\n")

	// go.mod
	writeHealthFile(t, filepath.Join(dir, "go.mod"), `module example.com/testproject

go 1.21

require (
	github.com/pkg/errors v0.9.1
)
`)

	// Source files
	writeHealthFile(t, filepath.Join(dir, "main.go"), `package main

import "fmt"

// Run starts the application.
func Run() error {
	fmt.Println("running")
	return nil
}

// Helper performs a helper operation.
func Helper(x int) int {
	if x > 0 {
		return x * 2
	}
	return 0
}

func main() {
	if err := Run(); err != nil {
		fmt.Printf("error: %v\n", err)
	}
}
`)

	// Create a sub-package
	pkgDir := filepath.Join(dir, "pkg", "util")
	os.MkdirAll(pkgDir, 0o755)

	writeHealthFile(t, filepath.Join(pkgDir, "util.go"), `package util

import "fmt"

// Format formats a value with a prefix.
func Format(prefix string, val int) string {
	return fmt.Sprintf("%s: %d", prefix, val)
}

// Add returns the sum of two integers.
func Add(a, b int) int {
	return a + b
}
`)

	if withTests {
		writeHealthFile(t, filepath.Join(dir, "main_test.go"), `package main

import "testing"

func TestRun(t *testing.T) {
	err := Run()
	if err != nil {
		t.Fatal(err)
	}
}

func TestHelper(t *testing.T) {
	if Helper(5) != 10 {
		t.Error("expected 10")
	}
}
`)

		writeHealthFile(t, filepath.Join(pkgDir, "util_test.go"), `package util

import "testing"

func TestFormat(t *testing.T) {
	got := Format("x", 1)
	if got != "x: 1" {
		t.Errorf("got %q", got)
	}
}

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Error("expected 3")
	}
}
`)
	}

	return dir
}

func writeHealthFile(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
