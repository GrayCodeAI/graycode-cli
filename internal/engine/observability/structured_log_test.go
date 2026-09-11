package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewStructuredLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelInfo, &buf)

	if logger.Level != LevelInfo {
		t.Errorf("expected level %d, got %d", LevelInfo, logger.Level)
	}
	if logger.Format != "json" {
		t.Errorf("expected format 'json', got %q", logger.Format)
	}
	if logger.Fields == nil {
		t.Error("expected Fields to be initialized")
	}
}

func TestLogLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelWarn, &buf)

	logger.Debug("should not appear")
	logger.Info("should not appear")
	if buf.Len() != 0 {
		t.Errorf("expected empty buffer for filtered messages, got %q", buf.String())
	}

	logger.Warn("should appear")
	if buf.Len() == 0 {
		t.Error("expected warn message to appear")
	}

	buf.Reset()
	logger.Error("should also appear")
	if buf.Len() == 0 {
		t.Error("expected error message to appear")
	}
}

func TestJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelDebug, &buf)
	logger.Format = "json"
	logger.SessionID = "sess-123"

	logger.Info("test message", map[string]interface{}{
		FieldTool: "Edit",
		FieldFile: "main.go",
	})

	var entry LogEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if entry.Level != "INFO" {
		t.Errorf("expected level INFO, got %q", entry.Level)
	}
	if entry.Message != "test message" {
		t.Errorf("expected message 'test message', got %q", entry.Message)
	}
	if entry.SessionID != "sess-123" {
		t.Errorf("expected session_id 'sess-123', got %q", entry.SessionID)
	}
	if entry.Fields[FieldTool] != "Edit" {
		t.Errorf("expected field tool=Edit, got %v", entry.Fields[FieldTool])
	}
	if entry.Fields[FieldFile] != "main.go" {
		t.Errorf("expected field file=main.go, got %v", entry.Fields[FieldFile])
	}
	if entry.Caller == "" {
		t.Error("expected caller to be populated")
	}
}

func TestTextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelDebug, &buf)
	logger.Format = "text"

	logger.Info("tool.execute", map[string]interface{}{
		FieldTool: "Edit",
		FieldFile: "main.go",
	})

	output := buf.String()
	if !strings.Contains(output, "[INFO]") {
		t.Errorf("expected [INFO] in text output, got %q", output)
	}
	if !strings.Contains(output, "tool.execute") {
		t.Errorf("expected 'tool.execute' in text output, got %q", output)
	}
	if !strings.Contains(output, "tool=Edit") {
		t.Errorf("expected 'tool=Edit' in text output, got %q", output)
	}
	if !strings.Contains(output, "file=main.go") {
		t.Errorf("expected 'file=main.go' in text output, got %q", output)
	}
}

func TestWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelDebug, &buf)

	child := logger.WithFields(map[string]interface{}{
		FieldModel:    "gpt-4",
		FieldProvider: "openai",
	})

	child.Info("request")

	var entry LogEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry.Fields[FieldModel] != "gpt-4" {
		t.Errorf("expected model=gpt-4, got %v", entry.Fields[FieldModel])
	}
	if entry.Fields[FieldProvider] != "openai" {
		t.Errorf("expected provider=openai, got %v", entry.Fields[FieldProvider])
	}

	// Ensure parent was not mutated.
	if len(logger.Fields) != 0 {
		t.Errorf("expected parent fields to be empty, got %v", logger.Fields)
	}
}

func TestWithSession(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelDebug, &buf)

	sessLogger := logger.WithSession("abc-def")
	sessLogger.Info("hello")

	var entry LogEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry.SessionID != "abc-def" {
		t.Errorf("expected session_id 'abc-def', got %q", entry.SessionID)
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"warn", LevelWarn},
		{"WARN", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"fatal", LevelFatal},
		{"FATAL", LevelFatal},
		{"unknown", LevelInfo},
		{"", LevelInfo},
	}

	for _, tt := range tests {
		got := ParseLevel(tt.input)
		if got != tt.expected {
			t.Errorf("ParseLevel(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestAgentLoggerToolCall(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelDebug, &buf)
	agent := &AgentLogger{Logger: logger, Turn: 3, Model: "claude-3"}

	agent.LogToolCall("Edit", "main.go", 120*time.Millisecond, nil)

	var entry LogEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry.Level != "INFO" {
		t.Errorf("expected INFO for successful tool call, got %q", entry.Level)
	}
	if entry.Message != "tool.execute" {
		t.Errorf("expected message 'tool.execute', got %q", entry.Message)
	}
	if entry.Fields[FieldTool] != "Edit" {
		t.Errorf("expected tool=Edit, got %v", entry.Fields[FieldTool])
	}

	// Test with error.
	buf.Reset()
	agent.LogToolCall("Bash", "script.sh", 500*time.Millisecond, errors.New("command failed"))

	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if entry.Level != "ERROR" {
		t.Errorf("expected ERROR for failed tool call, got %q", entry.Level)
	}
	if entry.Fields[FieldError] != "command failed" {
		t.Errorf("expected error='command failed', got %v", entry.Fields[FieldError])
	}
}

func TestAgentLoggerAPICall(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelDebug, &buf)
	agent := &AgentLogger{Logger: logger, Turn: 1, Model: "claude-3"}

	agent.LogAPICall("claude-3-opus", 1500, 0.045, 2*time.Second)

	var entry LogEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry.Message != "api.call" {
		t.Errorf("expected message 'api.call', got %q", entry.Message)
	}
	if entry.Fields[FieldModel] != "claude-3-opus" {
		t.Errorf("expected model=claude-3-opus, got %v", entry.Fields[FieldModel])
	}
	// JSON numbers decode as float64.
	if entry.Fields[FieldTokens] != float64(1500) {
		t.Errorf("expected tokens=1500, got %v", entry.Fields[FieldTokens])
	}
	if entry.Fields[FieldCost] != 0.045 {
		t.Errorf("expected cost=0.045, got %v", entry.Fields[FieldCost])
	}
}

func TestAgentLoggerCompaction(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelDebug, &buf)
	agent := &AgentLogger{Logger: logger, Turn: 5, Model: "claude-3"}

	agent.LogCompaction(100, 30)

	var entry LogEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry.Message != "context.compact" {
		t.Errorf("expected message 'context.compact', got %q", entry.Message)
	}
	if entry.Fields["before"] != float64(100) {
		t.Errorf("expected before=100, got %v", entry.Fields["before"])
	}
	if entry.Fields["after"] != float64(30) {
		t.Errorf("expected after=30, got %v", entry.Fields["after"])
	}
}

func TestAgentLoggerPermission(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelDebug, &buf)
	agent := &AgentLogger{Logger: logger, Turn: 2, Model: "claude-3"}

	agent.LogPermission("Bash", true)

	var entry LogEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry.Message != "permission.check" {
		t.Errorf("expected message 'permission.check', got %q", entry.Message)
	}
	if entry.Fields[FieldTool] != "Bash" {
		t.Errorf("expected tool=Bash, got %v", entry.Fields[FieldTool])
	}
	if entry.Fields["granted"] != true {
		t.Errorf("expected granted=true, got %v", entry.Fields["granted"])
	}
}

func TestConcurrentLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelDebug, &buf)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			logger.Info("concurrent", map[string]interface{}{"n": n})
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 100 {
		t.Errorf("expected 100 log lines, got %d", len(lines))
	}

	// Verify each line is valid JSON.
	for i, line := range lines {
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d is invalid JSON: %v", i, err)
		}
	}
}

func TestRotatingWriter(t *testing.T) {
	dir := t.TempDir()

	rw, err := NewRotatingWriter(dir, "hawk")
	if err != nil {
		t.Fatalf("failed to create rotating writer: %v", err)
	}
	defer rw.Close()

	// Set a small max size for testing.
	rw.MaxSize = 100
	rw.MaxFiles = 3

	// Write enough data to trigger rotation.
	data := bytes.Repeat([]byte("A"), 60)
	for i := 0; i < 5; i++ {
		_, writeErr := rw.Write(data)
		if writeErr != nil {
			t.Fatalf("write %d failed: %v", i, writeErr)
		}
	}

	// Check that rotated files exist.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}

	var logFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			logFiles = append(logFiles, e.Name())
		}
	}

	if len(logFiles) == 0 {
		t.Error("expected at least one log file")
	}

	// Verify main log file exists.
	mainLog := filepath.Join(dir, "hawk.log")
	if _, err := os.Stat(mainLog); os.IsNotExist(err) {
		t.Error("expected main log file hawk.log to exist")
	}
}

func TestRotatingWriterMaxFiles(t *testing.T) {
	dir := t.TempDir()

	rw, err := NewRotatingWriter(dir, "app")
	if err != nil {
		t.Fatalf("failed to create rotating writer: %v", err)
	}
	defer rw.Close()

	rw.MaxSize = 50
	rw.MaxFiles = 2

	// Write enough to trigger multiple rotations.
	data := bytes.Repeat([]byte("X"), 60)
	for i := 0; i < 10; i++ {
		rw.Write(data)
	}

	// We should have at most MaxFiles + 1 files (main + rotated).
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}

	var logFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			logFiles = append(logFiles, e.Name())
		}
	}

	maxExpected := rw.MaxFiles + 1
	if len(logFiles) > maxExpected {
		t.Errorf("expected at most %d log files, got %d: %v", maxExpected, len(logFiles), logFiles)
	}
}

func TestFieldKeyConstants(t *testing.T) {
	// Ensure field keys are non-empty and unique.
	keys := []string{FieldTool, FieldFile, FieldDuration, FieldTokens, FieldModel, FieldProvider, FieldError, FieldCost}
	seen := make(map[string]bool)
	for _, k := range keys {
		if k == "" {
			t.Error("field key constant must not be empty")
		}
		if seen[k] {
			t.Errorf("duplicate field key constant: %q", k)
		}
		seen[k] = true
	}
}

func TestWithFieldsChaining(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelDebug, &buf)

	child1 := logger.WithFields(map[string]interface{}{"a": 1})
	child2 := child1.WithFields(map[string]interface{}{"b": 2})

	child2.Info("chained")

	var entry LogEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry.Fields["a"] != float64(1) {
		t.Errorf("expected a=1, got %v", entry.Fields["a"])
	}
	if entry.Fields["b"] != float64(2) {
		t.Errorf("expected b=2, got %v", entry.Fields["b"])
	}
}

func TestMergeInlineFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(LevelDebug, &buf)
	logger = logger.WithFields(map[string]interface{}{"base": "val"})

	logger.Info("msg", map[string]interface{}{"inline": "yes"})

	var entry LogEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry.Fields["base"] != "val" {
		t.Errorf("expected base=val, got %v", entry.Fields["base"])
	}
	if entry.Fields["inline"] != "yes" {
		t.Errorf("expected inline=yes, got %v", entry.Fields["inline"])
	}
}
