package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestLevels(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{Debug, "DEBUG"},
		{Info, "INFO"},
		{Warn, "WARN"},
		{Error, "ERROR"},
		{Fatal, "FATAL"},
		{Level(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if s := tt.level.String(); s != tt.expected {
			t.Errorf("Level(%d).String() = %q, want %q", tt.level, s, tt.expected)
		}
	}
}

func TestLogging(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, Debug)

	l.Debug("debug message")
	if !strings.Contains(buf.String(), "debug message") {
		t.Fatal("expected debug message")
	}

	l.Info("info message")
	if !strings.Contains(buf.String(), "info message") {
		t.Fatal("expected info message")
	}

	l.Warn("warn message")
	if !strings.Contains(buf.String(), "warn message") {
		t.Fatal("expected warn message")
	}

	l.Error("error message")
	if !strings.Contains(buf.String(), "error message") {
		t.Fatal("expected error message")
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, Warn)

	l.Debug("debug")
	l.Info("info")
	if buf.Len() > 0 {
		t.Fatal("expected no output for filtered levels")
	}

	l.Warn("warn")
	if !strings.Contains(buf.String(), "warn") {
		t.Fatal("expected warn message")
	}
}

func TestWithFields(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, Debug)

	l.Info("test", map[string]interface{}{"key": "value", "num": 42})
	output := buf.String()
	if !strings.Contains(output, "key=value") {
		t.Fatalf("expected fields in output, got: %s", output)
	}
}

func TestWithPrefix(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, Debug).WithPrefix("[test]")

	l.Info("message")
	if !strings.Contains(buf.String(), "[test]") {
		t.Fatal("expected prefix in output")
	}
}

func TestSetLevel(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, Info)
	l.SetLevel(Error)

	l.Info("info")
	if buf.Len() > 0 {
		t.Fatal("expected no output after level change")
	}
}

func TestNewWithDeps_Defaults(t *testing.T) {
	l := NewWithDeps(LoggerDeps{})
	// Level zero-value is Debug (iota=0)
	if l.level != Debug {
		t.Fatalf("expected default level Debug (zero-value), got %v", l.level)
	}
	if l.output == nil {
		t.Fatal("expected non-nil default output")
	}
}

func TestNewWithDeps_Overrides(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithDeps(LoggerDeps{Output: &buf, Level: Warn})
	if l.level != Warn {
		t.Fatalf("expected level Warn, got %v", l.level)
	}
	l.Warn("test")
	if !strings.Contains(buf.String(), "test") {
		t.Fatal("expected output to configured writer")
	}
}
