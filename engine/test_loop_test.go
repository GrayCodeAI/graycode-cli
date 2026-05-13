package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewTestLoopDefaults(t *testing.T) {
	tl := NewTestLoop()

	if !tl.Enabled {
		t.Error("expected Enabled = true by default")
	}
	if tl.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", tl.MaxRetries)
	}
	if tl.Timeout != 120*time.Second {
		t.Errorf("Timeout = %v, want 120s", tl.Timeout)
	}
	if tl.Commands == nil {
		t.Fatal("Commands should not be nil")
	}
	if tl.retryCount == nil {
		t.Fatal("retryCount should not be nil")
	}
}

func TestDefaultTestCommands(t *testing.T) {
	cmds := DefaultTestCommands()

	expected := []string{"go", "python", "javascript", "typescript", "rust", "ruby", "java-gradle", "java-maven"}
	for _, key := range expected {
		if _, ok := cmds[key]; !ok {
			t.Errorf("missing default test command for %q", key)
		}
	}

	if cmds["go"] != "go test ./..." {
		t.Errorf("go command = %q, want %q", cmds["go"], "go test ./...")
	}
	if cmds["python"] != "pytest" {
		t.Errorf("python command = %q, want %q", cmds["python"], "pytest")
	}
	if cmds["rust"] != "cargo test" {
		t.Errorf("rust command = %q, want %q", cmds["rust"], "cargo test")
	}
	if cmds["ruby"] != "bundle exec rspec" {
		t.Errorf("ruby command = %q, want %q", cmds["ruby"], "bundle exec rspec")
	}
}

func TestDetectTestCommandGo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)

	cmd := DetectTestCommand(dir)
	if cmd != "go test -count=1 ./..." {
		t.Errorf("DetectTestCommand(go) = %q, want %q", cmd, "go test -count=1 ./...")
	}
}

func TestDetectTestCommandPython(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pytest.ini"), []byte("[pytest]\n"), 0644)

	cmd := DetectTestCommand(dir)
	if cmd != "pytest -x" {
		t.Errorf("DetectTestCommand(python/pytest.ini) = %q, want %q", cmd, "pytest -x")
	}

	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir2, "setup.py"), []byte("from setuptools import setup\n"), 0644)

	cmd2 := DetectTestCommand(dir2)
	if cmd2 != "pytest -x" {
		t.Errorf("DetectTestCommand(python/setup.py) = %q, want %q", cmd2, "pytest -x")
	}
}

func TestDetectTestCommandJavaScript(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := `{"name": "test", "scripts": {"test": "jest"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644)

	cmd := DetectTestCommand(dir)
	if cmd != "npm test" {
		t.Errorf("DetectTestCommand(js) = %q, want %q", cmd, "npm test")
	}
}

func TestDetectTestCommandJavaScriptNoScript(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := `{"name": "test"}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644)

	cmd := DetectTestCommand(dir)
	if cmd != "npm test" {
		t.Errorf("DetectTestCommand(js no scripts) = %q, want %q", cmd, "npm test")
	}
}

func TestDetectTestCommandRust(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"test\"\n"), 0644)

	cmd := DetectTestCommand(dir)
	if cmd != "cargo test" {
		t.Errorf("DetectTestCommand(rust) = %q, want %q", cmd, "cargo test")
	}
}

func TestDetectTestCommandRuby(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("source 'https://rubygems.org'\n"), 0644)

	cmd := DetectTestCommand(dir)
	if cmd != "bundle exec rspec" {
		t.Errorf("DetectTestCommand(ruby) = %q, want %q", cmd, "bundle exec rspec")
	}
}

func TestDetectTestCommandGradle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "build.gradle"), []byte("apply plugin: 'java'\n"), 0644)

	cmd := DetectTestCommand(dir)
	if cmd != "./gradlew test" {
		t.Errorf("DetectTestCommand(gradle) = %q, want %q", cmd, "./gradlew test")
	}
}

func TestDetectTestCommandMaven(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project></project>\n"), 0644)

	cmd := DetectTestCommand(dir)
	if cmd != "mvn test -q" {
		t.Errorf("DetectTestCommand(maven) = %q, want %q", cmd, "mvn test -q")
	}
}

func TestDetectTestCommandUnknown(t *testing.T) {
	dir := t.TempDir()

	cmd := DetectTestCommand(dir)
	if cmd != "" {
		t.Errorf("DetectTestCommand(empty) = %q, want empty string", cmd)
	}
}

func TestParseFailedTestsGo(t *testing.T) {
	output := `=== RUN   TestAdd
--- PASS: TestAdd (0.00s)
=== RUN   TestSubtract
--- FAIL: TestSubtract (0.01s)
    math_test.go:15: expected 5, got 3
=== RUN   TestMultiply
--- FAIL: TestMultiply (0.00s)
    math_test.go:22: expected 12, got 10
FAIL
exit status 1
FAIL    example.com/math    0.015s`

	tests := ParseFailedTests(output, "go")
	if len(tests) != 2 {
		t.Fatalf("ParseFailedTests(go) returned %d tests, want 2: %v", len(tests), tests)
	}
	if tests[0] != "TestSubtract" {
		t.Errorf("tests[0] = %q, want %q", tests[0], "TestSubtract")
	}
	if tests[1] != "TestMultiply" {
		t.Errorf("tests[1] = %q, want %q", tests[1], "TestMultiply")
	}
}

func TestParseFailedTestsGoSubtests(t *testing.T) {
	output := `--- FAIL: TestHandler/POST_invalid_json (0.00s)
--- FAIL: TestHandler/GET_missing_id (0.01s)`

	tests := ParseFailedTests(output, "go")
	if len(tests) != 2 {
		t.Fatalf("ParseFailedTests(go subtests) returned %d tests, want 2: %v", len(tests), tests)
	}
	if tests[0] != "TestHandler/POST_invalid_json" {
		t.Errorf("tests[0] = %q, want %q", tests[0], "TestHandler/POST_invalid_json")
	}
}

func TestParseFailedTestsPython(t *testing.T) {
	output := `============================= test session starts ==============================
collected 5 items

tests/test_math.py::test_add PASSED
tests/test_math.py::test_subtract FAILED
tests/test_math.py::test_multiply FAILED

=========================== short test summary info ============================
FAILED tests/test_math.py::test_subtract - AssertionError: assert 3 == 5
FAILED tests/test_math.py::test_multiply - AssertionError: assert 10 == 12
============================= 2 failed, 1 passed ==============================`

	tests := ParseFailedTests(output, "python")
	if len(tests) != 2 {
		t.Fatalf("ParseFailedTests(python) returned %d tests, want 2: %v", len(tests), tests)
	}
	if !strings.Contains(tests[0], "test_subtract") {
		t.Errorf("tests[0] = %q, expected it to contain test_subtract", tests[0])
	}
	if !strings.Contains(tests[1], "test_multiply") {
		t.Errorf("tests[1] = %q, expected it to contain test_multiply", tests[1])
	}
}

func TestParseFailedTestsJS(t *testing.T) {
	output := `  Math operations
    ✓ should add numbers
    ✗ should subtract numbers
      Expected 5 but got 3
    ✗ should multiply numbers
      Expected 12 but got 10

  2 failing`

	tests := ParseFailedTests(output, "javascript")
	if len(tests) != 2 {
		t.Fatalf("ParseFailedTests(js) returned %d tests, want 2: %v", len(tests), tests)
	}
	if !strings.Contains(tests[0], "subtract") {
		t.Errorf("tests[0] = %q, expected it to contain 'subtract'", tests[0])
	}
	if !strings.Contains(tests[1], "multiply") {
		t.Errorf("tests[1] = %q, expected it to contain 'multiply'", tests[1])
	}
}

func TestParseFailedTestsJSWithFAIL(t *testing.T) {
	output := `FAIL src/math.test.js
  ● Math > subtract
  FAIL src/utils.test.js
  ● Utils > format`

	tests := ParseFailedTests(output, "javascript")
	if len(tests) < 2 {
		t.Fatalf("ParseFailedTests(js FAIL) returned %d tests, want at least 2: %v", len(tests), tests)
	}
}

func TestParseFailedTestsRust(t *testing.T) {
	output := `running 4 tests
test math::test_add ... ok
test math::test_subtract ... FAILED
test math::test_multiply ... FAILED
test math::test_divide ... ok

failures:

---- math::test_subtract stdout ----
thread 'math::test_subtract' panicked at 'assertion failed'

test result: FAILED. 2 passed; 2 failed; 0 ignored`

	tests := ParseFailedTests(output, "rust")
	if len(tests) != 2 {
		t.Fatalf("ParseFailedTests(rust) returned %d tests, want 2: %v", len(tests), tests)
	}
	if tests[0] != "math::test_subtract" {
		t.Errorf("tests[0] = %q, want %q", tests[0], "math::test_subtract")
	}
	if tests[1] != "math::test_multiply" {
		t.Errorf("tests[1] = %q, want %q", tests[1], "math::test_multiply")
	}
}

func TestParseFailedTestsGeneric(t *testing.T) {
	output := `Running tests...
test_add: OK
test_subtract: FAIL
test_multiply: ERROR
test_divide: OK`

	tests := ParseFailedTests(output, "unknown")
	if len(tests) < 2 {
		t.Fatalf("ParseFailedTests(generic) returned %d tests, want at least 2: %v", len(tests), tests)
	}

	// Should contain lines with FAIL or ERROR
	found := false
	for _, test := range tests {
		if strings.Contains(test, "FAIL") || strings.Contains(test, "ERROR") {
			found = true
		}
	}
	if !found {
		t.Error("expected generic parser to find lines with FAIL or ERROR")
	}
}

func TestFilterRelevantOutputTruncation(t *testing.T) {
	// Build output longer than maxChars
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "error: something failed on line "+strings.Repeat("x", 50))
	}
	output := strings.Join(lines, "\n")

	filtered := FilterRelevantOutput(output, 500)
	if len(filtered) > 520 { // allow small overflow for truncation message
		t.Errorf("FilterRelevantOutput exceeded maxChars: len=%d", len(filtered))
	}
	if !strings.Contains(filtered, "... (truncated)") {
		t.Error("expected truncation indicator in output")
	}
}

func TestFilterRelevantOutputNoiseRemoval(t *testing.T) {
	output := `ok  	example.com/pkg	0.015s
PASS
--- FAIL: TestSomething (0.01s)
    file_test.go:10: expected 5, got 3
Compiling something
✓ test passed
error: cannot find module`

	filtered := FilterRelevantOutput(output, 5000)

	// Should keep failure lines
	if !strings.Contains(filtered, "FAIL: TestSomething") {
		t.Error("should keep FAIL lines")
	}
	if !strings.Contains(filtered, "expected 5, got 3") {
		t.Error("should keep assertion error lines")
	}
	if !strings.Contains(filtered, "cannot find module") {
		t.Error("should keep error lines")
	}

	// Should remove noise
	if strings.Contains(filtered, "ok  \t") {
		t.Error("should filter passing package lines")
	}
	if strings.Contains(filtered, "PASS") {
		t.Error("should filter PASS lines")
	}
	if strings.Contains(filtered, "Compiling") {
		t.Error("should filter Compiling lines")
	}
}

func TestFilterRelevantOutputEmpty(t *testing.T) {
	filtered := FilterRelevantOutput("", 1000)
	if filtered != "" {
		t.Errorf("expected empty string for empty input, got %q", filtered)
	}
}

func TestBuildTestFeedback(t *testing.T) {
	t.Run("nil result", func(t *testing.T) {
		msg := BuildTestFeedback(nil, []string{"main.go"})
		if msg != "" {
			t.Errorf("expected empty for nil result, got %q", msg)
		}
	})

	t.Run("passing result", func(t *testing.T) {
		result := &TestResult{Passed: true}
		msg := BuildTestFeedback(result, []string{"main.go"})
		if msg != "" {
			t.Errorf("expected empty for passing result, got %q", msg)
		}
	})

	t.Run("failing result", func(t *testing.T) {
		result := &TestResult{
			Passed:      false,
			Output:      "--- FAIL: TestAdd (0.00s)\n    math_test.go:10: expected 5, got 3\nFAIL",
			FailedTests: []string{"TestAdd"},
			ExitCode:    1,
		}
		msg := BuildTestFeedback(result, []string{"math.go", "math_test.go"})

		if !strings.Contains(msg, "math.go, math_test.go") {
			t.Error("should contain edited file names")
		}
		if !strings.Contains(msg, "Tests failed after your edits") {
			t.Error("should contain failure header")
		}
		if !strings.Contains(msg, "Failed tests: TestAdd") {
			t.Error("should list failed test names")
		}
		if !strings.Contains(msg, "Please fix the failing tests") {
			t.Error("should ask agent to fix")
		}
	})

	t.Run("multiple files and tests", func(t *testing.T) {
		result := &TestResult{
			Passed:      false,
			Output:      "--- FAIL: TestA (0.00s)\n--- FAIL: TestB (0.00s)\nFAIL",
			FailedTests: []string{"TestA", "TestB"},
			ExitCode:    1,
		}
		msg := BuildTestFeedback(result, []string{"a.go", "b.go", "c.go"})

		if !strings.Contains(msg, "a.go, b.go, c.go") {
			t.Error("should contain all edited file names")
		}
		if !strings.Contains(msg, "TestA, TestB") {
			t.Error("should list all failed tests")
		}
	})
}

func TestShouldRetryRecordRetryResetFile(t *testing.T) {
	tl := NewTestLoop()
	tl.MaxRetries = 3

	file := "/tmp/test.go"

	// Should allow retries initially
	if !tl.ShouldRetry(file) {
		t.Error("ShouldRetry should be true initially")
	}

	// Record retries up to max
	for i := 0; i < 3; i++ {
		if !tl.ShouldRetry(file) {
			t.Errorf("ShouldRetry should be true at retry %d", i)
		}
		tl.RecordRetry(file)
	}

	// Should now be exhausted
	if tl.ShouldRetry(file) {
		t.Error("ShouldRetry should be false after MaxRetries")
	}

	// Reset should restore ability to retry
	tl.ResetFile(file)
	if !tl.ShouldRetry(file) {
		t.Error("ShouldRetry should be true after ResetFile")
	}
}

func TestRetryCountTracking(t *testing.T) {
	tl := NewTestLoop()

	tl.RecordRetry("/a.go")
	tl.RecordRetry("/a.go")
	tl.RecordRetry("/b.go")

	if got := tl.RetryCount("/a.go"); got != 2 {
		t.Errorf("RetryCount(/a.go) = %d, want 2", got)
	}
	if got := tl.RetryCount("/b.go"); got != 1 {
		t.Errorf("RetryCount(/b.go) = %d, want 1", got)
	}
	if got := tl.RetryCount("/c.go"); got != 0 {
		t.Errorf("RetryCount(/c.go) = %d, want 0", got)
	}
}

func TestRetryIndependentPerFile(t *testing.T) {
	tl := NewTestLoop()
	tl.MaxRetries = 2

	// Exhaust retries for one file
	tl.RecordRetry("/a.go")
	tl.RecordRetry("/a.go")

	// Another file should still be retryable
	if !tl.ShouldRetry("/b.go") {
		t.Error("different file should be independently retryable")
	}
	if tl.ShouldRetry("/a.go") {
		t.Error("/a.go should be exhausted")
	}
}

func TestTestResultStruct(t *testing.T) {
	result := &TestResult{
		Passed:      false,
		Output:      "test output here",
		FailedTests: []string{"TestA", "TestB"},
		Duration:    5 * time.Second,
		ExitCode:    1,
	}

	if result.Passed {
		t.Error("expected Passed = false")
	}
	if result.Output != "test output here" {
		t.Errorf("Output = %q", result.Output)
	}
	if len(result.FailedTests) != 2 {
		t.Errorf("FailedTests len = %d, want 2", len(result.FailedTests))
	}
	if result.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want 5s", result.Duration)
	}
	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
}

func TestRunTestsDisabled(t *testing.T) {
	tl := NewTestLoop()
	tl.Enabled = false

	result, err := tl.RunTests(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result when disabled")
	}
}

func TestRunTestsNoProjectDetected(t *testing.T) {
	tl := NewTestLoop()
	dir := t.TempDir()

	result, err := tl.RunTests(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result when no project type detected")
	}
}

func TestRunTestsGoProject(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal Go project that passes
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc Add(a, b int) int { return a + b }\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(`package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Error("expected 5")
	}
}
`), 0644)

	tl := NewTestLoop()
	ctx := context.Background()

	result, err := tl.RunTests(ctx, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for Go project")
	}
	if !result.Passed {
		t.Errorf("expected tests to pass, output: %s", result.Output)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestRunTestsGoProjectFailing(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal Go project that fails
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc Add(a, b int) int { return a + b }\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(`package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 6 {
		t.Error("expected 6 but got 5")
	}
}
`), 0644)

	tl := NewTestLoop()
	ctx := context.Background()

	result, err := tl.RunTests(ctx, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Passed {
		t.Error("expected tests to fail")
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code")
	}
	if len(result.FailedTests) == 0 {
		t.Error("expected at least one failed test name")
	}
}

func TestDetectTestCommandPriority(t *testing.T) {
	// go.mod takes priority over package.json if both exist
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"test":"jest"}}`), 0644)

	cmd := DetectTestCommand(dir)
	if cmd != "go test -count=1 ./..." {
		t.Errorf("go.mod should take priority, got %q", cmd)
	}
}

func TestConcurrentRetryAccess(t *testing.T) {
	tl := NewTestLoop()
	tl.MaxRetries = 100

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			file := "/tmp/concurrent.go"
			for j := 0; j < 50; j++ {
				tl.RecordRetry(file)
				tl.ShouldRetry(file)
				tl.RetryCount(file)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// After 500 total records, count should be 500
	if got := tl.RetryCount("/tmp/concurrent.go"); got != 500 {
		t.Errorf("concurrent RetryCount = %d, want 500", got)
	}
}
