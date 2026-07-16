// capabilities.go — Terminal capability detection.
//
// Detects terminal features like truecolor support, 256-color mode, etc.

package theme

import (
	"os"
	"strings"
)

// TerminalCapabilities holds detected terminal features.
type TerminalCapabilities struct {
	ColorLevel    ColorLevel
	HasDarkBG     bool
	HasOSC12      bool // cursor color support
	HasOSC112     bool // cursor reset support
	HasAltScreen  bool
	HasMouse      bool
	HasBracketedPaste bool
}

// DetectTerminalCapabilities probes the terminal for supported features.
func DetectTerminalCapabilities() TerminalCapabilities {
	return TerminalCapabilities{
		ColorLevel:     DetectColorLevel(),
		HasDarkBG:      detectDarkBackground(),
		HasOSC12:       detectOSC12(),
		HasOSC112:      detectOSC112(),
		HasAltScreen:   detectAltScreen(),
		HasMouse:       detectMouseSupport(),
		HasBracketedPaste: detectBracketedPaste(),
	}
}

func detectDarkBackground() bool {
	// Check COLORTERM for background detection hints
	if ct := os.Getenv("COLORTERM"); strings.Contains(strings.ToLower(ct), "dark") {
		return true
	}
	// Check for explicit background color env
	if bg := os.Getenv("TERMINAL_BACKGROUND"); strings.ToLower(bg) == "dark" {
		return true
	}
	// Default to true for Hawk (dark theme is default)
	return true
}

func detectOSC12() bool {
	// Most modern terminals support OSC 12
	return true
}

func detectOSC112() bool {
	// Most modern terminals support OSC 112 (cursor reset)
	return true
}

func detectAltScreen() bool {
	// Check for alt screen disable
	if os.Getenv("TERM_ALT_SCREEN") == "0" {
		return false
	}
	// Most terminals support alt screen
	return true
}

func detectMouseSupport() bool {
	// Check for NO_MOUSE or similar override
	return os.Getenv("TERM_MOUSE") != "0"
}

func detectBracketedPaste() bool {
	// Most modern terminals support bracketed paste
	return true
}