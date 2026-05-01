package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// detectTerminal returns the terminal emulator type based on environment
// variables. Possible return values: "iterm2", "kitty", "ghostty",
// "apple", "generic".
func detectTerminal() string {
	termProgram := os.Getenv("TERM_PROGRAM")
	switch strings.ToLower(termProgram) {
	case "iterm.app":
		return "iterm2"
	case "apple_terminal":
		return "apple"
	}

	if os.Getenv("KITTY_PID") != "" || os.Getenv("KITTY_WINDOW_ID") != "" {
		return "kitty"
	}

	if os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return "ghostty"
	}

	if runtime.GOOS == "darwin" && termProgram == "" {
		return "apple"
	}

	return "generic"
}

// sendTerminalNotification sends a notification appropriate for the detected
// terminal emulator. iTerm2 uses OSC 9, Kitty uses OSC 99, Ghostty uses
// OSC 777, Apple Terminal uses osascript, and generic falls back to BEL.
func sendTerminalNotification(title, body string) {
	switch detectTerminal() {
	case "iterm2":
		// iTerm2 OSC 9 notification
		fmt.Fprintf(os.Stderr, "\033]9;%s\007", body)
	case "kitty":
		// Kitty OSC 99 notification
		fmt.Fprintf(os.Stderr, "\033]99;i=hawk:d=0;%s\033\\", body)
	case "ghostty":
		// Ghostty OSC 777 notification
		fmt.Fprintf(os.Stderr, "\033]777;notify;%s;%s\033\\", title, body)
	case "apple":
		if runtime.GOOS == "darwin" {
			script := fmt.Sprintf(`display notification "%s" with title "%s"`, body, title)
			_ = exec.Command("osascript", "-e", script).Start()
		}
	default:
		// Generic: BEL character
		fmt.Fprint(os.Stderr, "\a")
	}
}
