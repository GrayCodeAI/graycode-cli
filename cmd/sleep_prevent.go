package cmd

import (
	"context"
	"os/exec"
	"runtime"
)

// preventSleep starts caffeinate on macOS to keep the system awake during long
// operations. It returns a cancel function that stops the background process.
// On non-macOS systems it is a no-op.
func preventSleep() func() {
	switch runtime.GOOS {
	case "darwin":
		return preventSleepMacOS()
	case "linux":
		return preventSleepLinux()
	default:
		return func() {}
	}
}

func preventSleepMacOS() func() {
	cmd := exec.CommandContext(context.Background(), "caffeinate", "-i")
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

func preventSleepLinux() func() {
	// systemd-inhibit blocks sleep/idle inhibitors for the lifetime of its
	// child process. Using --mode=block ensures the inhibitor is held until
	// we explicitly kill the process. We wrap `sleep` as the child; it
	// blocks without consuming CPU. The cancel function kills the child to
	// release the inhibitor immediately when the turn ends.
	cmd := exec.CommandContext(context.Background(),
		"systemd-inhibit",
		"--mode=block",
		"--what=idle:sleep",
		"--who=graycode",
		"--why=Agent turn in progress",
		"sleep", "86400")
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
