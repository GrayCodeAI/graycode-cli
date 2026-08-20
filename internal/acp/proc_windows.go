//go:build windows

package acp

import (
	"os"
	"os/exec"
)

func setCmdProcessGroup(cmd *exec.Cmd) {
	// Process groups on windows handled by default or JobObjects
}

func killProcessGroup(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	return proc.Kill()
}
