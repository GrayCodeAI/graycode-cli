package cmd

import (
	"context"
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

	msg := "Graycode query completed"

	switch runtime.GOOS {
	case "darwin":
		// macOS: use osascript for native notification
		_ = exec.CommandContext(
			context.Background(),
			"osascript", "-e",
			`display notification "`+msg+`" with title "Graycode"`,
		).Start()
	case "linux":
		// Linux: use notify-send if available
		_ = exec.CommandContext(context.Background(), "notify-send", "Graycode", msg).Start()
	}
}
