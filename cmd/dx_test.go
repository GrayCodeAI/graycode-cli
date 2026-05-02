package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
	"github.com/GrayCodeAI/hawk/engine"
	"github.com/GrayCodeAI/hawk/tool"
)

func TestDoctorOutputContainsSections(t *testing.T) {
	version = "test-dx-version"
	settings := hawkconfig.Settings{
		Provider: "openai",
		Model:    "gpt-4o",
	}

	out := doctorOutput(settings)

	sections := []string{
		"Hawk Doctor",
		"Go version:",
		"OS:",
		"Arch:",
		"Shell:",
		"TERM:",
		"COLORTERM:",
		"Version:",
		"Provider:",
		"API key:",
		"Model:",
		"Session directory:",
		"MCP servers:",
		"Plugins:",
		"AGENTS.md:",
		"Git:",
	}
	for _, section := range sections {
		if !strings.Contains(out, section) {
			t.Errorf("doctorOutput missing section %q\nOutput:\n%s", section, out)
		}
	}
}

func TestDoctorOutputWithMCPServers(t *testing.T) {
	version = "test-dx-version"
	settings := hawkconfig.Settings{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-20250514",
		MCPServers: []hawkconfig.MCPServerConfig{
			{Name: "test-mcp", Command: "test-cmd"},
		},
	}

	out := doctorOutput(settings)
	if !strings.Contains(out, "MCP servers:     1") {
		t.Errorf("expected MCP servers count of 1, got:\n%s", out)
	}
}

func TestDebugOutputHasMemoryStats(t *testing.T) {
	sess := engine.NewSession("openai", "gpt-4o", "test system", tool.NewRegistry())
	sess.AddUser("hello")

	out := debugOutput(sess, "test-session-id")

	required := []string{
		"Debug Info",
		"Session ID:",
		"Message count:",
		"Token estimate:",
		"Provider:",
		"Model:",
		"Compaction:",
		"Memory:",
		"Alloc:",
		"TotalAlloc:",
		"Goroutines:",
		"Uptime:",
	}
	for _, section := range required {
		if !strings.Contains(out, section) {
			t.Errorf("debugOutput missing %q\nOutput:\n%s", section, out)
		}
	}

	// Session ID should appear
	if !strings.Contains(out, "test-session-id") {
		t.Errorf("debugOutput missing session ID")
	}
}

func TestDebugOutputShowsProvider(t *testing.T) {
	sess := engine.NewSession("anthropic", "claude-sonnet-4-20250514", "test", tool.NewRegistry())

	out := debugOutput(sess, "abc123")
	if !strings.Contains(out, "anthropic") {
		t.Errorf("expected provider 'anthropic' in debug output, got:\n%s", out)
	}
	if !strings.Contains(out, "claude-sonnet-4-20250514") {
		t.Errorf("expected model in debug output, got:\n%s", out)
	}
}

func TestMetricsOutputContainsSections(t *testing.T) {
	sess := engine.NewSession("openai", "gpt-4o", "test", tool.NewRegistry())

	out := metricsOutput(sess)

	required := []string{
		"Resource Metrics",
		"Allocated memory:",
		"Total allocations:",
		"GC runs:",
		"Goroutines active:",
	}
	for _, section := range required {
		if !strings.Contains(out, section) {
			t.Errorf("metricsOutput missing %q\nOutput:\n%s", section, out)
		}
	}
}

func TestExportMarkdownCreatesFile(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	messages := []displayMsg{
		{role: "user", content: "Hello hawk"},
		{role: "assistant", content: "Hello! How can I help?"},
		{role: "system", content: "System message here"},
		{role: "welcome", content: "Should be skipped"},
	}

	path, err := exportMarkdown(messages, "test-export-id")
	if err != nil {
		t.Fatalf("exportMarkdown failed: %v", err)
	}

	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "# Hawk Session: test-export-id") {
		t.Errorf("export missing session header")
	}
	if !strings.Contains(content, "Hello hawk") {
		t.Errorf("export missing user message")
	}
	if !strings.Contains(content, "Hello! How can I help?") {
		t.Errorf("export missing assistant message")
	}
	if strings.Contains(content, "Should be skipped") {
		t.Errorf("export should not contain welcome messages")
	}
	if !strings.Contains(content, "## User") {
		t.Errorf("export missing User heading")
	}
	if !strings.Contains(content, "## Assistant") {
		t.Errorf("export missing Assistant heading")
	}
}

func TestExportJSONCreatesValidFile(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	messages := []displayMsg{
		{role: "user", content: "What is Go?"},
		{role: "assistant", content: "Go is a programming language."},
		{role: "welcome", content: "Skip this"},
		{role: "error", content: "Something went wrong"},
	}

	path, err := exportJSON(messages, "json-export-id")
	if err != nil {
		t.Fatalf("exportJSON failed: %v", err)
	}

	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read exported JSON file: %v", err)
	}

	// Verify valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("exported JSON is invalid: %v\nContent:\n%s", err, string(data))
	}

	if result["session_id"] != "json-export-id" {
		t.Errorf("expected session_id 'json-export-id', got %v", result["session_id"])
	}

	msgs, ok := result["messages"].([]interface{})
	if !ok {
		t.Fatalf("expected messages array, got %T", result["messages"])
	}
	// welcome is skipped, so we should have 3 messages
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages (welcome skipped), got %d", len(msgs))
	}

	// Verify first message is user
	first, _ := msgs[0].(map[string]interface{})
	if first["role"] != "user" {
		t.Errorf("expected first message role 'user', got %v", first["role"])
	}
}

func TestExportJSONSkipsEmptyContent(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	messages := []displayMsg{
		{role: "user", content: "Test"},
		{role: "usage", content: ""}, // empty content should be skipped
	}

	path, err := exportJSON(messages, "empty-test")
	if err != nil {
		t.Fatalf("exportJSON failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	var result map[string]interface{}
	json.Unmarshal(data, &result)

	msgs := result["messages"].([]interface{})
	if len(msgs) != 1 {
		t.Errorf("expected 1 message (empty skipped), got %d", len(msgs))
	}
}

func TestRetryLastPrompt(t *testing.T) {
	sess := engine.NewSession("openai", "gpt-4o", "test", tool.NewRegistry())
	sess.AddUser("first question")
	sess.AddAssistant("first answer")
	sess.AddUser("second question")
	sess.AddAssistant("second answer")

	last := retryLastPrompt(sess)
	if last != "second question" {
		t.Errorf("expected 'second question', got %q", last)
	}
}

func TestRetryLastPromptEmpty(t *testing.T) {
	sess := engine.NewSession("openai", "gpt-4o", "test", tool.NewRegistry())

	last := retryLastPrompt(sess)
	if last != "" {
		t.Errorf("expected empty string for no messages, got %q", last)
	}
}

func TestRetryLastPromptSkipsAssistant(t *testing.T) {
	sess := engine.NewSession("openai", "gpt-4o", "test", tool.NewRegistry())
	sess.AddUser("the only question")
	sess.AddAssistant("the answer")

	last := retryLastPrompt(sess)
	if last != "the only question" {
		t.Errorf("expected 'the only question', got %q", last)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    uint64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
		{2560, "2.50 KB"},
	}
	for _, tc := range tests {
		got := formatBytes(tc.input)
		if got != tc.expected {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestMaskedKeyStatus(t *testing.T) {
	// Empty provider
	result := maskedKeyStatus("")
	if result != "(no provider set)" {
		t.Errorf("expected '(no provider set)', got %q", result)
	}

	// Provider with no key set should show missing
	result = maskedKeyStatus("nonexistent-provider-xyz")
	if result != "missing" {
		t.Errorf("expected 'missing' for unset provider, got %q", result)
	}
}

func TestCountOpenFDs(t *testing.T) {
	count := countOpenFDs()
	// On darwin and linux, we should have at least stdin/stdout/stderr
	if count < 0 {
		t.Skip("countOpenFDs not available on this platform")
	}
	if count < 3 {
		t.Errorf("expected at least 3 open FDs, got %d", count)
	}
}
