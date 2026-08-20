package lsp

import (
	"os/exec"
	"time"
)

// KillProcessTree terminates the command process and all its descendants.
func KillProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := killProcessTreePlatform(cmd)
	go func() {
		_ = cmd.Wait()
	}()
	return err
}

// WaitForProcessQuiescence polls until the process exits or timeout expires.
func WaitForProcessQuiescence(pid int, timeout time.Duration) bool {
	if pid <= 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return !isProcessAlive(pid)
}
