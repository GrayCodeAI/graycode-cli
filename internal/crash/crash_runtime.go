//go:build go1.23
// +build go1.23

package crash

import (
	"os"
	"path/filepath"
	"runtime/debug"
)

// installRuntimeCrashOutput routes fatal runtime crash output (e.g. fatal
// error: concurrent map writes, out of memory) to the crash dir via
// runtime/debug.SetCrashOutput (Go 1.23+). Only a single additional crash
// output file is supported by the runtime, so this writes the latest crash
// output to a fixed "runtime-crash.log" in the crash dir. Caller-side
// pruning (pruneReports) then caps retained files.
//
// Guarded to go1.23+; crash_runtime_stub.go builds the no-op on older Go.
func installRuntimeCrashOutput() {
	dir, err := reportDir()
	if err != nil {
		return
	}
	path := filepath.Join(dir, "runtime-crash.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- path is the private runtime crash report path
	if err != nil {
		return
	}
	// SetCrashOutput duplicates f's fd; closing our copy here is safe and lets
	// us prune by name later. The runtime keeps its own reference.
	defer func() { _ = f.Close() }()
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		return
	}
}
