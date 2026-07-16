//go:build windows
// +build windows

package crash

// installSignalHandlers is a no-op on Windows. The POSIX SIGQUIT/SIGTERM dump
// flow does not translate (Windows has no Unix signals; it uses structured
// exception handling and SetUnhandledExceptionFilter, which we avoid pulling
// into this stdlib-only package). The panic-recover path and the
// runtime.SetCrashOutput sink (crash_runtime.go) still apply on Windows.
func installSignalHandlers() {}
