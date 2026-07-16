//go:build !go1.23
// +build !go1.23

package crash

// installRuntimeCrashOutput is a no-op on Go versions older than 1.23, where
// runtime/debug.SetCrashOutput does not exist. The crash handler still works
// via the panic-recover path and the POSIX signal handlers; only the
// runtime-triggered crash sink (e.g. fatal error: concurrent map writes) is
// unavailable. crash_runtime.go (go1.23+) wires the full implementation.
func installRuntimeCrashOutput() {}
