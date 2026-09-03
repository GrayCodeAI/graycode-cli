//go:build !windows

package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/GrayCodeAI/graycode-cli/internal/terminal/tape"
)

// watchTerminalResize records terminal resize (SIGWINCH) events into the tape
// recorder while --record is active. It returns a stop function that
// deregisters the signal handler.
func watchTerminalResize(rec *tape.Recorder) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			w, h := TermSize()
			_ = rec.Resize(uint16(w), uint16(h))
		}
	}()
	return func() {
		signal.Stop(sigCh)
	}
}
