package cmd

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// tabProgress manages the terminal tab progress bar via OSC 9;4 escape
// sequences. Supported by iTerm2, Kitty, Ghostty, and WezTerm. Other
// terminals silently ignore the escapes. All methods are no-ops when
// stdout is not a terminal or progress is disabled.
var tabProgress struct {
	mu        sync.Mutex
	enabled   bool
	showing   bool // whether the bar is currently visible
	lastValue int  // -1 = indeterminate, 0-100 = percentage
}

// EnableTabProgress enables OSC 9;4 tab progress bar updates. Called at
// startup when the terminal is a TUI session.
func EnableTabProgress() {
	tabProgress.mu.Lock()
	defer tabProgress.mu.Unlock()
	tabProgress.enabled = stdoutIsTerminal()
	tabProgress.lastValue = -1
}

// SetTabProgress updates the terminal tab progress bar to the given
// percentage (0-100). Values outside this range clamp. A value of -1
// shows an indeterminate spinner. This is a no-op when progress is disabled.
func SetTabProgress(value int) {
	tabProgress.mu.Lock()
	defer tabProgress.mu.Unlock()
	if !tabProgress.enabled {
		return
	}
	if value < -1 {
		value = -1
	}
	if value > 100 {
		value = 100
	}
	if value == tabProgress.lastValue {
		return
	}
	tabProgress.lastValue = value
	tabProgress.showing = true
	writeTabProgress(value)
}

// ClearTabProgress removes the progress bar from the terminal tab.
// Should be called when a long-running operation completes.
func ClearTabProgress() {
	tabProgress.mu.Lock()
	defer tabProgress.mu.Unlock()
	if !tabProgress.enabled || !tabProgress.showing {
		return
	}
	tabProgress.showing = false
	tabProgress.lastValue = -2 // reset to "unset"
	writeTabProgressClear()
}

// writeTabProgress emits the OSC 9;4 escape sequence for the given value.
// Caller must hold tabProgress.mu.
func writeTabProgress(value int) {
	if value < 0 {
		// Indeterminate: OSC 9;4 ; -1 ; BEL
		_, _ = io.WriteString(os.Stdout, "\033]9;4;-1;\a")
	} else {
		// Percentage: OSC 9;4 ; <percent> ; BEL
		_, _ = fmt.Fprintf(os.Stdout, "\033]9;4;%d;\a", value)
	}
}

// writeTabProgressClear emits the OSC 9;4 escape sequence to clear the bar.
// Caller must hold tabProgress.mu.
func writeTabProgressClear() {
	// OSC 9;4 ; 0 ; BEL clears the progress bar.
	_, _ = io.WriteString(os.Stdout, "\033]9;4;0;\a")
}
