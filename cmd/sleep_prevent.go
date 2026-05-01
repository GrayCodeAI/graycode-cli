package cmd

import (
	"os/exec"
	"runtime"
)

// preventSleep starts caffeinate on macOS to keep the system awake during long
// operations. It returns a cancel function that stops the background process.
// On non-macOS systems it is a no-op.
func preventSleep() func() {
	if runtime.GOOS != "darwin" {
		return func() {}
	}

	cmd := exec.Command("caffeinate", "-i")
	if err := cmd.Start(); err != nil {
		return func() {}
	}

	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}
}
