package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// BenchmarkResult holds the result of a single benchmark run.
type BenchmarkResult struct {
	Name      string        `json:"name"`
	Duration  time.Duration `json:"duration_ms"`
	Score     float64       `json:"score"`
	Metric    string        `json:"metric"`
	Passed    bool          `json:"passed"`
	Details   string        `json:"details,omitempty"`
}

// BenchmarkSuite runs benchmarks across the hawk-eco ecosystem.
type BenchmarkSuite struct {
	Results []BenchmarkResult `json:"results"`
}

// RunAll runs all available benchmarks and returns the results.
func RunAll(projectDir string) (*BenchmarkSuite, error) {
	suite := &BenchmarkSuite{}

	// Run yaad benchmarks
	if yaadResults, err := runYaadBench(projectDir); err == nil {
		suite.Results = append(suite.Results, yaadResults...)
	}

	// Run tok benchmarks
	if tokResults, err := runTokBench(projectDir); err == nil {
		suite.Results = append(suite.Results, tokResults...)
	}

	// Run hawk build benchmark
	if hawkResult, err := runHawkBuildBench(projectDir); err == nil {
		suite.Results = append(suite.Results, hawkResult)
	}

	return suite, nil
}

// runYaadBench runs yaad's built-in benchmark suite.
func runYaadBench(projectDir string) ([]BenchmarkResult, error) {
	yaadDir := filepath.Join(projectDir, "external", "yaad")
	if _, err := os.Stat(filepath.Join(yaadDir, "go.mod")); err != nil {
		return nil, fmt.Errorf("yaad not found")
	}

	start := time.Now()
	cmd := exec.Command("go", "test", "-bench=.", "-benchmem", "-count=1", "-timeout=60s", "./engine/...")
	cmd.Dir = yaadDir
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	result := BenchmarkResult{
		Name:     "yaad/engine",
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

// runTokBench runs tok's benchmark suite.
func runTokBench(projectDir string) ([]BenchmarkResult, error) {
	tokDir := filepath.Join(projectDir, "external", "tok")
	if _, err := os.Stat(filepath.Join(tokDir, "go.mod")); err != nil {
		return nil, fmt.Errorf("tok not found")
	}

	start := time.Now()
	cmd := exec.Command("go", "test", "-bench=.", "-benchmem", "-count=1", "-timeout=60s", "./...")
	cmd.Dir = tokDir
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	result := BenchmarkResult{
		Name:     "tok",
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

// runHawkBuildBench measures hawk build time.
func runHawkBuildBench(projectDir string) (BenchmarkResult, error) {
	hawkDir := filepath.Join(projectDir)
	if _, err := os.Stat(filepath.Join(hawkDir, "go.mod")); err != nil {
		return BenchmarkResult{}, fmt.Errorf("hawk not found")
	}

	start := time.Now()
	cmd := exec.Command("go", "build", "-o", "/dev/null", ".")
	cmd.Dir = hawkDir
	err := cmd.Run()
	duration := time.Since(start)

	return BenchmarkResult{
		Name:     "hawk/build",
		Duration: duration,
		Score:    float64(duration.Milliseconds()),
		Metric:   "time",
		Passed:   err == nil,
	}, nil
}

// FormatReport returns a human-readable benchmark report.
func (s *BenchmarkSuite) FormatReport() string {
	var report string
	report += "## Hawk-Eco Benchmark Report\n\n"
	report += fmt.Sprintf("Generated: %s\n\n", time.Now().Format(time.RFC3339))
	report += "| Benchmark | Duration | Score | Status |\n"
	report += "|-----------|----------|-------|--------|\n"

	for _, r := range s.Results {
		status := "✓"
		if !r.Passed {
			status = "✗"
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
	return os.WriteFile(path, data, 0644)
}
