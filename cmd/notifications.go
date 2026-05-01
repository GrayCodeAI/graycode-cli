package cmd

import (
	"os/exec"
	"runtime"
	"time"
)

// notifyCompletion plays a sound or sends a system notification when a long query completes.
// Only triggers if the operation took longer than the threshold.
func notifyCompletion(duration time.Duration) {
	const threshold = 30 * time.Second
	if duration < threshold {
		return
	}

	msg := "Hawk query completed"

	switch runtime.GOOS {
	case "darwin":
		// macOS: use osascript for native notification
		_ = exec.Command("osascript", "-e",
			`display notification "`+msg+`" with title "Hawk"`,
		).Start()
	case "linux":
		// Linux: use notify-send if available
		_ = exec.Command("notify-send", "Hawk", msg).Start()
	}
}
