//go:build !windows

package tool

import (
	"os"
	"os/exec"
	"syscall"
)

func setCmdProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(proc *os.Process) error {
	return syscall.Kill(-proc.Pid, syscall.SIGKILL)
}
