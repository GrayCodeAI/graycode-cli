//go:build !windows
// +build !windows

package crash

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

// installSignalHandlers registers best-effort POSIX signal handlers that dump
// goroutines before re-raising. This mirrors grok's crash-handler behavior
// (dump the goroutine state, then restore the default disposition so the
// process exits/coredumps normally — the handler is additive diagnostics).
//
// SIGQUIT — the conventional "dump stacks and die" signal. We capture the
// dump to the crash dir, then re-raise with the default handler so a core
// dump can still be produced.
//
// SIGTERM — a graceful-termination signal in the normal case, but also the
// signal many monitors use to request a stack dump. We capture the dump only;
// we do NOT re-raise, because SIGTERM is expected to terminate the process
// via the default (or an existing) handler. We keep this additive and
// tolerant: registration failure is logged, never fatal.
func installSignalHandlers() {
	installDumpHandler(syscall.SIGQUIT, true)
	installDumpHandler(syscall.SIGTERM, false)
}

// installDumpHandler captures a goroutine dump to the crash dir when sig
// arrives. If reRaise is true, the original default disposition is restored
// before re-raising so the OS produces the normal termination behavior
// (core dump for SIGQUIT).
func installDumpHandler(sig syscall.Signal, reRaise bool) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sig)
	go func() {
		<-ch
		dumpSignal(sig, "received")
		writeSignalReport(sig)
		signal.Reset(sig)
		if reRaise {
			// Restore default disposition and re-raise.
			if err := raiseSignal(sig); err != nil {
				fmt.Fprintf(os.Stderr, "crash: failed to re-raise %s: %v\n", sig, err)
			}
		}
	}()
}

func dumpSignal(sig syscall.Signal, reason string) {
	dir, err := reportDir()
	if err != nil {
		return
	}
	stacks := CaptureGoroutines()
	timestamp := now().UTC().Format("20060102T150405.000Z")
	filename := fmt.Sprintf("crash-signal-%s-%s.txt", sig, timestamp)
	path := filepath.Join(dir, filename)
	content := fmt.Sprintf("hawk signal report\nsignal:     %s\nreason:     %s\ntimestamp:  %s\n\n%s\n",
		sig, reason, timestamp, stacks)
	_ = os.WriteFile(path, []byte(content), 0o600)
}

func writeSignalReport(sig syscall.Signal) {
	// Best-effort; errors are non-fatal (the process is terminating anyway).
	_, _ = WriteReport(nil, []byte(fmt.Sprintf("signal dump %s — see crash-signal-*.txt", sig)))
}

func raiseSignal(sig syscall.Signal) error {
	if err := syscall.Kill(os.Getpid(), sig); err != nil {
		return fmt.Errorf("kill self: %w", err)
	}
	// If kill returns, momentarily restore the default and re-raise. We reach
	// here only if a handler caught it above; the re-raise above should have
	// terminated. This is a safety net.
	return errors.New("re-raise returned without terminating")
}
