package health

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/provider/gateway"
)

func TestNewDiagnostics(t *testing.T) {
	d := NewDiagnostics()
	if d == nil {
		t.Fatal("NewDiagnostics returned nil")
	}
	if len(d.Checks) == 0 {
		t.Fatal("NewDiagnostics should register built-in checks")
	}

	// Verify we have checks in each category
	categories := map[string]bool{}
	for _, c := range d.Checks {
		categories[c.Category] = true
	}

	expected := []string{"environment", "config", "network", "tools", "permissions"}
	for _, cat := range expected {
		if !categories[cat] {
			t.Errorf("Expected category %q not found in registered checks", cat)
		}
	}
}

func TestDiagnosticCheckFields(t *testing.T) {
	d := NewDiagnostics()
	for _, check := range d.Checks {
		if check.Name == "" {
			t.Error("Check has empty Name")
		}
		if check.Category == "" {
			t.Errorf("Check %q has empty Category", check.Name)
		}
		if check.RunFn == nil {
			t.Errorf("Check %q has nil RunFn", check.Name)
		}
	}
}

func TestRunAll(t *testing.T) {
	d := NewDiagnostics()
	suite := d.RunAll()

	if suite == nil {
		t.Fatal("RunAll returned nil suite")
	}
	if suite.StartTime.IsZero() {
		t.Error("Suite StartTime should not be zero")
	}
	if suite.Duration == 0 {
		t.Error("Suite Duration should not be zero")
	}
	if len(suite.Results) == 0 {
		t.Error("Suite should have results")
	}
	if len(suite.Results) != len(d.Checks) {
		t.Errorf("Expected %d results, got %d", len(d.Checks), len(suite.Results))
	}

	// Verify all results have valid status
	for _, r := range suite.Results {
		if r.Status != "pass" && r.Status != "warn" && r.Status != "fail" {
			t.Errorf("Result %q has invalid status: %q", r.Name, r.Status)
		}
		if r.Name == "" {
			t.Error("Result has empty Name")
		}
	}
}

func TestRunCategory(t *testing.T) {
	d := NewDiagnostics()

	suite := d.RunCategory("environment")
	if suite == nil {
		t.Fatal("RunCategory returned nil suite")
	}
	if len(suite.Results) == 0 {
		t.Error("Expected results for 'environment' category")
	}

	// Verify all results are from the environment checks
	envChecks := 0
	for _, check := range d.Checks {
		if check.Category == "environment" {
			envChecks++
		}
	}
	if len(suite.Results) != envChecks {
		t.Errorf("Expected %d environment results, got %d", envChecks, len(suite.Results))
	}
}

func TestRunCategoryEmpty(t *testing.T) {
	d := NewDiagnostics()
	suite := d.RunCategory("nonexistent")
	if suite == nil {
		t.Fatal("RunCategory returned nil for nonexistent category")
	}
	if len(suite.Results) != 0 {
		t.Errorf("Expected 0 results for nonexistent category, got %d", len(suite.Results))
	}
}

func TestFormatResults(t *testing.T) {
	suite := &DiagnosticSuite{
		StartTime: time.Now(),
		Duration:  100 * time.Millisecond,
		Results: []DiagnosticResult{
			{
				Name:     "test_pass",
				Status:   "pass",
				Message:  "All good",
				Duration: 10 * time.Millisecond,
			},
			{
				Name:     "test_warn",
				Status:   "warn",
				Message:  "Minor issue",
				Fix:      "Do something",
				Duration: 20 * time.Millisecond,
			},
			{
				Name:     "test_fail",
				Status:   "fail",
				Message:  "Critical failure",
				Fix:      "Fix it now",
				Duration: 30 * time.Millisecond,
			},
		},
	}

	output := FormatResults(suite)

	if !strings.Contains(output, "Graycode Diagnostics") {
		t.Error("Output should contain header")
	}
	if !strings.Contains(output, "test_pass") {
		t.Error("Output should contain test_pass")
	}
	if !strings.Contains(output, "test_warn") {
		t.Error("Output should contain test_warn")
	}
	if !strings.Contains(output, "test_fail") {
		t.Error("Output should contain test_fail")
	}
	if !strings.Contains(output, "Fix it now") {
		t.Error("Output should show fix for failed check")
	}
	if !strings.Contains(output, "1 passed") {
		t.Error("Output should show pass count")
	}
	if !strings.Contains(output, "1 warnings") {
		t.Error("Output should show warn count")
	}
	if !strings.Contains(output, "1 failed") {
		t.Error("Output should show fail count")
	}
}

func TestFormatResultsNil(t *testing.T) {
	output := FormatResults(nil)
	if !strings.Contains(output, "No diagnostic results") {
		t.Error("Nil suite should produce 'no results' message")
	}
}

func TestFormatResultsEmpty(t *testing.T) {
	suite := &DiagnosticSuite{}
	output := FormatResults(suite)
	if !strings.Contains(output, "No diagnostic results") {
		t.Error("Empty suite should produce 'no results' message")
	}
}

func TestQuickCheck(t *testing.T) {
	// Create a diagnostics with only checks that should pass
	d := &Diagnostics{}
	d.Checks = append(d.Checks, DiagnosticCheck{
		Name:     "always_pass",
		Category: "test",
		RunFn: func() DiagnosticResult {
			return DiagnosticResult{
				Name:     "always_pass",
				Status:   "pass",
				Message:  "ok",
				Duration: time.Millisecond,
			}
		},
	})

	if !d.QuickCheck() {
		t.Error("QuickCheck should return true when all checks pass")
	}
}

func TestQuickCheckFails(t *testing.T) {
	d := &Diagnostics{}
	d.Checks = append(d.Checks, DiagnosticCheck{
		Name:     "always_fail",
		Category: "test",
		RunFn: func() DiagnosticResult {
			return DiagnosticResult{
				Name:     "always_fail",
				Status:   "fail",
				Message:  "broken",
				Fix:      "fix it",
				Duration: time.Millisecond,
			}
		},
	})

	if d.QuickCheck() {
		t.Error("QuickCheck should return false when a check fails")
	}
}

func TestQuickCheckWarningsPasses(t *testing.T) {
	d := &Diagnostics{}
	d.Checks = append(d.Checks, DiagnosticCheck{
		Name:     "warning_only",
		Category: "test",
		RunFn: func() DiagnosticResult {
			return DiagnosticResult{
				Name:     "warning_only",
				Status:   "warn",
				Message:  "not ideal",
				Fix:      "improve it",
				Duration: time.Millisecond,
			}
		},
	})

	if !d.QuickCheck() {
		t.Error("QuickCheck should return true when only warnings exist (no failures)")
	}
}

func TestSuggestFixes(t *testing.T) {
	suite := &DiagnosticSuite{
		Results: []DiagnosticResult{
			{Name: "ok", Status: "pass", Message: "fine"},
			{Name: "bad", Status: "fail", Message: "broken", Fix: "fix bad"},
			{Name: "meh", Status: "warn", Message: "sorta", Fix: "fix meh"},
			{Name: "worse", Status: "fail", Message: "very broken", Fix: "fix worse"},
		},
	}

	fixes := SuggestFixes(suite)
	if len(fixes) != 3 {
		t.Fatalf("Expected 3 fixes, got %d", len(fixes))
	}

	// Failures should come first
	if !strings.Contains(fixes[0], "[FAIL]") {
		t.Error("First fix should be a FAIL")
	}
	if !strings.Contains(fixes[1], "[FAIL]") {
		t.Error("Second fix should be a FAIL")
	}
	if !strings.Contains(fixes[2], "[WARN]") {
		t.Error("Third fix should be a WARN")
	}

	if !strings.Contains(fixes[0], "fix bad") {
		t.Errorf("Expected 'fix bad' in first fix, got: %s", fixes[0])
	}
}

func TestSuggestFixesNil(t *testing.T) {
	fixes := SuggestFixes(nil)
	if fixes != nil {
		t.Errorf("Expected nil fixes for nil suite, got %v", fixes)
	}
}

func TestSuggestFixesNoFixes(t *testing.T) {
	suite := &DiagnosticSuite{
		Results: []DiagnosticResult{
			{Name: "ok", Status: "pass", Message: "fine"},
		},
	}
	fixes := SuggestFixes(suite)
	if len(fixes) != 0 {
		t.Errorf("Expected 0 fixes for all-pass suite, got %d", len(fixes))
	}
}

func TestCheckGoVersion(t *testing.T) {
	result := checkGoVersion()
	if result.Name != "go_version" {
		t.Errorf("Expected name 'go_version', got %q", result.Name)
	}
	if result.Status != "pass" {
		t.Errorf("Expected status 'pass', got %q", result.Status)
	}
	if !strings.Contains(result.Message, "Go version") {
		t.Errorf("Expected message to contain 'Go version', got %q", result.Message)
	}
}

func TestCheckShellAvailable(t *testing.T) {
	result := checkShellAvailable()
	if result.Name != "shell_available" {
		t.Errorf("Expected name 'shell_available', got %q", result.Name)
	}
	// On any dev machine a shell should be available
	if result.Status == "fail" {
		t.Log("Warning: shell check failed, likely running in unusual environment")
	}
}

func TestCheckDiskSpace(t *testing.T) {
	result := checkDiskSpace()
	if result.Name != "disk_space" {
		t.Errorf("Expected name 'disk_space', got %q", result.Name)
	}
	if result.Status != "pass" {
		t.Errorf("Expected disk space check to pass, got status %q", result.Status)
	}
}

func TestCheckTempDirWritable(t *testing.T) {
	result := checkTempDirWritable()
	if result.Name != "temp_dir_writable" {
		t.Errorf("Expected name 'temp_dir_writable', got %q", result.Name)
	}
	if result.Status != "pass" {
		t.Errorf("Expected temp dir to be writable, got status %q: %s", result.Status, result.Message)
	}
}

func TestCheckAPIKeySet(t *testing.T) {
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() { gateway.SetDefaultStore(nil) })

	result := checkAPIKeySet()
	if result.Status != "fail" {
		t.Errorf("Expected fail when no API keys stored, got %q", result.Status)
	}

	ctx := context.Background()
	_ = store.Set(ctx, gateway.AccountForEnv("ANTHROPIC_API_KEY"), "sk-ant-test-key-1234567890")
	result = checkAPIKeySet()
	if result.Status != "pass" {
		t.Errorf("Expected pass when key is in store, got %q: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "anthropic") {
		t.Errorf("Expected message to mention the configured gateway, got %q", result.Message)
	}
}

func TestCheckModelConfigured(t *testing.T) {
	orig := os.Getenv("GRAYCODE_MODEL")
	os.Unsetenv("GRAYCODE_MODEL")
	defer func() {
		if orig != "" {
			os.Setenv("GRAYCODE_MODEL", orig)
		}
	}()

	result := checkModelConfigured()
	if result.Status != "warn" {
		t.Errorf("Expected warn when GRAYCODE_MODEL not set, got %q", result.Status)
	}

	os.Setenv("GRAYCODE_MODEL", "claude-opus-4-20250514")
	result = checkModelConfigured()
	if result.Status != "pass" {
		t.Errorf("Expected pass when GRAYCODE_MODEL is set, got %q", result.Status)
	}
}

func TestCheckDNSWorks(t *testing.T) {
	result := checkDNSWorks()
	if result.Name != "dns_works" {
		t.Errorf("Expected name 'dns_works', got %q", result.Name)
	}
	// DNS should work in most environments
	if result.Status == "fail" {
		t.Log("DNS check failed - network may be unavailable in test environment")
	}
}

func TestCheckBinaryAvailableGit(t *testing.T) {
	result := checkGitBinary()
	if result.Name != "git_binary" {
		t.Errorf("Expected name 'git_binary', got %q", result.Name)
	}
	// git should be available in dev environments
	if result.Status != "pass" {
		t.Log("git_binary check did not pass - git may not be installed")
	}
}

func TestDiagnosticResultDuration(t *testing.T) {
	d := &Diagnostics{}
	d.Checks = append(d.Checks, DiagnosticCheck{
		Name:     "timed_check",
		Category: "test",
		RunFn: func() DiagnosticResult {
			start := time.Now()
			time.Sleep(5 * time.Millisecond)
			return DiagnosticResult{
				Name:     "timed_check",
				Status:   "pass",
				Message:  "took some time",
				Duration: time.Since(start),
			}
		},
	})

	suite := d.RunAll()
	if len(suite.Results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(suite.Results))
	}
	if suite.Results[0].Duration < 5*time.Millisecond {
		t.Errorf("Expected duration >= 5ms, got %v", suite.Results[0].Duration)
	}
}

func TestConcurrentRunAll(t *testing.T) {
	d := NewDiagnostics()

	// Run diagnostics concurrently to test mutex safety
	done := make(chan struct{}, 3)
	for i := 0; i < 3; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			suite := d.RunAll()
			if suite == nil {
				t.Error("RunAll returned nil in concurrent execution")
			}
		}()
	}

	for i := 0; i < 3; i++ {
		<-done
	}
}
