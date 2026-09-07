package cmd

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

// CLIProgress renders a ProgressTracker as live single-line progress for
// non-TUI commands. On a terminal it animates the active step in place with
// the spinner wave; when stdout is piped or CI it emits one clean static line
// per completed step so output stays parseable. Glyphs come from
// internal/ui/icons so the cmd/ no-emoji audit holds.
type CLIProgress struct {
	w       io.Writer
	pt      *ProgressTracker
	spinner *BrailleSpinner
	tty     bool
}

// NewCLIProgress builds a tracker for title with the given steps, writing to
// stdout and animating only when stdout is a terminal.
func NewCLIProgress(title string, steps []string) *CLIProgress {
	return newCLIProgress(title, steps, os.Stdout, stdoutIsTerminal())
}

// newCLIProgress is the testable core: writer and tty are injected.
func newCLIProgress(title string, steps []string, w io.Writer, tty bool) *CLIProgress {
	pt := NewProgressTracker(title)
	for _, s := range steps {
		pt.AddStep(s)
	}
	return &CLIProgress{w: w, pt: pt, spinner: NewBrailleSpinner(SpinnerGraycode, ""), tty: tty}
}

// StartStep marks step i active and, on a TTY, starts animating it in place.
// A fresh BrailleSpinner is created per step because the spinner is one-shot:
// its stop channel is closed on Stop() and cannot be restarted.
func (c *CLIProgress) StartStep(i int) {
	c.pt.StartStep(i)
	if IsQuiet() || !c.tty || i < 0 || i >= len(c.pt.Steps) {
		return
	}
	c.spinner = NewBrailleSpinner(SpinnerGraycode, c.pt.Steps[i].Name)
	c.spinner.Start(80*time.Millisecond, func(frame string) {
		eta := ""
		if remaining := c.pt.EstimateRemaining(); remaining > 0 {
			eta = fmt.Sprintf(" · ETA %s", formatDurationShort(remaining))
		}
		name := c.tint(c.pt.Steps[i].Name, textPrimary)
		_, _ = fmt.Fprintf(c.w, "\r%s %s %s %d/%d%s\033[K", frame, c.bar(), name, i+1, len(c.pt.Steps), eta)
	})
}

// CompleteStep finalizes step i with its duration and prints a clean line.
func (c *CLIProgress) CompleteStep(i int) {
	c.spinner.Stop()
	c.pt.CompleteStep(i)
	if IsQuiet() || i < 0 || i >= len(c.pt.Steps) {
		return
	}
	s := c.pt.Steps[i]
	c.writeLine(fmt.Sprintf("%s %s (%s)",
		c.tint(icons.CheckBold(), doneGreen),
		c.tint(s.Name, textPrimary),
		c.tint(formatDurationShort(s.Duration), textMuted)))
}

// FailStep finalizes step i as failed with a reason.
func (c *CLIProgress) FailStep(i int, reason string) {
	c.spinner.Stop()
	c.pt.FailStep(i, reason)
	if IsQuiet() || i < 0 || i >= len(c.pt.Steps) {
		return
	}
	s := c.pt.Steps[i]
	c.writeLine(fmt.Sprintf("%s %s (%s) : %s",
		c.tint(icons.CloseThick(), errorCoral),
		c.tint(s.Name, textPrimary),
		c.tint(formatDurationShort(s.Duration), textMuted),
		c.tint(reason, errorCoral)))
}

// tint applies a theme foreground color when color output is appropriate
// (honors --quiet, NO_COLOR, FORCE_COLOR, and TTY state via ShouldColor).
// Piped/CI output stays plain so scripts never see stray ANSI escapes.
func (c *CLIProgress) tint(s string, color color.Color) string {
	if !ShouldColor() || s == "" {
		return s
	}
	return lipgloss.NewStyle().Foreground(color).Render(s)
}

// bar renders the overall progress as a compact theme-colored bar (filled =
// successTeal, empty = borderDim). Block glyphs (U+2588/U+2591) are in the
// range the cmd/ no-emoji audit permits, matching ProgressTracker.Render.
func (c *CLIProgress) bar() string {
	pct := c.pt.overallProgress()
	const width = 12
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	filledStr := strings.Repeat("█", filled)
	emptyStr := strings.Repeat("░", width-filled)
	return c.tint(filledStr, successTeal) + c.tint(emptyStr, borderDim)
}

// Done stops any animation and prints a themed completion summary with the
// final progress bar. Uses errorCoral when any step failed.
func (c *CLIProgress) Done() {
	c.spinner.Stop()
	if IsQuiet() {
		return
	}
	elapsed := c.pt.GetElapsed()
	failures := 0
	for _, s := range c.pt.Steps {
		if s.Status == "failed" {
			failures++
		}
	}
	mark := icons.CheckBold()
	markColor := doneGreen
	verb := "complete"
	if failures > 0 {
		mark = icons.CloseThick()
		markColor = errorCoral
		verb = "finished"
	}
	line := fmt.Sprintf("%s %s %s in %s",
		c.tint(mark, markColor),
		c.tint(c.pt.Title, textPrimary),
		verb,
		c.tint(formatDurationShort(elapsed), textMuted))
	if failures > 0 {
		line += c.tint(fmt.Sprintf(" (%d failed)", failures), errorCoral)
	}
	c.writeLine(fmt.Sprintf("%s %s", c.bar(), line))
}

// Abort stops any running animation without printing a completion line. Safe
// to call from deferred error paths so a spinner never leaks past a return.
func (c *CLIProgress) Abort() {
	c.spinner.Stop()
}

func (c *CLIProgress) writeLine(line string) {
	if c.tty {
		_, _ = fmt.Fprintf(c.w, "\r%s\033[K\n", line)
		return
	}
	_, _ = fmt.Fprintln(c.w, line)
}
