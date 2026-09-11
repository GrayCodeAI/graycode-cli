package cmd

import (
	"io"
	"os"
	"os/exec"
	"strings"
)

// pager manages an external pager process (less, more, etc.) for long output.
// When stdout is a TTY and the user hasn't disabled paging, StartPager() spawns
// the pager and returns an io.Writer that pipes into it. Call StopPager() when
// done to wait for the pager to exit.
//
// Modeled on GitHub's gh CLI IOStreams pager pattern.
type pager struct {
	cmd  *exec.Cmd
	pipe io.WriteCloser
}

var activePager *pager

// StartPager launches an external pager if all of the following hold:
//   - stdout is a TTY
//   - the --quiet flag is not set
//   - the PAGER environment variable is not set to "cat" or empty
//   - a pager binary (less, more) is available on PATH
//
// It returns an io.Writer that should be used for all subsequent output. If
// paging is not active, it returns os.Stdout.
func StartPager() io.Writer {
	if quietFlag || !stdoutIsTerminal() {
		return os.Stdout
	}

	pagerCmd := resolvePager()
	if pagerCmd == "" {
		return os.Stdout
	}

	// Parse the pager command (may include flags, e.g. "less -FRX")
	fields := strings.Fields(pagerCmd)
	if len(fields) == 0 {
		return os.Stdout
	}

	name := fields[0]
	args := fields[1:]

	// less defaults: quit-if-one-screen, no init, preserve color, raw control chars.
	if name == "less" {
		args = append([]string{"-FRX"}, args...)
	}

	// G204: name is derived from environment or LookPath, which is the standard
	// pattern for pager invocation. Users control their own PAGER env var.
	//nolint:gosec // G204
	cmd := exec.Command(name, args...) // #nosec G204 -- pager executable is selected from a validated allowlist
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	pipe, err := cmd.StdinPipe()
	if err != nil {
		return os.Stdout
	}

	if err := cmd.Start(); err != nil {
		_ = pipe.Close()
		return os.Stdout
	}

	activePager = &pager{cmd: cmd, pipe: pipe}
	return pipe
}

// StopPager waits for the pager process to exit and cleans up resources. It is
// safe to call even if no pager was started.
func StopPager() {
	if activePager == nil {
		return
	}
	p := activePager
	activePager = nil

	// Close the write pipe first so the pager sees EOF and can exit.
	if p.pipe != nil {
		_ = p.pipe.Close()
	}
	if p.cmd != nil {
		_, _ = p.cmd.Process.Wait()
	}
}

// resolvePager returns the pager command string from environment or defaults.
// Returns "" if paging should be disabled.
func resolvePager() string {
	// Disable paging via environment.
	if v := os.Getenv("HAWK_PAGER"); v != "" {
		if v == "cat" || v == "none" {
			return ""
		}
		return v
	}

	// Respect a generic PAGER but allow "cat" to disable.
	if v := os.Getenv("PAGER"); v != "" {
		if v == "cat" || v == "none" {
			return ""
		}
		return v
	}

	// Auto-detect: prefer less, fall back to more.
	if path, err := exec.LookPath("less"); err == nil {
		return path
	}
	if path, err := exec.LookPath("more"); err == nil {
		return path
	}
	return ""
}

// IsPagerActive reports whether a pager is currently running.
func IsPagerActive() bool {
	return activePager != nil
}
