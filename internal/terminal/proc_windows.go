//go:build windows

package terminal

import (
	"os"
)

func killProcessGroup(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	return proc.Kill()
}
