//go:build !windows

package acp

import (
	"os"
	"os/exec"
	"syscall"
)

func setCmdProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	return syscall.Kill(-proc.Pid, syscall.SIGKILL)
}
