package cmd

import (
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/GrayCodeAI/hawk/internal/terminal/tape"
)

// recordPath is set by --record; when non-empty, interactive REPL output is
// captured to an fxtape file.
var recordPath string

// replOut is where interactive REPL output is written. It defaults to
// os.Stdout and is swapped for a tape Recorder while --record is active so the
// live stream is both shown and captured.
var replOut io.Writer = os.Stdout

// startRecording begins capturing interactive REPL output to path as an
// fxtape (fx `--record` parity): stdout bytes are recorded as frames along
// with terminal resize events. It returns a cleanup function that restores the
// default writer and closes the tape.
func startRecording(path string) (func(), error) {
	w, h := TermSize()
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	rec, err := tape.NewRecorder(f, replOut, uint16(w), uint16(h), nil)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	prev := replOut
	replOut = rec

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			nw, nh := TermSize()
			_ = rec.Resize(uint16(nw), uint16(nh))
		}
	}()

	return func() {
		signal.Stop(sigCh)
		replOut = prev
		_ = rec.Close()
		_ = f.Close()
	}, nil
}
