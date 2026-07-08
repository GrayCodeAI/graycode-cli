// Package sessioncapture provides terminal context capture with delta-based tracking.
// Inspired by lacy's context.sh — only sends what changed since the last query.
package sessioncapture

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// TerminalContext captures and tracks terminal state changes between queries.
// Only includes what changed since the last query to minimize token usage.
type TerminalContext struct {
	mu sync.Mutex

	lastCWD       string
	lastGitBranch string
	lastExitCode  int
	cmdBuffer     []string
	maxCmds       int
	hadRealCmd    bool

	outputEnabled  bool
	outputMaxLines int
	captureCmd     string // detected at init
}

// NewTerminalContext creates a new delta-tracking terminal context.
func NewTerminalContext() *TerminalContext {
	tc := &TerminalContext{
		maxCmds:        10,
		outputEnabled:  true,
		outputMaxLines: 50,
	}
	tc.captureCmd = detectTerminalCapture()
	return tc
}

// MarkCommand records a shell command that was executed.
func (tc *TerminalContext) MarkCommand(cmd string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	tc.cmdBuffer = append(tc.cmdBuffer, cmd)
	if len(tc.cmdBuffer) > tc.maxCmds {
		tc.cmdBuffer = tc.cmdBuffer[len(tc.cmdBuffer)-tc.maxCmds:]
	}
	tc.hadRealCmd = true
}

// MarkExitCode records the exit code of the last command.
func (tc *TerminalContext) MarkExitCode(code int) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.lastExitCode = code
}

// BuildContext returns a delta-based context string and resets tracking state.
func (tc *TerminalContext) BuildContext(query string) string {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	var parts []string

	// CWD delta
	cwd, _ := os.Getwd()
	if cwd != tc.lastCWD {
		parts = append(parts, fmt.Sprintf("[cwd: %s]", cwd))
		tc.lastCWD = cwd
	}

	// Git branch delta
	branch := currentGitBranch()
	if branch != "" && branch != tc.lastGitBranch {
		parts = append(parts, fmt.Sprintf("[git: %s]", branch))
		tc.lastGitBranch = branch
	}

	// Exit code (only if non-zero and a real command ran)
	if tc.lastExitCode != 0 && tc.hadRealCmd {
		parts = append(parts, fmt.Sprintf("[exit: %d]", tc.lastExitCode))
	}

	// Recent commands
	if len(tc.cmdBuffer) > 0 {
		parts = append(parts, fmt.Sprintf("[recent: %s]", strings.Join(tc.cmdBuffer, " | ")))
	}

	// Terminal output capture
	if tc.outputEnabled && tc.captureCmd != "" && tc.hadRealCmd {
		if output := tc.captureTerminalOutput(); output != "" {
			parts = append(parts, fmt.Sprintf("[terminal-output]\n%s\n[/terminal-output]", output))
		}
	}

	// Reset state
	tc.cmdBuffer = nil
	tc.lastExitCode = 0
	tc.hadRealCmd = false

	if len(parts) == 0 {
		return query
	}
	return strings.Join(parts, " ") + "\n" + query
}

// Reset clears all state (e.g., on /new session).
func (tc *TerminalContext) Reset() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.lastCWD = ""
	tc.lastGitBranch = ""
	tc.lastExitCode = 0
	tc.cmdBuffer = nil
	tc.hadRealCmd = false
}

// captureTerminalOutput grabs visible terminal content via detected method.
func (tc *TerminalContext) captureTerminalOutput() string {
	if tc.captureCmd == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "sh", "-c", tc.captureCmd).Output() // #nosec G204 -- captureCmd is one of a fixed set of internally-detected commands (tmux/screen/osascript), not external input
	if err != nil {
		return ""
	}
	lines := strings.Split(stripANSI(string(out)), "\n")
	if len(lines) > tc.outputMaxLines {
		lines = lines[len(lines)-tc.outputMaxLines:]
	}
	// Trim trailing empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// detectTerminalCapture checks for tmux, screen, or macOS terminal APIs.
func detectTerminalCapture() string {
	// tmux takes priority (works inside terminal emulators)
	if os.Getenv("TMUX") != "" {
		if _, err := exec.LookPath("tmux"); err == nil {
			return "tmux capture-pane -p"
		}
	}
	// screen
	if os.Getenv("STY") != "" {
		if _, err := exec.LookPath("screen"); err == nil {
			return "screen -X hardcopy /dev/stdout"
		}
	}
	// macOS iTerm2
	if os.Getenv("TERM_PROGRAM") == "iTerm.app" {
		return `osascript -e 'tell application "iTerm2" to tell current session of current window to get contents'`
	}
	// macOS Terminal.app
	if os.Getenv("TERM_PROGRAM") == "Apple_Terminal" {
		return `osascript -e 'tell application "Terminal" to get contents of selected tab of front window'`
	}
	return ""
}

// currentGitBranch returns the current git branch or short SHA for detached HEAD.
func currentGitBranch() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		out, err = exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return branch
}

var ansiRegex = regexp.MustCompile(`\x1b(?:\[[0-9;]*[a-zA-Z]|\][^\x07\x1b]*(?:\x07|\x1b\\))|\x9b[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}
