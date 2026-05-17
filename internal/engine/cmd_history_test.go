package engine

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewCommandHistory(t *testing.T) {
	ch := NewCommandHistory()
	if ch == nil {
		t.Fatal("NewCommandHistory returned nil")
	}
	if len(ch.Commands) != 0 {
		t.Errorf("expected empty Commands, got %d", len(ch.Commands))
	}
	if ch.Patterns == nil {
		t.Error("Patterns map is nil")
	}
	if ch.Failures == nil {
		t.Error("Failures map is nil")
	}
}

func TestCmdHistoryRecord(t *testing.T) {
	ch := NewCommandHistory()

	ch.Record("go test ./...", 0, 3*time.Second, "PASS")
	ch.Record("go build .", 1, 2*time.Second, "error: something failed")

	if len(ch.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(ch.Commands))
	}

	// Check first record
	rec := ch.Commands[0]
	if rec.Command != "go test ./..." {
		t.Errorf("expected 'go test ./...', got %q", rec.Command)
	}
	if rec.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", rec.ExitCode)
	}
	if rec.Duration != 3*time.Second {
		t.Errorf("expected 3s duration, got %v", rec.Duration)
	}
	if rec.Output != "PASS" {
		t.Errorf("expected 'PASS' output, got %q", rec.Output)
	}

	// Check patterns tracking
	if ch.Patterns["go"] != 2 {
		t.Errorf("expected 'go' pattern count 2, got %d", ch.Patterns["go"])
	}

	// Check failures tracking
	if ch.Failures["go build ."] != 1 {
		t.Errorf("expected 1 failure for 'go build .', got %d", ch.Failures["go build ."])
	}
}

func TestRecordTruncatesOutput(t *testing.T) {
	ch := NewCommandHistory()

	longOutput := strings.Repeat("x", 600)
	ch.Record("cmd", 0, time.Second, longOutput)

	rec := ch.Commands[0]
	if len(rec.Output) > 520 {
		t.Errorf("output should be truncated, got length %d", len(rec.Output))
	}
	if !strings.HasSuffix(rec.Output, "... (truncated)") {
		t.Error("truncated output should end with '... (truncated)'")
	}
}

func TestGetFrequent(t *testing.T) {
	ch := NewCommandHistory()

	// Record commands with varying frequencies
	for i := 0; i < 10; i++ {
		ch.Record("git status", 0, 100*time.Millisecond, "")
	}
	for i := 0; i < 5; i++ {
		ch.Record("go test ./...", 0, 3*time.Second, "PASS")
	}
	for i := 0; i < 3; i++ {
		ch.Record("go build .", 0, 2*time.Second, "")
	}
	ch.Record("ls", 0, 50*time.Millisecond, "")

	// Get top 3
	freq := ch.GetFrequent(3)
	if len(freq) != 3 {
		t.Fatalf("expected 3 results, got %d", len(freq))
	}

	if freq[0].Command != "git status" {
		t.Errorf("expected 'git status' as most frequent, got %q", freq[0].Command)
	}
	if freq[0].Count != 10 {
		t.Errorf("expected count 10, got %d", freq[0].Count)
	}

	if freq[1].Command != "go test ./..." {
		t.Errorf("expected 'go test ./...' as second, got %q", freq[1].Command)
	}
	if freq[1].Count != 5 {
		t.Errorf("expected count 5, got %d", freq[1].Count)
	}

	// Check average duration
	if freq[0].AvgDuration != 100*time.Millisecond {
		t.Errorf("expected avg 100ms for git status, got %v", freq[0].AvgDuration)
	}
}

func TestGetFrequentWithFailRate(t *testing.T) {
	ch := NewCommandHistory()

	// 3 successes, 2 failures
	for i := 0; i < 3; i++ {
		ch.Record("npm test", 0, time.Second, "PASS")
	}
	for i := 0; i < 2; i++ {
		ch.Record("npm test", 1, time.Second, "FAIL")
	}

	freq := ch.GetFrequent(1)
	if len(freq) != 1 {
		t.Fatalf("expected 1 result, got %d", len(freq))
	}

	expectedRate := 0.4 // 2/5
	if freq[0].FailRate < 0.39 || freq[0].FailRate > 0.41 {
		t.Errorf("expected fail rate ~0.4, got %f", expectedRate)
	}
}

func TestGetFrequentWithZeroLimit(t *testing.T) {
	ch := NewCommandHistory()
	ch.Record("ls", 0, time.Millisecond, "")
	ch.Record("pwd", 0, time.Millisecond, "")

	freq := ch.GetFrequent(0)
	if len(freq) != 2 {
		t.Errorf("expected all 2 results with limit 0, got %d", len(freq))
	}
}

func TestGetFailing(t *testing.T) {
	ch := NewCommandHistory()

	ch.Record("go test ./...", 0, time.Second, "PASS")
	ch.Record("npm install", 1, 5*time.Second, "ERR! network")
	ch.Record("go build .", 0, 2*time.Second, "")
	ch.Record("npm test", 2, time.Second, "FAIL")

	failing := ch.GetFailing()
	if len(failing) != 2 {
		t.Fatalf("expected 2 failing commands, got %d", len(failing))
	}

	// Should be sorted by timestamp descending
	if failing[0].Command != "npm test" {
		t.Errorf("expected 'npm test' first (most recent), got %q", failing[0].Command)
	}
	if failing[1].Command != "npm install" {
		t.Errorf("expected 'npm install' second, got %q", failing[1].Command)
	}
}

func TestGetFailingEmpty(t *testing.T) {
	ch := NewCommandHistory()
	ch.Record("ls", 0, time.Millisecond, "")

	failing := ch.GetFailing()
	if len(failing) != 0 {
		t.Errorf("expected no failing commands, got %d", len(failing))
	}
}

func TestGetSlow(t *testing.T) {
	ch := NewCommandHistory()

	ch.Record("go test -race ./...", 0, 12*time.Second, "PASS")
	ch.Record("git status", 0, 100*time.Millisecond, "")
	ch.Record("go build .", 0, 8*time.Second, "")
	ch.Record("ls", 0, 50*time.Millisecond, "")

	slow := ch.GetSlow(5 * time.Second)
	if len(slow) != 2 {
		t.Fatalf("expected 2 slow commands, got %d", len(slow))
	}

	// Should be sorted by duration descending
	if slow[0].Command != "go test -race ./..." {
		t.Errorf("expected 'go test -race ./...' first, got %q", slow[0].Command)
	}
	if slow[0].Duration != 12*time.Second {
		t.Errorf("expected 12s, got %v", slow[0].Duration)
	}
	if slow[1].Command != "go build ." {
		t.Errorf("expected 'go build .' second, got %q", slow[1].Command)
	}
}

func TestGetSlowEmpty(t *testing.T) {
	ch := NewCommandHistory()
	ch.Record("ls", 0, time.Millisecond, "")

	slow := ch.GetSlow(time.Second)
	if len(slow) != 0 {
		t.Errorf("expected no slow commands, got %d", len(slow))
	}
}

func TestSuggestAlias(t *testing.T) {
	ch := NewCommandHistory()

	// Use a multi-word command 5 times
	for i := 0; i < 5; i++ {
		ch.Record("go test ./...", 0, 3*time.Second, "PASS")
	}
	// Single word command shouldn't get alias suggestion
	for i := 0; i < 5; i++ {
		ch.Record("ls", 0, time.Millisecond, "")
	}
	// Below threshold
	ch.Record("git diff --cached", 0, time.Second, "")

	suggestions := ch.SuggestAlias(3)

	// Should suggest alias for "go test ./..." but not "ls" or "git diff --cached"
	found := false
	for _, s := range suggestions {
		if s.Command == "go test ./..." {
			found = true
			if s.Count != 5 {
				t.Errorf("expected count 5, got %d", s.Count)
			}
			if s.Alias == "" {
				t.Error("alias should not be empty")
			}
			if s.Reason == "" {
				t.Error("reason should not be empty")
			}
		}
		if s.Command == "ls" {
			t.Error("single-word command 'ls' should not get alias suggestion")
		}
	}
	if !found {
		t.Error("expected alias suggestion for 'go test ./...'")
	}
}

func TestSuggestAliasEmpty(t *testing.T) {
	ch := NewCommandHistory()
	ch.Record("go test ./...", 0, time.Second, "")

	suggestions := ch.SuggestAlias(5)
	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions below threshold, got %d", len(suggestions))
	}
}

func TestDetectPatterns(t *testing.T) {
	ch := NewCommandHistory()

	// Create a sequential pattern: build then test
	for i := 0; i < 4; i++ {
		ch.Record("go build .", 0, 2*time.Second, "")
		ch.Record("go test ./...", 0, 3*time.Second, "PASS")
	}

	patterns := ch.DetectPatterns()
	if len(patterns) == 0 {
		t.Fatal("expected at least one pattern detected")
	}

	// Should detect the build-then-test pattern
	foundSequential := false
	for _, p := range patterns {
		if strings.Contains(p, "go test") && strings.Contains(p, "go build") && strings.Contains(p, "combining") {
			foundSequential = true
		}
	}
	if !foundSequential {
		t.Error("expected to detect go build -> go test sequential pattern")
	}
}

func TestDetectPatternsHighFailRate(t *testing.T) {
	ch := NewCommandHistory()

	// Command with high failure rate
	ch.Record("npm install", 1, 5*time.Second, "ERR")
	ch.Record("npm install", 1, 5*time.Second, "ERR")
	ch.Record("npm install", 0, 5*time.Second, "OK")

	patterns := ch.DetectPatterns()

	foundFail := false
	for _, p := range patterns {
		if strings.Contains(p, "npm install") && strings.Contains(p, "fails") {
			foundFail = true
		}
	}
	if !foundFail {
		t.Error("expected to detect high failure rate for npm install")
	}
}

func TestDetectPatternsMostCommon(t *testing.T) {
	ch := NewCommandHistory()

	for i := 0; i < 5; i++ {
		ch.Record("git status", 0, 100*time.Millisecond, "")
	}
	ch.Record("ls", 0, time.Millisecond, "")

	patterns := ch.DetectPatterns()

	foundMostCommon := false
	for _, p := range patterns {
		if strings.Contains(p, "git status") && strings.Contains(p, "most common") {
			foundMostCommon = true
		}
	}
	if !foundMostCommon {
		t.Error("expected to detect git status as most common command")
	}
}

func TestFormatSummary(t *testing.T) {
	ch := NewCommandHistory()

	// Empty history
	summary := ch.FormatSummary()
	if !strings.Contains(summary, "empty") {
		t.Error("expected empty summary message")
	}

	// Add some commands
	for i := 0; i < 12; i++ {
		ch.Record("go test ./...", 0, 3200*time.Millisecond, "PASS")
	}
	for i := 0; i < 8; i++ {
		ch.Record("git diff", 0, 100*time.Millisecond, "")
	}
	for i := 0; i < 3; i++ {
		ch.Record("npm test", 1, time.Second, "FAIL")
	}
	for i := 0; i < 2; i++ {
		ch.Record("npm test", 0, time.Second, "PASS")
	}
	ch.Record("go test -race ./...", 0, 12*time.Second, "PASS")

	summary = ch.FormatSummary()

	if !strings.Contains(summary, "Command History") {
		t.Error("summary should contain 'Command History' header")
	}
	if !strings.Contains(summary, "Most used:") {
		t.Error("summary should contain 'Most used:' section")
	}
	if !strings.Contains(summary, "go test ./...") {
		t.Error("summary should mention most used command")
	}
	if !strings.Contains(summary, "Failing:") {
		t.Error("summary should contain 'Failing:' section")
	}
	if !strings.Contains(summary, "npm test") {
		t.Error("summary should mention failing command")
	}
}

func TestFormatSummaryEmpty(t *testing.T) {
	ch := NewCommandHistory()
	summary := ch.FormatSummary()
	if !strings.Contains(summary, "empty") {
		t.Error("empty history should produce empty message")
	}
}

func TestSearchCommands(t *testing.T) {
	ch := NewCommandHistory()

	ch.Record("go test ./...", 0, time.Second, "PASS ok github.com/test")
	ch.Record("go build .", 0, time.Second, "")
	ch.Record("git status", 0, time.Millisecond, "nothing to commit")
	ch.Record("npm install", 1, 5*time.Second, "ERR! network timeout")

	// Search by command
	results := ch.SearchCommands("go")
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'go', got %d", len(results))
	}

	// Search by output
	results = ch.SearchCommands("network")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'network', got %d", len(results))
	}
	if results[0].Command != "npm install" {
		t.Errorf("expected 'npm install', got %q", results[0].Command)
	}

	// Case-insensitive
	results = ch.SearchCommands("PASS")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'PASS', got %d", len(results))
	}

	// No results
	results = ch.SearchCommands("python")
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'python', got %d", len(results))
	}
}

func TestSearchCommandsEmpty(t *testing.T) {
	ch := NewCommandHistory()
	results := ch.SearchCommands("anything")
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty history, got %d", len(results))
	}
}

func TestCmdHistoryClear(t *testing.T) {
	ch := NewCommandHistory()

	ch.Record("go test ./...", 0, time.Second, "PASS")
	ch.Record("go build .", 1, time.Second, "error")

	if len(ch.Commands) != 2 {
		t.Fatal("expected 2 commands before clear")
	}

	ch.Clear()

	if len(ch.Commands) != 0 {
		t.Errorf("expected 0 commands after clear, got %d", len(ch.Commands))
	}
	if len(ch.Patterns) != 0 {
		t.Errorf("expected 0 patterns after clear, got %d", len(ch.Patterns))
	}
	if len(ch.Failures) != 0 {
		t.Errorf("expected 0 failures after clear, got %d", len(ch.Failures))
	}
}

func TestCmdHistoryConcurrentAccess(t *testing.T) {
	ch := NewCommandHistory()

	var wg sync.WaitGroup
	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ch.Record("cmd", 0, time.Millisecond, "output")
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch.GetFrequent(5)
			ch.GetFailing()
			ch.GetSlow(time.Second)
			ch.SearchCommands("cmd")
			ch.DetectPatterns()
		}()
	}

	wg.Wait()

	if len(ch.Commands) != 50 {
		t.Errorf("expected 50 commands after concurrent writes, got %d", len(ch.Commands))
	}
}

func TestExtractBaseCommand(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"go test ./...", "go"},
		{"git status", "git"},
		{"ls", "ls"},
		{"npm install --save-dev", "npm"},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractBaseCommand(tt.input)
		if result != tt.expected {
			t.Errorf("extractBaseCommand(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestGenerateAlias(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"go test ./..."},
		{"git commit -m"},
		{"npm run build"},
	}

	for _, tt := range tests {
		result := generateAlias(tt.input)
		if result == "" {
			t.Errorf("generateAlias(%q) returned empty string", tt.input)
		}
		if len(result) >= len(tt.input) {
			t.Errorf("generateAlias(%q) = %q, should be shorter than input", tt.input, result)
		}
	}
}
