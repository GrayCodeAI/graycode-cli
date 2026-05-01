package cmd

import (
	"os"
	"testing"
)

func TestDetectTerminal_Default(t *testing.T) {
	// Save and clear environment
	origTP := os.Getenv("TERM_PROGRAM")
	origKitty := os.Getenv("KITTY_PID")
	origGhostty := os.Getenv("GHOSTTY_RESOURCES_DIR")
	os.Unsetenv("TERM_PROGRAM")
	os.Unsetenv("KITTY_PID")
	os.Unsetenv("KITTY_WINDOW_ID")
	os.Unsetenv("GHOSTTY_RESOURCES_DIR")
	defer func() {
		setEnvIfNonEmpty("TERM_PROGRAM", origTP)
		setEnvIfNonEmpty("KITTY_PID", origKitty)
		setEnvIfNonEmpty("GHOSTTY_RESOURCES_DIR", origGhostty)
	}()

	term := detectTerminal()
	// On macOS without TERM_PROGRAM set, we get "apple"; otherwise "generic"
	if term != "generic" && term != "apple" {
		t.Fatalf("expected generic or apple, got %s", term)
	}
}

func TestDetectTerminal_ITerm2(t *testing.T) {
	orig := os.Getenv("TERM_PROGRAM")
	os.Setenv("TERM_PROGRAM", "iTerm.app")
	defer setEnvIfNonEmpty("TERM_PROGRAM", orig)

	if got := detectTerminal(); got != "iterm2" {
		t.Fatalf("expected iterm2, got %s", got)
	}
}

func TestDetectTerminal_Kitty(t *testing.T) {
	origTP := os.Getenv("TERM_PROGRAM")
	origKitty := os.Getenv("KITTY_PID")
	os.Unsetenv("TERM_PROGRAM")
	os.Setenv("KITTY_PID", "12345")
	defer func() {
		setEnvIfNonEmpty("TERM_PROGRAM", origTP)
		setEnvIfNonEmpty("KITTY_PID", origKitty)
	}()

	if got := detectTerminal(); got != "kitty" {
		t.Fatalf("expected kitty, got %s", got)
	}
}

func TestSendTerminalNotification_NoPanic(t *testing.T) {
	// Just verify it does not panic on any terminal type.
	sendTerminalNotification("Test", "Hello")
}

func setEnvIfNonEmpty(key, value string) {
	if value != "" {
		os.Setenv(key, value)
	} else {
		os.Unsetenv(key)
	}
}
