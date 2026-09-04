package crash_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/crash"
	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

// reportRoot returns the crash report dir by appending the subdir name to the
// state dir, matching the layout crash.WriteReport uses. We go through the
// public storage.StateDir so the test stays in-package-friendly and inherits
// the same GRAYCODE_STATE_DIR override production uses.
func reportRoot(t *testing.T) string {
	t.Helper()
	base := storage.StateDir()
	if base == "" {
		t.Fatalf("unable to determine state dir")
	}
	return filepath.Join(base, "crash")
}

func TestReportPath_InStateDir(t *testing.T) {
	// GRAYCODE_STATE_DIR must be set before Install/crash writes anything; mirror
	// the convention storage's own tests use.
	dir := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", dir)

	got, err := crash.WriteReport("boom", []byte("goroutine 1 [running]:\nmain.main()\n"))
	if err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	wantDir := filepath.Join(dir, "crash")
	if filepath.Dir(got) != wantDir {
		t.Fatalf("report path = %q, want under %q", got, wantDir)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("report file missing: %v", err)
	}
}

func TestInstall_DoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", dir)

	// Install must be safe and idempotent.
	crash.Install()
	crash.Install()

	// Exercise the panic-recover write path with a synthetic recovered value
	// via Recover. Recover re-panics, so drive it in a subprocess-like goroutine
	// and assert the report file exists afterward.
	done := make(chan string, 1)
	go func() {
		var path string
		var err error
		func() {
			defer func() {
				r := recover()
				if r == nil {
					// We intend to panic below; if not, that's a test bug.
					t.Errorf("expected panic to be triggered")
					return
				}
				// Re-run Recover's write logic by writing a report for the value.
				path, err = crash.WriteReport(r, crash.CaptureGoroutines())
			}()
			panic("synthetic test panic")
		}()
		if err != nil {
			t.Errorf("WriteReport after panic: %v", err)
		}
		done <- path
	}()

	<-done

	// Crash one more recovered report from a plain goroutine to exercise the
	// write path with a non-nil recovered value and confirm a report lands in
	// the state dir.
	extra := make(chan string, 1)
	go func() {
		path, err := crash.WriteReport("second crash", nil)
		if err != nil {
			t.Errorf("WriteReport: %v", err)
		}
		extra <- path
	}()
	if p := <-extra; p == "" {
		t.Fatal("expected a report path")
	}

	// At least one report file should exist in the crash dir.
	entries, err := os.ReadDir(reportRoot(t))
	if err != nil {
		t.Fatalf("read crash dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one crash report file in state dir")
	}
}

func TestCaptureGoroutines_Nonempty(t *testing.T) {
	out := crash.CaptureGoroutines()
	if len(out) == 0 {
		t.Fatal("CaptureGoroutines returned empty dump")
	}
	// Should contain the current goroutine's stack marker.
	if !contains(string(out), "goroutine") {
		t.Fatalf("stack dump missing goroutine marker: %q", string(out)[:min(128, len(out))])
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

// indexOf is a tiny local strings.Index to keep the test stdlib-only and clear.
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
