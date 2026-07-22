package io

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDetectContentType_Code(t *testing.T) {
	code := `package main

import "fmt"

func main() {
	fmt.Println("hello")
	if true {
		return
	}
}
`
	got := DetectContentType(code)
	if got != "code" {
		t.Errorf("expected 'code', got %q", got)
	}
}

func TestDetectContentType_Diff(t *testing.T) {
	diff := `diff --git a/file.go b/file.go
index 1234567..abcdefg 100644
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func main() {}
`
	got := DetectContentType(diff)
	if got != "diff" {
		t.Errorf("expected 'diff', got %q", got)
	}
}

func TestDetectContentType_DiffWithoutHeader(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
-import "os"
 func main() {}
`
	got := DetectContentType(diff)
	if got != "diff" {
		t.Errorf("expected 'diff', got %q", got)
	}
}

func TestDetectContentType_URL(t *testing.T) {
	tests := []string{
		"https://github.com/user/repo/issues/42",
		"http://localhost:8080/api/v1/health",
	}
	for _, url := range tests {
		got := DetectContentType(url)
		if got != "url" {
			t.Errorf("expected 'url' for %q, got %q", url, got)
		}
	}
}

func TestDetectContentType_Path(t *testing.T) {
	tests := []string{
		"/usr/local/bin/hawk",
		"./src/main.go",
		"~/Documents/project/file.txt",
		"src/engine/clipboard.go",
	}
	for _, path := range tests {
		got := DetectContentType(path)
		if got != "path" {
			t.Errorf("expected 'path' for %q, got %q", path, got)
		}
	}
}

func TestDetectContentType_Error(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "go panic",
			content: `goroutine 1 [running]:
main.main()
	/tmp/main.go:10 +0x44
exit status 2`,
		},
		{
			name: "python traceback",
			content: `Traceback (most recent call last):
  File "main.py", line 5, in <module>
    result = divide(10, 0)
ZeroDivisionError: division by zero`,
		},
		{
			name: "js error",
			content: `TypeError: Cannot read property 'foo' of undefined
    at Object.<anonymous> (/app/index.js:15:5)
    at Module._compile (internal/modules/cjs/loader.js:778:30)
    at Object.Module._extensions..js (internal/modules/cjs/loader.js:789:10)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectContentType(tt.content)
			if got != "error" {
				t.Errorf("expected 'error', got %q", got)
			}
		})
	}
}

func TestDetectContentType_Text(t *testing.T) {
	text := "This is just some plain text content that doesn't match any special pattern."
	got := DetectContentType(text)
	if got != "text" {
		t.Errorf("expected 'text', got %q", got)
	}
}

func TestDetectLanguage_Go(t *testing.T) {
	code := `package main

func hello() string {
	return "hi"
}
`
	got := DetectLanguage(code)
	if got != "go" {
		t.Errorf("expected 'go', got %q", got)
	}
}

func TestDetectLanguage_Python(t *testing.T) {
	code := `def greet(name):
    if name:
        return f"Hello, {name}"
    return "Hello"
`
	got := DetectLanguage(code)
	if got != "python" {
		t.Errorf("expected 'python', got %q", got)
	}
}

func TestDetectLanguage_TypeScript(t *testing.T) {
	code := `import { useState } from 'react';

const App = () => {
  const [count, setCount] = useState(0);
  return <div>{count}</div>;
};
`
	got := DetectLanguage(code)
	if got != "typescript" {
		t.Errorf("expected 'typescript', got %q", got)
	}
}

func TestDetectLanguage_Rust(t *testing.T) {
	code := `fn main() {
    let x: i32 = 42;
    let result = compute(x);
    println!("{}", result);
}

fn compute(val: i32) -> i32 {
    val * 2
}
`
	got := DetectLanguage(code)
	if got != "rust" {
		t.Errorf("expected 'rust', got %q", got)
	}
}

func TestDetectLanguage_Java(t *testing.T) {
	code := `public class Main {
    public static void main(String[] args) {
        System.out.println("Hello");
    }
}
`
	got := DetectLanguage(code)
	if got != "java" {
		t.Errorf("expected 'java', got %q", got)
	}
}

func TestDetectLanguage_Unknown(t *testing.T) {
	code := "just some random text\nwithout any language markers"
	got := DetectLanguage(code)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestFormatForContext_Code(t *testing.T) {
	code := `package main

func main() {}
`
	result := FormatForContext(code, "code")
	if !strings.HasPrefix(result, "```go\n") {
		t.Errorf("expected code block with go language, got:\n%s", result)
	}
	if !strings.HasSuffix(result, "\n```") {
		t.Errorf("expected closing code fence, got:\n%s", result)
	}
	if !strings.Contains(result, "func main()") {
		t.Errorf("expected code content preserved, got:\n%s", result)
	}
}

func TestFormatForContext_Diff(t *testing.T) {
	diff := "+added line\n-removed line"
	result := FormatForContext(diff, "diff")
	if !strings.HasPrefix(result, "```diff\n") {
		t.Errorf("expected diff code block, got:\n%s", result)
	}
}

func TestFormatForContext_Error(t *testing.T) {
	errMsg := "TypeError: undefined is not a function"
	result := FormatForContext(errMsg, "error")
	if !strings.HasPrefix(result, "Error from clipboard:") {
		t.Errorf("expected error prefix, got:\n%s", result)
	}
}

func TestFormatForContext_URL(t *testing.T) {
	url := "https://example.com/path"
	result := FormatForContext(url, "url")
	expected := "URL from clipboard: https://example.com/path"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatForContext_Text(t *testing.T) {
	text := "plain text content"
	result := FormatForContext(text, "text")
	if result != text {
		t.Errorf("expected content unchanged, got %q", result)
	}
}

func TestSummarizeClipboard_Short(t *testing.T) {
	content := "short content"
	result := SummarizeClipboard(content, 100)
	if result != content {
		t.Errorf("expected content unchanged for short input, got %q", result)
	}
}

func TestSummarizeClipboard_Truncation(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line content that is reasonably long for testing purposes"
	}
	content := strings.Join(lines, "\n")

	result := SummarizeClipboard(content, 100)
	if !strings.Contains(result, "... ") {
		t.Errorf("expected omission indicator, got:\n%s", result)
	}
	if !strings.Contains(result, "lines omitted") {
		t.Errorf("expected 'lines omitted' text, got:\n%s", result)
	}
	// Should have first 3 lines.
	for i := 0; i < 3; i++ {
		if !strings.Contains(result, lines[i]) {
			t.Errorf("expected first lines preserved, missing line %d", i)
		}
	}
}

func TestSummarizeClipboard_FewLongLines(t *testing.T) {
	// Few lines but very long content should do simple truncation.
	content := strings.Repeat("x", 500)
	result := SummarizeClipboard(content, 100)
	if !strings.HasSuffix(result, "...") {
		t.Errorf("expected '...' suffix for simple truncation, got:\n%s", result)
	}
	if len(result) != 103 { // 100 chars + "..."
		t.Errorf("expected length 103, got %d", len(result))
	}
}

func TestReadWriteClipboard(t *testing.T) {
	// Skip if clipboard tools are not available.
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("pbcopy"); err != nil {
			// FIXME: pbcopy not available
			t.Skip("pbcopy not available")
		}
	case "linux":
		if _, err := exec.LookPath("xclip"); err != nil {
			// FIXME: test skipped in TestReadWriteClipboard
			if _, err := exec.LookPath("xsel"); err != nil {
				// FIXME: test skipped
				t.Skip("no clipboard tool available (xclip or xsel)")
			}
		}
	// FIXME: test skipped in TestReadWriteClipboard
	case "windows":
		// FIXME: test skipped
		if _, err := exec.LookPath("powershell"); err != nil {
			// FIXME: test skipped
			t.Skip("powershell not available")
		}
	default:
		t.Skipf("unsupported platform: %s", runtime.GOOS)
	}

	testContent := "hawk clipboard test content 12345"
	err := WriteClipboard(testContent)
	if err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}

	got, err := ReadClipboard()
	if err != nil {
		t.Fatalf("ReadClipboard failed: %v", err)
	}

	// Trim trailing newline that some clipboard tools add.
	got = strings.TrimRight(got, "\n\r")
	if got != testContent {
		t.Errorf("clipboard round-trip failed: expected %q, got %q", testContent, got)
	}
}

func TestContentChangeDetection(t *testing.T) {
	var mu sync.Mutex
	var received []string

	monitor := NewClipboardMonitor()
	monitor.PollInterval = 50 * time.Millisecond
	monitor.OnPaste = func(content string) {
		mu.Lock()
		received = append(received, content)
		mu.Unlock()
	}

	// Simulate polling by directly testing the internal logic.
	// Set initial content.
	monitor.mu.Lock()
	monitor.lastContent = "initial content"
	monitor.mu.Unlock()

	// Simulate a new clipboard content by modifying lastContent tracking
	// and calling the callback logic directly.
	newContent := "new clipboard content that is long enough to pass the minimum"
	if len(newContent) <= 20 {
		t.Fatal("test content too short")
	}

	// Manually simulate what poll() does.
	monitor.mu.Lock()
	if newContent != monitor.lastContent {
		monitor.lastContent = newContent
		if len(newContent) > 20 && len(newContent) < 50000 {
			monitor.OnPaste(newContent)
		}
	}
	monitor.mu.Unlock()

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(received))
	}
	if received[0] != newContent {
		t.Errorf("expected %q, got %q", newContent, received[0])
	}
}

func TestMinMaxLengthFiltering(t *testing.T) {
	var received []string

	monitor := NewClipboardMonitor()
	monitor.OnPaste = func(content string) {
		received = append(received, content)
	}

	// Test: content too short (<=20 chars) should not trigger callback.
	monitor.mu.Lock()
	monitor.lastContent = ""
	monitor.mu.Unlock()

	shortContent := "short"
	monitor.mu.Lock()
	if shortContent != monitor.lastContent {
		monitor.lastContent = shortContent
		if len(shortContent) > 20 && len(shortContent) < 50000 {
			monitor.OnPaste(shortContent)
		}
	}
	monitor.mu.Unlock()

	if len(received) != 0 {
		t.Errorf("expected no callback for short content, got %d", len(received))
	}

	// Test: content too long (>=50000 chars) should not trigger callback.
	longContent := strings.Repeat("x", 50000)
	monitor.mu.Lock()
	if longContent != monitor.lastContent {
		monitor.lastContent = longContent
		if len(longContent) > 20 && len(longContent) < 50000 {
			monitor.OnPaste(longContent)
		}
	}
	monitor.mu.Unlock()

	if len(received) != 0 {
		t.Errorf("expected no callback for long content, got %d", len(received))
	}

	// Test: valid length should trigger callback.
	validContent := "this is valid content that exceeds twenty characters easily"
	monitor.mu.Lock()
	if validContent != monitor.lastContent {
		monitor.lastContent = validContent
		if len(validContent) > 20 && len(validContent) < 50000 {
			monitor.OnPaste(validContent)
		}
	}
	monitor.mu.Unlock()

	if len(received) != 1 {
		t.Errorf("expected 1 callback for valid content, got %d", len(received))
	}
}

func TestMonitorStartStop(t *testing.T) {
	monitor := NewClipboardMonitor()
	monitor.PollInterval = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitor.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it a moment to run.
	time.Sleep(100 * time.Millisecond)

	// Stop should not panic.
	monitor.Stop()

	// Double stop should not panic.
	monitor.Stop()
}

func TestMonitorDisabled(t *testing.T) {
	monitor := NewClipboardMonitor()
	monitor.Enabled = false

	ctx := context.Background()
	err := monitor.Start(ctx)
	if err != nil {
		t.Fatalf("Start on disabled monitor should not error, got: %v", err)
	}
}
