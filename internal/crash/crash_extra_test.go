package crash

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testEnvStateDir = "HAWK_STATE_DIR"

// --- WriteReport tests ---

func TestWriteReport_ValidInput(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv(testEnvStateDir, stateDir)

	stack := []byte("goroutine 1 [running]:\nmain.main()")
	path, err := WriteReport("test panic", stack)
	if err != nil {
		t.Fatalf("WriteReport returned error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}

	// Verify file exists
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}
	if !strings.Contains(string(content), "test panic") {
		t.Error("report should contain panic value")
	}
	if !strings.Contains(string(content), "goroutine") {
		t.Error("report should contain stack trace")
	}
}

func TestWriteReport_NilRecovered(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv(testEnvStateDir, stateDir)

	path, err := WriteReport(nil, []byte("stack trace"))
	if err != nil {
		t.Fatalf("WriteReport returned error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}
	// Should not contain "panic value" line since recovered is nil
	if strings.Contains(string(content), "panic value") {
		t.Error("report should not contain panic value for nil recovered")
	}
}

func TestWriteReport_EmptyStack(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv(testEnvStateDir, stateDir)

	path, err := WriteReport("error", []byte{})
	if err != nil {
		t.Fatalf("WriteReport returned error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestWriteReport_CreatesDirectory(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv(testEnvStateDir, stateDir)

	// Directory shouldn't exist yet
	reportDirPath := filepath.Join(stateDir, reportDirName)
	if _, err := os.Stat(reportDirPath); !os.IsNotExist(err) {
		t.Fatal("report dir should not exist before WriteReport")
	}

	// WriteReport should create it
	_, err := WriteReport("test", []byte("stack"))
	if err != nil {
		t.Fatalf("WriteReport returned error: %v", err)
	}

	// Directory should now exist
	info, err := os.Stat(reportDirPath)
	if err != nil {
		t.Fatalf("report dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("report path should be a directory")
	}
}

// --- formatReport tests ---

func TestFormatReport_FullFields(t *testing.T) {
	r := CrashReport{
		Timestamp:  mustParseTime(t, "2024-01-15T10:30:00Z"),
		Version:    "1.2.3",
		PanicValue: "runtime error: nil pointer",
		Signal:     "SIGSEGV",
		Stack:      "goroutine 1 [running]:\nmain.main()",
	}

	result := formatReport(r)
	if !strings.Contains(result, "hawk crash report") {
		t.Error("report should have header")
	}
	if !strings.Contains(result, "1.2.3") {
		t.Error("report should contain version")
	}
	if !strings.Contains(result, "nil pointer") {
		t.Error("report should contain panic value")
	}
	if !strings.Contains(result, "SIGSEGV") {
		t.Error("report should contain signal")
	}
	if !strings.Contains(result, "goroutine") {
		t.Error("report should contain stack")
	}
}

func TestFormatReport_MinimalFields(t *testing.T) {
	r := CrashReport{
		Timestamp: mustParseTime(t, "2024-01-15T10:30:00Z"),
		Stack:     "stack trace only",
	}

	result := formatReport(r)
	if strings.Contains(result, "version:") {
		t.Error("report should not contain version when empty")
	}
	if strings.Contains(result, "panic value:") {
		t.Error("report should not contain panic value when empty")
	}
	if strings.Contains(result, "signal:") {
		t.Error("report should not contain signal when empty")
	}
}

func TestFormatReport_TimestampFormat(t *testing.T) {
	r := CrashReport{
		Timestamp: mustParseTime(t, "2024-01-15T10:30:00.123456789Z"),
		Stack:     "stack",
	}

	result := formatReport(r)
	// Should use RFC3339Nano format
	if !strings.Contains(result, "2024-01-15T10:30:00") {
		t.Errorf("report should contain formatted timestamp, got: %s", result)
	}
}

// --- pruneReports tests ---

func TestPruneReports_UnderLimit(t *testing.T) {
	dir := t.TempDir()

	// Create fewer files than the limit with different names
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, fmt.Sprintf("crash-2024010%dT000000.000Z.txt", i))
		if err := os.WriteFile(name, []byte("report"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	pruneReports(dir)

	// All files should still exist
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 files, got %d", len(entries))
	}
}

func TestPruneReports_OverLimit(t *testing.T) {
	dir := t.TempDir()

	// Create more files than the limit with different names
	for i := 0; i < 15; i++ {
		name := filepath.Join(dir, fmt.Sprintf("crash-2024010%dT000000.000Z.txt", i))
		if err := os.WriteFile(name, []byte("report"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	pruneReports(dir)

	// Should have maxReports files remaining
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != maxReports {
		t.Errorf("expected %d files, got %d", maxReports, len(entries))
	}
}

func TestPruneReports_IgnoresNonTxtFiles(t *testing.T) {
	dir := t.TempDir()

	// Create some .txt files and some non-.txt files
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, "crash-20240101T000000.000Z.txt")
		if err := os.WriteFile(name, []byte("report"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Create a non-.txt file that should be ignored
	otherFile := filepath.Join(dir, "other.json")
	if err := os.WriteFile(otherFile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	pruneReports(dir)

	// Non-.txt file should still exist
	if _, err := os.Stat(otherFile); err != nil {
		t.Error("non-.txt file should not be pruned")
	}
}

func TestPruneReports_IgnoresDirectories(t *testing.T) {
	dir := t.TempDir()

	// Create a subdirectory
	subDir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subDir, 0o700); err != nil {
		t.Fatal(err)
	}

	pruneReports(dir)

	// Subdirectory should still exist
	if _, err := os.Stat(subDir); err != nil {
		t.Error("subdirectory should not be pruned")
	}
}

// --- CaptureGoroutines tests ---

func TestCaptureGoroutines_ReturnsData(t *testing.T) {
	result := CaptureGoroutines()
	if len(result) == 0 {
		t.Error("CaptureGoroutines should return non-empty result")
	}
	// Should contain goroutine information
	if !strings.Contains(string(result), "goroutine") {
		t.Error("result should contain 'goroutine'")
	}
}

func TestCaptureGoroutines_NeverPanics(t *testing.T) {
	// Just verify it doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CaptureGoroutines panicked: %v", r)
		}
	}()
	_ = CaptureGoroutines()
}

// --- reportDir tests ---

func TestReportDir_CreatesDirectory(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv(testEnvStateDir, stateDir)

	dir, err := reportDir()
	if err != nil {
		t.Fatalf("reportDir returned error: %v", err)
	}

	// Should be stateDir/crash
	expected := filepath.Join(stateDir, reportDirName)
	if dir != expected {
		t.Errorf("reportDir = %q, want %q", dir, expected)
	}

	// Directory should exist
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("should be a directory")
	}
}

func TestReportDir_Idempotent(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv(testEnvStateDir, stateDir)

	// Call twice - should succeed both times
	dir1, err := reportDir()
	if err != nil {
		t.Fatalf("first call returned error: %v", err)
	}
	dir2, err := reportDir()
	if err != nil {
		t.Fatalf("second call returned error: %v", err)
	}
	if dir1 != dir2 {
		t.Errorf("reportDir should be idempotent: %q vs %q", dir1, dir2)
	}
}

// --- CrashReport struct tests ---

func TestCrashReport_StructCreation(t *testing.T) {
	r := CrashReport{
		Timestamp:  mustParseTime(t, "2024-01-15T10:30:00Z"),
		PanicValue: "test",
		Stack:      "stack",
	}
	if r.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
	if r.PanicValue != "test" {
		t.Error("PanicValue should be set")
	}
	if r.Stack != "stack" {
		t.Error("Stack should be set")
	}
}

// --- Helper ---

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		// Try without nanoseconds
		ts, err = time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatalf("failed to parse time %q: %v", s, err)
		}
	}
	return ts
}
