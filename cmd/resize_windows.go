//go:build windows

package cmd

import "github.com/GrayCodeAI/hawk/internal/terminal/tape"

// watchTerminalResize is a no-op on Windows, where SIGWINCH is not defined.
func watchTerminalResize(rec *tape.Recorder) func() {
	return func() {}
}
