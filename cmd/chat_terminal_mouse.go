package cmd

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// ANSI sequences to turn off xterm SGR/cell mouse tracking. Cursor's integrated
// terminal can leave these modes enabled after a prior TUI session, which causes
// scroll events to arrive as literal "[<65;99;16M" KeyRunes in the input.
const (
	disableMouseCSI = "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l"
	enableMouseCSI  = "\x1b[?1006h\x1b[?1002h"
)

func writeTerminalMouse(mode string) {
	_, _ = os.Stdout.WriteString(mode)
}

func syncTerminalMouse(enabled bool) {
	if enabled {
		writeTerminalMouse(enableMouseCSI)
	} else {
		writeTerminalMouse(disableMouseCSI)
	}
}

func initTerminalMouseCmd(enabled bool) tea.Cmd {
	return func() tea.Msg {
		syncTerminalMouse(enabled)
		return nil
	}
}
