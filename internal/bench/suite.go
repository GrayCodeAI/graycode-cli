package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

// BenchmarkResult holds the result of a single benchmark run.
type BenchmarkResult struct {
	Name     string        `json:"name"`
	Duration time.Duration `json:"duration_ms"`
	Score    float64       `json:"score"`
	Metric   string        `json:"metric"`
	Passed   bool          `json:"passed"`
	Details  string        `json:"details,omitempty"`
}

// BenchmarkSuite runs benchmarks across the graycode-eco ecosystem.
type BenchmarkSuite struct {
	Results []BenchmarkResult `json:"results"`
}

// RunAll runs all available benchmarks and returns the results.
func RunAll(projectDir string) (*BenchmarkSuite, error) {
	suite := &BenchmarkSuite{}

	// Run harrier benchmarks
	if harrierResults, err := runHarrierBench(projectDir); err == nil {
		suite.Results = append(suite.Results, harrierResults...)
	}

	// Run shrike benchmarks
	if shrikeResults, err := runShrikeBench(projectDir); err == nil {
		suite.Results = append(suite.Results, shrikeResults...)
	}

	// Run graycode build benchmark
	if graycodeResult, err := runGraycodeBuildBench(projectDir); err == nil {
		suite.Results = append(suite.Results, graycodeResult)
	}

	return suite, nil
}

// runHarrierBench runs harrier's built-in benchmark suite.
func runHarrierBench(projectDir string) ([]BenchmarkResult, error) {
	harrierDir := filepath.Join(filepath.Dir(projectDir), "harrier")
	if _, err := os.Stat(filepath.Join(harrierDir, "go.mod")); err != nil {
		return nil, fmt.Errorf("harrier not found")
	}

	start := time.Now()
	cmd := exec.CommandContext(context.Background(), "go", "test", "-bench=.", "-benchmem", "-count=1", "-timeout=60s", "./engine/...")
	cmd.Dir = harrierDir
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	result := BenchmarkResult{
		Name:     "harrier/engine",
		Duration: duration,
		Metric:   "time",
		Passed:   err == nil,
		Details:  string(output),
	}
	if err == nil {
		result.Score = float64(duration.Milliseconds())
	}

	return []BenchmarkResult{result}, nil
}

// runShrikeBench runs shrike's benchmark suite.
func runShrikeBench(projectDir string) ([]BenchmarkResult, error) {
	shrikeDir := filepath.Join(filepath.Dir(projectDir), "shrike")
	if _, err := os.Stat(filepath.Join(shrikeDir, "go.mod")); err != nil {
		return nil, fmt.Errorf("shrike not found")
	}

	start := time.Now()
	cmd := exec.CommandContext(context.Background(), "go", "test", "-bench=.", "-benchmem", "-count=1", "-timeout=60s", "./...")
	cmd.Dir = shrikeDir
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	result := BenchmarkResult{
		Name:     "shrike",
		Duration: duration,
		Metric:   "time",
		Passed:   err == nil,
		Details:  string(output),
	}
	if err == nil {
		result.Score = float64(duration.Milliseconds())
	}

	return []BenchmarkResult{result}, nil
}

// runGraycodeBuildBench measures graycode build time.
func runGraycodeBuildBench(projectDir string) (BenchmarkResult, error) {
	graycodeDir := filepath.Join(projectDir)
	if _, err := os.Stat(filepath.Join(graycodeDir, "go.mod")); err != nil {
		return BenchmarkResult{}, fmt.Errorf("graycode not found")
	}

	start := time.Now()
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", "/dev/null", ".")
	cmd.Dir = graycodeDir
	err := cmd.Run()
	duration := time.Since(start)

	return BenchmarkResult{
		Name:     "graycode/build",
		Duration: duration,
		Score:    float64(duration.Milliseconds()),
		Metric:   "time",
		Passed:   err == nil,
	}, nil
}

// FormatReport returns a human-readable benchmark report.
func (s *BenchmarkSuite) FormatReport() string {
	var report string
	report += "## graycode-eco Benchmark Report\n\n"
	report += fmt.Sprintf("Generated: %s\n\n", time.Now().Format(time.RFC3339))
	report += "| Benchmark | Duration | Score | Status |\n"
	report += "|-----------|----------|-------|--------|\n"

	for _, r := range s.Results {
		status := icons.CheckBold()
		if !r.Passed {
			status = icons.CloseThick()
		}
		report += fmt.Sprintf("| %s | %s | %.0fms | %s |\n",
			r.Name, r.Duration.Round(time.Millisecond), r.Score, status)
	}

	return report
}

// SaveJSON saves the benchmark results to a JSON file.
func (s *BenchmarkSuite) SaveJSON(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
