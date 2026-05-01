package cmd

import (
	"runtime"
	"testing"
)

func TestPreventSleep_ReturnsCancel(t *testing.T) {
	cancel := preventSleep()
	if cancel == nil {
		t.Fatal("preventSleep should return a non-nil cancel function")
	}
	// Calling cancel should not panic.
	cancel()
}

func TestPreventSleep_CancelIdempotent(t *testing.T) {
	cancel := preventSleep()
	cancel()
	cancel() // second call should not panic
}

func TestPreventSleep_Platform(t *testing.T) {
	cancel := preventSleep()
	defer cancel()
	// On any platform, we should get a valid cancel function.
	if runtime.GOOS != "darwin" {
		// Non-macOS: should be a no-op cancel
		t.Log("non-darwin platform, cancel is no-op")
	}
}
