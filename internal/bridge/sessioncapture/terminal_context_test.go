package sessioncapture

import (
	"os"
	"testing"
)

func TestTerminalContext_BuildContext_Empty(t *testing.T) {
	tc := NewTerminalContext()
	got := tc.BuildContext("hello")
	// First call always includes CWD since lastCWD is empty
	if got == "hello" {
		t.Error("expected CWD to be included on first call")
	}
}

func TestTerminalContext_BuildContext_Delta(t *testing.T) {
	tc := NewTerminalContext()
	tc.captureCmd = "" // disable terminal capture for test

	// First call — includes CWD
	result := tc.BuildContext("q1")
	cwd, _ := os.Getwd()
	if !contains(result, "[cwd: "+cwd+"]") {
		t.Errorf("expected cwd in first call, got: %s", result)
	}

	// Second call — CWD unchanged, should NOT include it
	result = tc.BuildContext("q2")
	if contains(result, "[cwd:") {
		t.Errorf("expected no cwd delta on second call, got: %s", result)
	}
	if result != "q2" {
		t.Errorf("expected bare query when nothing changed, got: %s", result)
	}
}

func TestTerminalContext_MarkCommand(t *testing.T) {
	tc := NewTerminalContext()
	tc.captureCmd = ""
	tc.lastCWD, _ = os.Getwd() // pre-set to avoid CWD delta

	tc.MarkCommand("go test ./...")
	tc.MarkExitCode(1)

	result := tc.BuildContext("why did that fail?")
	if !contains(result, "[recent: go test ./...]") {
		t.Errorf("expected recent command, got: %s", result)
	}
	if !contains(result, "[exit: 1]") {
		t.Errorf("expected exit code, got: %s", result)
	}
}

func TestTerminalContext_Reset(t *testing.T) {
	tc := NewTerminalContext()
	tc.captureCmd = ""
	tc.lastCWD = "/some/path"
	tc.MarkCommand("ls")

	tc.Reset()

	// After reset, CWD should be included again
	result := tc.BuildContext("test")
	cwd, _ := os.Getwd()
	if !contains(result, "[cwd: "+cwd+"]") {
		t.Errorf("expected cwd after reset, got: %s", result)
	}
}

func TestStripANSI(t *testing.T) {
	input := "\x1b[31mERROR\x1b[0m: something failed"
	got := stripANSI(input)
	want := "ERROR: something failed"
	if got != want {
		t.Errorf("stripANSI = %q, want %q", got, want)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
