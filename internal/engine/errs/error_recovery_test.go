package errs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

func TestNewErrorRecovery(t *testing.T) {
	er := NewErrorRecovery()
	if er == nil {
		t.Fatal("NewErrorRecovery returned nil")
	}
	if er.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", er.MaxAttempts)
	}
	if len(er.Strategies) == 0 {
		t.Error("expected built-in strategies to be registered")
	}

	expectedStrategies := []string{
		"file_not_found", "permission_denied", "module_not_found",
		"port_in_use", "out_of_memory", "timeout", "rate_limited",
		"syntax_error", "import_cycle", "merge_conflict",
		"git_dirty", "build_failed", "test_failed",
	}
	for _, name := range expectedStrategies {
		if _, ok := er.Strategies[name]; !ok {
			t.Errorf("missing built-in strategy: %s", name)
		}
	}
}

func TestRecover_FileNotFound(t *testing.T) {
	er := NewErrorRecovery()

	err := errors.New("open /tmp/test_dir/atuh.go: no such file or directory")

	// Create a temp directory with a file to test Levenshtein matching.
	dir := t.TempDir()
	authFile := filepath.Join(dir, "auth.go")
	if err2 := os.WriteFile(authFile, []byte("package main"), 0o644); err2 != nil {
		t.Fatal(err2)
	}

	// Use an error referencing the temp dir with a typo.
	typoPath := filepath.Join(dir, "atuh.go")
	err = fmt.Errorf("open %s: no such file or directory", typoPath)

	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
		Attempt:  0,
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Recovered {
		t.Error("expected Recovered to be true")
	}
	if result.RetryWith == "" {
		t.Error("expected RetryWith to contain suggested path")
	}
	if result.RetryWith != authFile {
		t.Errorf("RetryWith = %q, want %q", result.RetryWith, authFile)
	}
}

func TestRecover_PermissionDenied(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("open /etc/shadow: permission denied")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected successful recovery")
	}
	if result.Action == "" {
		t.Error("expected non-empty Action")
	}
}

func TestRecover_ModuleNotFound(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("cannot find module providing package github.com/foo/bar")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected successful recovery")
	}
	if result.RetryWith != "go mod tidy" {
		t.Errorf("RetryWith = %q, want %q", result.RetryWith, "go mod tidy")
	}
}

func TestRecover_PortInUse(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("listen tcp :8080: bind: address already in use")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected successful recovery")
	}
	if result.Action == "" {
		t.Error("expected non-empty action with port info")
	}
}

func TestRecover_OutOfMemory(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("fatal: out of memory allocating 1048576 bytes")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected successful recovery")
	}
}

func TestRecover_Timeout(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("context deadline exceeded")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected successful recovery")
	}
}

func TestRecover_RateLimited(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("429 too many requests: rate limit exceeded")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected successful recovery")
	}
}

func TestRecover_SyntaxError(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("src/main.go:42: syntax error: unexpected token")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected successful recovery")
	}
}

func TestRecover_ImportCycle(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("import cycle not allowed: package a imports b imports a")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected successful recovery")
	}
}

func TestRecover_MergeConflict(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("CONFLICT (content): Merge conflict in src/main.go")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected successful recovery")
	}
}

func TestRecover_GitDirty(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("error: your local changes to the following files would be overwritten")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected successful recovery")
	}
	if result.RetryWith != "git stash" {
		t.Errorf("RetryWith = %q, want %q", result.RetryWith, "git stash")
	}
}

func TestRecover_BuildFailed(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("FAILED: build target //src:main")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected successful recovery")
	}
}

func TestRecover_TestFailed(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("--- FAIL  TestSomething (0.01s)")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover returned error: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected successful recovery")
	}
}

func TestRecover_NilError(t *testing.T) {
	er := NewErrorRecovery()
	result, recErr := er.Recover(nil, nil)
	if recErr != nil {
		t.Fatalf("unexpected error: %v", recErr)
	}
	if result != nil {
		t.Error("expected nil result for nil error")
	}
}

func TestRecover_UnknownError(t *testing.T) {
	er := NewErrorRecovery()
	err := errors.New("something completely unrecognized happened in the cosmic void")
	ctx := &RecoveryContext{
		Error:    err,
		ErrorMsg: err.Error(),
	}

	result, recErr := er.Recover(err, ctx)
	if recErr == nil {
		t.Error("expected error for unrecoverable error")
	}
	if result != nil {
		t.Error("expected nil result for unmatched error")
	}
}

func TestShouldRetryRecovery(t *testing.T) {
	er := NewErrorRecovery()

	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("no such file or directory"), true},
		{errors.New("permission denied"), true},
		{errors.New("context deadline exceeded"), true},
		{errors.New("429 too many requests"), true},
		{errors.New("completely unknown error xyz"), false},
		{nil, false},
	}

	for _, tt := range tests {
		got := er.ShouldRetry(tt.err)
		errStr := "<nil>"
		if tt.err != nil {
			errStr = tt.err.Error()
		}
		if got != tt.want {
			t.Errorf("ShouldRetry(%q) = %v, want %v", errStr, got, tt.want)
		}
	}
}

func TestBuildRecoveryPrompt(t *testing.T) {
	er := NewErrorRecovery()

	result := &RecoveryResult{
		Recovered: true,
		Action:    `Did you mean "src/auth.go"? (Levenshtein distance: 1)`,
		Message:   `file not found "src/atuh.go"`,
		RetryWith: "src/auth.go",
	}

	prompt := er.BuildRecoveryPrompt(result)
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !recoveryContainsAll(prompt, "Error encountered:", "Suggested recovery:", "Retry with:") {
		t.Errorf("prompt missing expected sections: %s", prompt)
	}
}

func TestBuildRecoveryPrompt_NilResult(t *testing.T) {
	er := NewErrorRecovery()
	prompt := er.BuildRecoveryPrompt(nil)
	if prompt != "" {
		t.Errorf("expected empty prompt for nil result, got %q", prompt)
	}
}

func TestFormatHistory(t *testing.T) {
	er := NewErrorRecovery()

	// Empty history.
	output := er.FormatHistory(5)
	if output != "No recovery attempts recorded." {
		t.Errorf("unexpected empty history output: %s", output)
	}

	// Add some attempts.
	er.History = append(er.History, RecoveryAttempt{
		Error:     "file not found",
		Strategy:  "file_not_found",
		Recovered: true,
		Duration:  5 * time.Millisecond,
		Timestamp: time.Now().Add(-2 * time.Minute),
	})
	er.History = append(er.History, RecoveryAttempt{
		Error:     "timeout",
		Strategy:  "timeout",
		Recovered: false,
		Duration:  100 * time.Millisecond,
		Timestamp: time.Now().Add(-1 * time.Minute),
	})
	er.History = append(er.History, RecoveryAttempt{
		Error:     "permission denied on /etc/shadow",
		Strategy:  "permission_denied",
		Recovered: true,
		Duration:  2 * time.Millisecond,
		Timestamp: time.Now(),
	})

	// Limit to 2.
	output = er.FormatHistory(2)
	if !recoveryContainsAll(output, "Recovery History:", "timeout", "permission_denied") {
		t.Errorf("limited history missing expected entries: %s", output)
	}

	// Limit of 0 returns all.
	output = er.FormatHistory(0)
	if !recoveryContainsAll(output, "file_not_found", "timeout", "permission_denied") {
		t.Errorf("unlimited history missing expected entries: %s", output)
	}
}

func TestSuccessRate(t *testing.T) {
	er := NewErrorRecovery()

	// No history.
	if rate := er.SuccessRate(); rate != 0.0 {
		t.Errorf("SuccessRate() = %f, want 0.0", rate)
	}

	// Add mixed results.
	er.History = []RecoveryAttempt{
		{Recovered: true},
		{Recovered: true},
		{Recovered: false},
		{Recovered: true},
	}

	rate := er.SuccessRate()
	expected := 0.75
	if rate != expected {
		t.Errorf("SuccessRate() = %f, want %f", rate, expected)
	}
}

func TestRegisterStrategy(t *testing.T) {
	er := NewErrorRecovery()
	initialCount := len(er.Strategies)

	custom := &RecoveryStrategy{
		Name:         "custom_error",
		ErrorPattern: regexp.MustCompile(`custom failure xyz`),
		Priority:     200,
		RecoverFn: func(err error, ctx *RecoveryContext) (*RecoveryResult, error) {
			return &RecoveryResult{
				Recovered: true,
				Action:    "Custom recovery executed",
				Message:   ctx.ErrorMsg,
			}, nil
		},
	}
	er.RegisterStrategy(custom)

	if len(er.Strategies) != initialCount+1 {
		t.Errorf("strategy count = %d, want %d", len(er.Strategies), initialCount+1)
	}

	// Test that the custom strategy works.
	err := errors.New("custom failure xyz occurred")
	ctx := &RecoveryContext{Error: err, ErrorMsg: err.Error()}
	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover with custom strategy failed: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("custom strategy should have recovered")
	}
	if result.Action != "Custom recovery executed" {
		t.Errorf("Action = %q, want %q", result.Action, "Custom recovery executed")
	}
}

func TestRegisterStrategy_Nil(t *testing.T) {
	er := NewErrorRecovery()
	count := len(er.Strategies)
	er.RegisterStrategy(nil)
	if len(er.Strategies) != count {
		t.Error("registering nil strategy should not change count")
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"atuh", "auth", 2},
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
	}

	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestExtractPort(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"listen tcp :8080: bind: address already in use", "8080"},
		{"listen tcp :443: bind: address already in use", "443"},
		{"no port here", ""},
	}

	for _, tt := range tests {
		got := extractPort(tt.msg)
		if got != tt.want {
			t.Errorf("extractPort(%q) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

func TestExtractLineNumber(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"main.go:42: syntax error", "42"},
		{"error on line 15", "15"},
		{"no line number here", ""},
	}

	for _, tt := range tests {
		got := extractLineNumber(tt.msg)
		if got != tt.want {
			t.Errorf("extractLineNumber(%q) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "he..."},
		{"hi", 2, "hi"},
		{"abcdefghij", 7, "abcd..."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.s, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
		}
	}
}

func TestRecoveryConcurrency(t *testing.T) {
	er := NewErrorRecovery()
	done := make(chan struct{})

	// Run concurrent recoveries.
	for i := 0; i < 20; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			err := fmt.Errorf("timeout error %d: context deadline exceeded", n)
			ctx := &RecoveryContext{Error: err, ErrorMsg: err.Error()}
			er.Recover(err, ctx)
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	if len(er.History) != 20 {
		t.Errorf("History length = %d, want 20", len(er.History))
	}
}

func TestPriorityOrdering(t *testing.T) {
	er := NewErrorRecovery()

	// An error that matches both "timeout" and "rate_limited" patterns.
	// The higher-priority strategy should be used first.
	err := errors.New("429 rate limit: request timed out")
	ctx := &RecoveryContext{Error: err, ErrorMsg: err.Error()}

	result, recErr := er.Recover(err, ctx)
	if recErr != nil {
		t.Fatalf("Recover failed: %v", recErr)
	}
	if result == nil || !result.Recovered {
		t.Fatal("expected recovery")
	}

	// The first attempt in history should be the highest-priority matching strategy.
	if len(er.History) == 0 {
		t.Fatal("expected history entries")
	}

	// Timeout has priority 75, rate_limited has priority 70.
	// So timeout should match first.
	first := er.History[0]
	if first.Strategy != "timeout" {
		t.Errorf("first strategy tried = %q, want %q (higher priority)", first.Strategy, "timeout")
	}
}

func TestHistoryRecordsAttempts(t *testing.T) {
	er := NewErrorRecovery()

	err := errors.New("open /nonexistent/path.go: no such file or directory")
	ctx := &RecoveryContext{Error: err, ErrorMsg: err.Error()}

	er.Recover(err, ctx)

	if len(er.History) == 0 {
		t.Fatal("expected at least one history entry")
	}

	entry := er.History[0]
	if entry.Strategy != "file_not_found" {
		t.Errorf("Strategy = %q, want %q", entry.Strategy, "file_not_found")
	}
	if entry.Error != err.Error() {
		t.Errorf("Error = %q, want %q", entry.Error, err.Error())
	}
	if entry.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

// recoveryContainsAll checks if s contains all the given substrings.
func recoveryContainsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !recoveryContains(s, sub) {
			return false
		}
	}
	return true
}

func recoveryContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
