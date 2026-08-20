//go:build !windows

package terminal

import (
	"os"
	"syscall"
)

func killProcessGroup(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	return syscall.Kill(-proc.Pid, syscall.SIGKILL)
}
