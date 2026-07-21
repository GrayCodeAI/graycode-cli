//go:build windows

package tool

import (
	"os"
	"os/exec"
)

func setCmdProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(proc *os.Process) error {
	return proc.Kill()
}
