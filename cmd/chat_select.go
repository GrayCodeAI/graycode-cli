package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// selectionResumedMsg is delivered to the chat model after the user finishes a
// native terminal selection and presses a key to return to the TUI.
type selectionResumedMsg struct{}

// enterSelectionMode temporarily releases the terminal so the user can use
// their terminal emulator's native text selection (click-and-drag, etc.) to
// copy text from the chat. The TUI is paused: alt screen is exited, mouse
// tracking is suspended, and the program's input reader is cancelled. Any
// keypress restores the TUI.
//
// This exists because tea.WithAltScreen + tea.WithMouseCellMotion together
// disable native text selection in most terminals (Terminal.app, iTerm2,
// Ghostty, WezTerm, etc.) — mouse events are routed to the app, so the
// terminal never sees a click-and-drag gesture. Releasing the terminal is
// the standard TUI workaround (same approach used by btop, lazygit, fzf).
//
// transcript is printed to stdout after the alt screen is released. Without
// that dump the chat vanishes from view and there is nothing to select.
func enterSelectionMode(ref *progRef, transcript string, restoreMouse bool) tea.Cmd {
	if ref == nil {
		return nil
	}
	ref.mu.Lock()
	p := ref.p
	ref.mu.Unlock()
	if p == nil {
		return nil
	}
	return func() tea.Msg {
		_ = p.ReleaseTerminal()
		writeTerminalMouse(disableMouseCSI)
		if strings.TrimSpace(transcript) != "" {
			fmt.Print(transcript)
			if !strings.HasSuffix(transcript, "\n") {
				fmt.Println()
			}
			fmt.Println()
		}
		// Banner on stderr so it doesn't get clobbered by the program repaint.
		// Use plain ASCII so it renders identically in every terminal.
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "── SELECT MODE ─────────────────────────────────────────────")
		fmt.Fprintln(os.Stderr, "  Click and drag to select text in this terminal.")
		fmt.Fprintln(os.Stderr, "  Copy with your terminal's normal copy shortcut (e.g. Cmd+C,")
		fmt.Fprintln(os.Stderr, "  Ctrl+Shift+C, or Ctrl+Insert).")
		fmt.Fprintln(os.Stderr, "  Press any key to return to hawk.")
		fmt.Fprintln(os.Stderr, "────────────────────────────────────────────────────────────")
		fmt.Fprintln(os.Stderr, "")
		// Block on stdin in raw mode so any single keypress resumes the
		// TUI. The TUI's input reader has been cancelled by
		// ReleaseTerminal so this read will not race with it.
		restore, _ := makeStdinRaw()
		buf := make([]byte, 1)
		_, _ = os.Stdin.Read(buf)
		if restore != nil {
			restore()
		}
		_ = p.RestoreTerminal()
		syncTerminalMouse(restoreMouse)
		// Give the terminal a beat to finish restoring state before we
		// start firing events at the program; without this the first
		// post-resume keystroke can land in the still-restoring tty.
		time.Sleep(40 * time.Millisecond)
		return selectionResumedMsg{}
	}
}

// makeStdinRaw switches stdin to raw mode (no echo, no line buffering) and
// returns a restore function that puts it back. On terminals where raw mode
// is unsupported, restore is nil and the call is a no-op.
func makeStdinRaw() (func(), error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, nil
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return func() { _ = term.Restore(fd, old) }, nil
}

// plainTranscript renders chat messages as plain text for clipboard export and
// native terminal selection. ANSI styling is omitted so copy/paste stays clean.
func plainTranscript(messages []displayMsg, partial string) string {
	var b strings.Builder
	for _, msg := range messages {
		line, ok := plainTranscriptLine(msg)
		if !ok {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n\n")
	}
	if partial != "" {
		b.WriteString("hawk: ")
		b.WriteString(partial)
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func plainTranscriptLine(msg displayMsg) (string, bool) {
	content := strings.TrimSpace(msg.content)
	if content == "" {
		return "", false
	}
	switch msg.role {
	case "welcome", "usage", "setup_complete":
		return "", false
	case "user":
		return "You: " + content, true
	case "assistant":
		return "hawk: " + content, true
	case "error":
		return "error: " + content, true
	case "system":
		return content, true
	case "thinking":
		return "thinking: " + content, true
	case "tool_use":
		return "tool: " + content, true
	case "tool_result":
		return content, true
	case "permission":
		return "permission: " + content, true
	case "question":
		return content, true
	default:
		return content, true
	}
}
