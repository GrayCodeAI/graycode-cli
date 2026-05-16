package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewSelfHealer(t *testing.T) {
	chatFn := func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}
	sh := NewSelfHealer(chatFn)

	if sh.MaxAttempts != 5 {
		t.Errorf("expected MaxAttempts=5, got %d", sh.MaxAttempts)
	}
	if sh.Timeout != 60*time.Second {
		t.Errorf("expected Timeout=60s, got %v", sh.Timeout)
	}
	if sh.ChatFn == nil {
		t.Error("expected ChatFn to be set")
	}
	if sh.History == nil {
		t.Error("expected History to be initialized")
	}
}

func TestHeal_SuccessOnFirstAttempt(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ok.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	chatFn := func(ctx context.Context, prompt string) (string, error) {
		t.Error("ChatFn should not be called on success")
		return "", nil
	}
	sh := NewSelfHealer(chatFn)
	result, err := sh.Heal(context.Background(), script)
	if err != nil {
		t.Fatal(err)
	}
	if !result.FinalSuccess {
		t.Error("expected FinalSuccess=true")
	}
	if len(result.Attempts) != 1 {
		t.Errorf("expected 1 attempt, got %d", len(result.Attempts))
	}
	if result.FixesApplied != 0 {
		t.Errorf("expected 0 fixes, got %d", result.FixesApplied)
	}
}

func TestHeal_FixAfterFailure(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "broken.sh")
	// Script that fails initially
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	callCount := 0
	chatFn := func(ctx context.Context, prompt string) (string, error) {
		callCount++
		// Return a fix that makes the script succeed
		return fmt.Sprintf(`Here is the fix:

@@FIX
FILE: %s
LINE: 2
ACTION: replace
OLD_CONTENT:
exit 1
END_OLD
NEW_CONTENT:
exit 0
END_NEW
@@END
`, script), nil
	}

	sh := NewSelfHealer(chatFn)
	sh.MaxAttempts = 3
	result, err := sh.Heal(context.Background(), script)
	if err != nil {
		t.Fatal(err)
	}
	if !result.FinalSuccess {
		t.Error("expected FinalSuccess=true after fix")
	}
	if callCount != 1 {
		t.Errorf("expected ChatFn called once, got %d", callCount)
	}
	if result.FixesApplied != 1 {
		t.Errorf("expected 1 fix applied, got %d", result.FixesApplied)
	}
	if len(result.Attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", len(result.Attempts))
	}
}

func TestHeal_MaxAttemptsExhausted(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "unfixable.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Always return an unparseable fix
	chatFn := func(ctx context.Context, prompt string) (string, error) {
		return "I don't know how to fix this", nil
	}

	sh := NewSelfHealer(chatFn)
	sh.MaxAttempts = 3
	result, err := sh.Heal(context.Background(), script)
	if err != nil {
		t.Fatal(err)
	}
	if result.FinalSuccess {
		t.Error("expected FinalSuccess=false when unfixable")
	}
	if len(result.Attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", len(result.Attempts))
	}
}

func TestHeal_ContextCanceled(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "slow.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	chatFn := func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}
	sh := NewSelfHealer(chatFn)
	_, err := sh.Heal(ctx, script)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestHealCommand_Success(t *testing.T) {
	chatFn := func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}
	sh := NewSelfHealer(chatFn)
	result, err := sh.HealCommand(context.Background(), "true")
	if err != nil {
		t.Fatal(err)
	}
	if !result.FinalSuccess {
		t.Error("expected FinalSuccess=true for 'true' command")
	}
}

func TestHealCommand_FixFailingCommand(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "data.txt")

	// Command that will fail because file doesn't exist, then succeed after fix creates it
	command := fmt.Sprintf("cat %s", target)

	callCount := 0
	chatFn := func(ctx context.Context, prompt string) (string, error) {
		callCount++
		// Create the file as the "fix"
		os.WriteFile(target, []byte("hello\n"), 0o644)
		return "I cannot provide a structured fix for this", nil
	}

	sh := NewSelfHealer(chatFn)
	sh.MaxAttempts = 3
	result, err := sh.HealCommand(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	// The fix response is unparseable, so no patches are applied via ApplyFixes,
	// but the ChatFn side-effect created the file, so next run succeeds.
	if !result.FinalSuccess {
		t.Error("expected FinalSuccess=true after file creation")
	}
}

func TestBuildFixPrompt(t *testing.T) {
	chatFn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	sh := NewSelfHealer(chatFn)

	prompt := sh.BuildFixPrompt("print('hello')", "NameError: name 'x' is not defined", "NameError", 2)

	if !strings.Contains(prompt, "print('hello')") {
		t.Error("prompt should contain script content")
	}
	if !strings.Contains(prompt, "NameError") {
		t.Error("prompt should contain error message")
	}
	if !strings.Contains(prompt, "Attempt 2 of 5") {
		t.Error("prompt should contain attempt info")
	}
	if !strings.Contains(prompt, "@@FIX") {
		t.Error("prompt should contain fix format instructions")
	}
	if !strings.Contains(prompt, "ACTION:") {
		t.Error("prompt should describe ACTION field")
	}
}

func TestBuildFixPrompt_WithHistory(t *testing.T) {
	chatFn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	sh := NewSelfHealer(chatFn)
	sh.History = []HealAttempt{
		{Attempt: 1, Script: "test.py", ExitCode: 1, Error: "SyntaxError", Fix: "fixed indent"},
	}

	prompt := sh.BuildFixPrompt("test.py", "TypeError", "TypeError", 2)
	if !strings.Contains(prompt, "Previous Attempts") {
		t.Error("prompt should reference previous attempts when history exists")
	}
}

func TestParseFix_SingleFix(t *testing.T) {
	chatFn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	sh := NewSelfHealer(chatFn)

	response := `Here's the fix:

@@FIX
FILE: /tmp/test.py
LINE: 5
ACTION: replace
OLD_CONTENT:
print(x)
END_OLD
NEW_CONTENT:
x = 42
print(x)
END_NEW
@@END
`
	fixes, err := sh.ParseFix(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 1 {
		t.Fatalf("expected 1 fix, got %d", len(fixes))
	}
	if fixes[0].File != "/tmp/test.py" {
		t.Errorf("expected file=/tmp/test.py, got %s", fixes[0].File)
	}
	if fixes[0].Line != 5 {
		t.Errorf("expected line=5, got %d", fixes[0].Line)
	}
	if fixes[0].Action != "replace" {
		t.Errorf("expected action=replace, got %s", fixes[0].Action)
	}
	if fixes[0].OldContent != "print(x)" {
		t.Errorf("expected OldContent='print(x)', got '%s'", fixes[0].OldContent)
	}
	if fixes[0].NewContent != "x = 42\nprint(x)" {
		t.Errorf("expected NewContent='x = 42\\nprint(x)', got '%s'", fixes[0].NewContent)
	}
}

func TestParseFix_MultipleFixes(t *testing.T) {
	chatFn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	sh := NewSelfHealer(chatFn)

	response := `
@@FIX
FILE: a.py
LINE: 1
ACTION: insert
OLD_CONTENT:
END_OLD
NEW_CONTENT:
import os
END_NEW
@@END

@@FIX
FILE: b.py
LINE: 10
ACTION: delete
OLD_CONTENT:
unused_var = True
END_OLD
NEW_CONTENT:
END_NEW
@@END
`
	fixes, err := sh.ParseFix(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 2 {
		t.Fatalf("expected 2 fixes, got %d", len(fixes))
	}
	if fixes[0].Action != "insert" {
		t.Errorf("first fix action should be insert, got %s", fixes[0].Action)
	}
	if fixes[1].Action != "delete" {
		t.Errorf("second fix action should be delete, got %s", fixes[1].Action)
	}
}

func TestParseFix_InvalidResponse(t *testing.T) {
	chatFn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	sh := NewSelfHealer(chatFn)

	_, err := sh.ParseFix("just some random text without any fix blocks")
	if err == nil {
		t.Error("expected error for unparseable response")
	}
}

func TestParseFix_InvalidAction(t *testing.T) {
	chatFn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	sh := NewSelfHealer(chatFn)

	response := `@@FIX
FILE: test.py
LINE: 1
ACTION: rename
OLD_CONTENT:
x
END_OLD
NEW_CONTENT:
y
END_NEW
@@END`

	_, err := sh.ParseFix(response)
	if err == nil {
		t.Error("expected error for invalid action")
	}
}

func TestApplyFixes_Replace(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.py")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	chatFn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	sh := NewSelfHealer(chatFn)

	fixes := []FileFix{
		{File: file, Line: 2, Action: "replace", OldContent: "line2", NewContent: "fixed_line2"},
	}
	if err := sh.ApplyFixes(fixes); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "fixed_line2") {
		t.Errorf("expected replaced content, got: %s", string(data))
	}
	if strings.Contains(string(data), "line2\n") && !strings.Contains(string(data), "fixed_line2") {
		t.Error("old content should be replaced")
	}
}

func TestApplyFixes_Insert(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.py")
	content := "line1\nline2\nline3"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	chatFn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	sh := NewSelfHealer(chatFn)

	fixes := []FileFix{
		{File: file, Line: 2, Action: "insert", NewContent: "inserted"},
	}
	if err := sh.ApplyFixes(fixes); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(file)
	lines := strings.Split(string(data), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines, got %d: %v", len(lines), lines)
	}
	if lines[1] != "inserted" {
		t.Errorf("expected 'inserted' at line 2, got '%s'", lines[1])
	}
}

func TestApplyFixes_Delete(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.py")
	content := "line1\nline2\nline3"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	chatFn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	sh := NewSelfHealer(chatFn)

	fixes := []FileFix{
		{File: file, Line: 2, Action: "delete", OldContent: "line2"},
	}
	if err := sh.ApplyFixes(fixes); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(file)
	if strings.Contains(string(data), "line2") {
		t.Errorf("expected 'line2' to be deleted, got: %s", string(data))
	}
}

func TestApplyFixes_FileNotFound(t *testing.T) {
	chatFn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	sh := NewSelfHealer(chatFn)

	fixes := []FileFix{
		{File: "/nonexistent/path/file.py", Line: 1, Action: "replace", OldContent: "x", NewContent: "y"},
	}
	err := sh.ApplyFixes(fixes)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestRunScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "test.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho hello\necho oops >&2\nexit 42\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	chatFn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	sh := NewSelfHealer(chatFn)
	stdout, stderr, exitCode, err := sh.RunScript(context.Background(), script)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitCode)
	}
	if !strings.Contains(stdout, "hello") {
		t.Errorf("expected stdout to contain 'hello', got '%s'", stdout)
	}
	if !strings.Contains(stderr, "oops") {
		t.Errorf("expected stderr to contain 'oops', got '%s'", stderr)
	}
}

func TestRunScript_Timeout(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "slow.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 10\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	chatFn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	sh := NewSelfHealer(chatFn)
	sh.Timeout = 100 * time.Millisecond

	_, _, exitCode, _ := sh.RunScript(context.Background(), script)
	if exitCode == 0 {
		t.Error("expected non-zero exit code for timed-out script")
	}
}

func TestFormatHealResult(t *testing.T) {
	result := &HealResult{
		Attempts: []HealAttempt{
			{Attempt: 1, Script: "script.py", ExitCode: 1, Error: "IndentationError at line 15", Fix: "@@FIX\nFILE: script.py\nACTION: replace\n@@END"},
			{Attempt: 2, Script: "script.py", ExitCode: 1, Error: "NameError: 'config' undefined", Fix: "@@FIX\nFILE: script.py\nACTION: insert\n@@END"},
			{Attempt: 3, Script: "script.py", ExitCode: 0, Success: true},
		},
		FinalSuccess:  true,
		TotalDuration: 8200 * time.Millisecond,
		FixesApplied:  2,
	}

	output := FormatHealResult(result)

	if !strings.Contains(output, "Self-Healing: script.py") {
		t.Error("output should contain script name")
	}
	if !strings.Contains(output, "Attempt 1: FAILED") {
		t.Error("output should show attempt 1 failure")
	}
	if !strings.Contains(output, "Attempt 3: SUCCESS") {
		t.Error("output should show attempt 3 success")
	}
	if !strings.Contains(output, "3 attempts") {
		t.Error("output should show total attempts")
	}
	if !strings.Contains(output, "2 fixes applied") {
		t.Error("output should show fixes applied count")
	}
}

func TestFormatResult_NilResult(t *testing.T) {
	output := FormatHealResult(nil)
	if !strings.Contains(output, "no attempts made") {
		t.Error("should indicate no attempts for nil result")
	}
}

func TestFormatResult_EmptyAttempts(t *testing.T) {
	result := &HealResult{Attempts: []HealAttempt{}}
	output := FormatHealResult(result)
	if !strings.Contains(output, "no attempts made") {
		t.Error("should indicate no attempts for empty result")
	}
}

func TestHeal_ChatFnError(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "broken.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	chatFn := func(ctx context.Context, prompt string) (string, error) {
		return "", fmt.Errorf("API rate limited")
	}
	sh := NewSelfHealer(chatFn)
	sh.MaxAttempts = 2
	_, err := sh.Heal(context.Background(), script)
	if err == nil {
		t.Error("expected error when ChatFn fails")
	}
	if !strings.Contains(err.Error(), "LLM chat failed") {
		t.Errorf("expected LLM chat error, got: %v", err)
	}
}

func TestHeal_ScriptNotReadable(t *testing.T) {
	chatFn := func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}
	sh := NewSelfHealer(chatFn)
	_, err := sh.Heal(context.Background(), "/nonexistent/script.sh")
	if err == nil {
		// The RunScript will fail (non-zero exit), then reading the script will fail
		t.Error("expected error for nonexistent script")
	}
}

func TestExtractErrorSummary(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "unknown error"},
		{"simple error", "simple error"},
		{"Traceback (most recent call last):\n  File \"test.py\"\nNameError: x", "File \"test.py\""},
		{strings.Repeat("a", 100), strings.Repeat("a", 57) + "..."},
	}

	for _, tt := range tests {
		got := extractErrorSummary(tt.input)
		if got != tt.expected {
			t.Errorf("extractErrorSummary(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestHealTruncate(t *testing.T) {
	if healTruncate("short", 10) != "short" {
		t.Error("short strings should not be truncated")
	}
	result := healTruncate("this is a very long string that should be truncated", 20)
	if len(result) != 20 {
		t.Errorf("expected length 20, got %d", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("truncated strings should end with ...")
	}
}

func TestConcurrentHealAttempts(t *testing.T) {
	chatFn := func(ctx context.Context, prompt string) (string, error) {
		return "no fix", nil
	}
	sh := NewSelfHealer(chatFn)

	// Ensure concurrent access to History doesn't race
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			sh.recordAttempt(HealAttempt{Attempt: i, Script: "test"})
		}
	}()

	for i := 0; i < 100; i++ {
		sh.recordAttempt(HealAttempt{Attempt: i + 100, Script: "test"})
	}
	<-done

	sh.mu.Lock()
	count := len(sh.History)
	sh.mu.Unlock()
	if count != 200 {
		t.Errorf("expected 200 history entries, got %d", count)
	}
}
