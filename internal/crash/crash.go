// Package crash installs an additive panic/crash handler that writes a
// structured, timestamped crash report to the state dir and then re-raises the
// original panic/signal. It exists purely for diagnostics: behavior for callers
// is unchanged because the original fault is always re-raised after the report
// is written.
//
// The handler is optional and safe to call on all platforms. Everything is
// guarded so this package never panics itself. The POSIX signal wiring
// (SIGQUIT goroutine dumps) is gated behind build tags so it does not compile
// on Windows. SIGTERM is deliberately left to the app's own graceful-shutdown
// handling (Bubble Tea) so a normal `kill` does not produce a spurious crash
// report.
//
// On Go 1.23+ runtime.SetCrashOutput is used as a complementary sink for fatal
// runtime errors. The call is split into per-Go-version files
// (crash_runtime.go / crash_runtime_stub.go) so this package still builds on
// older Go.
//
// Modeled on grok `xai-crash-handler` (SIGBUS/SIGSEGV + goroutine dump + report
// archive), translated to a Go recover()-based panic path + signal path.
package crash

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

const (
	// maxReports caps the number of crash reports kept in the state dir;
	// older reports are pruned on each successfully written incident.
	maxReports = 10
	// reportDirName is the state subdir holding crash reports.
	reportDirName = "crash"
)

// now is a thin indirection so tests can pin time if needed.
var now = time.Now

var (
	// mu serializes concurrent installs / report writes.
	mu        sync.Mutex
	installed bool
)

// CrashReport is the on-disk + in-memory representation of a single crash.
type CrashReport struct {
	// Timestamp is when the report was written.
	Timestamp time.Time `json:"timestamp"`
	// Version is captured from the report dir; populated by caller if available.
	Version string `json:"version,omitempty"`
	// PanicValue is the recovered panic value (empty if triggered by a signal).
	PanicValue string `json:"panic_value,omitempty"`
	// Signal is the signal name for signal-triggered dumps, else "".
	Signal string `json:"signal,omitempty"`
	// Stack is the goroutine stack captured at the time of the fault.
	Stack string `json:"stack"`
}

// Install sets up the crash handler. Safe to call multiple times (idempotent):
// after the first successful call, subsequent calls are a no-op. Install is
// additive — it does not replace any existing panic/recover logic the caller
// may already have.
func Install() {
	mu.Lock()
	defer mu.Unlock()
	if installed {
		return
	}
	installSignalHandlers()
	// installRuntimeCrashOutput is defined in a per-Go-version file:
	// crash_runtime.go (go1.23+, wires runtime/debug.SetCrashOutput) and
	// crash_runtime_stub.go (older Go, no-op). It opens the runtime crash log
	// and (on supported versions) points the runtime's crash output at it.
	installRuntimeCrashOutput()
	installed = true
}

// Recover is intended to be deferred in the goroutine you want crash-diagnosed.
//
//	defer crash.Recover()
//
// On panic it captures the value + stack, writes a report, and re-panics so the
// original behavior is preserved. If the report cannot be written the panic is
// still re-raised. Calling Recover when there is no panic is a no-op.
func Recover() {
	if r := recover(); r != nil {
		stack := CaptureGoroutines()
		if _, err := WriteReport(r, stack); err != nil {
			// Last-ditch: surface the failure on stderr so diagnostics are
			// still visible even if the report itself failed.
			fmt.Fprintf(os.Stderr, "crash: failed to write report: %v\n", err)
		}
		panic(r)
	}
}

// reportDir returns the directory for crash reports, creating it if needed.
func reportDir() (string, error) {
	base := storage.StateDir()
	if base == "" {
		return "", errors.New("crash: unable to determine state dir")
	}
	dir := filepath.Join(base, reportDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("crash: creating report dir: %w", err)
	}
	return dir, nil
}

// WriteReport writes a crash report for the provided recovered value + stack,
// prunes old reports, and returns the on-disk path. It is safe to call with a
// nil recovered value (e.g. a signal-only dump) — PanicValue will be empty.
// WriteReport never panics; a write failure is returned but does not crash the
// process.
func WriteReport(recovered any, stack []byte) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	dir, err := reportDir()
	if err != nil {
		return "", err
	}

	r := CrashReport{
		Timestamp: now().UTC(),
		Stack:     string(stack),
	}
	if recovered != nil {
		r.PanicValue = fmt.Sprintf("%v", recovered)
	}
	filename := fmt.Sprintf("crash-%s.txt", now().UTC().Format("20060102T150405.000Z"))
	path := filepath.Join(dir, filename)

	content := formatReport(r)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("crash: writing report: %w", err)
	}

	pruneReports(dir)
	return path, nil
}

func formatReport(r CrashReport) string {
	var b strings.Builder
	b.WriteString("hawk crash report\n")
	fmt.Fprintf(&b, "timestamp:   %s\n", r.Timestamp.Format(time.RFC3339Nano))
	if r.Version != "" {
		fmt.Fprintf(&b, "version:     %s\n", r.Version)
	}
	if r.PanicValue != "" {
		fmt.Fprintf(&b, "panic value: %s\n", r.PanicValue)
	}
	if r.Signal != "" {
		fmt.Fprintf(&b, "signal:      %s\n", r.Signal)
	}
	fmt.Fprintf(&b, "\n%s\n", strings.TrimSpace(r.Stack))
	return b.String()
}

// pruneReports keeps at most maxReports reports, removing the oldest first.
func pruneReports(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".txt" {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	if len(files) <= maxReports {
		return
	}
	// Sort by name; the crash-<timestamp>.txt naming sorts chronologically.
	sort.Strings(files)
	for _, f := range files[:len(files)-maxReports] {
		_ = os.Remove(f)
	}
}

// CaptureGoroutines returns a dump of all goroutines' stacks. Useful for tests
// and for attaching diagnostics to other error paths. Never panics.
func CaptureGoroutines() []byte {
	buf := make([]byte, 1<<20)    // 1 MB cap
	n := runtime.Stack(buf, true) // all goroutines
	if n < 0 || n > len(buf) {
		return nil
	}
	return buf[:n]
}
