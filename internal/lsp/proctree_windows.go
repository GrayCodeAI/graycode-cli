//go:build windows

package lsp

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func prepareCmdSysProcAttr(cmd *exec.Cmd) {}

func killProcessTreePlatform(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	// taskkill /T (tree) /F (force) /PID <pid>
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	_ = cmd.Process.Kill()
	return nil
}

func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = process
	out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/NH").Output()
	if err != nil {
		return false
	}
	return len(out) > 0 && !strings.Contains(string(out), "No tasks are running")
}
