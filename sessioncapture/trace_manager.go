// Package sessioncapture manages trace integration — auto-installing,
// enabling, disabling, and checking status of trace for session recording.
//
// trace remains a standalone binary. hawk bundles it and manages its lifecycle.
package sessioncapture

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TraceManager handles trace lifecycle from within hawk.
type TraceManager struct {
	ProjectDir string
}

// NewTraceManager creates a manager for the given project directory.
func NewTraceManager(projectDir string) *TraceManager {
	return &TraceManager{ProjectDir: projectDir}
}

// IsInstalled checks if the trace binary is available in PATH.
func (tm *TraceManager) IsInstalled() bool {
	_, err := exec.LookPath("trace")
	return err == nil
}

// IsEnabled checks if trace is enabled in the current project.
func (tm *TraceManager) IsEnabled() bool {
	// trace stores config in .trace/ at the repo root
	settingsPath := filepath.Join(tm.ProjectDir, ".trace", "settings.json")
	_, err := os.Stat(settingsPath)
	return err == nil
}

// Enable activates trace in the current project with hawk as the agent.
func (tm *TraceManager) Enable() error {
	if !tm.IsInstalled() {
		return fmt.Errorf("trace is not installed — run: brew install GrayCodeAI/tap/trace")
	}

	cmd := exec.Command("trace", "enable", "--agent", "hawk")
	cmd.Dir = tm.ProjectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("trace enable failed: %s\n%s", err, string(output))
	}
	return nil
}

// Disable deactivates trace in the current project (removes hooks).
func (tm *TraceManager) Disable() error {
	if !tm.IsInstalled() {
		return nil // nothing to disable
	}

	cmd := exec.Command("trace", "disable")
	cmd.Dir = tm.ProjectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("trace disable failed: %s\n%s", err, string(output))
	}
	return nil
}

// Status returns the current trace status for this project.
func (tm *TraceManager) Status() string {
	if !tm.IsInstalled() {
		return "not installed"
	}
	if !tm.IsEnabled() {
		return "installed but not enabled"
	}

	cmd := exec.Command("trace", "status")
	cmd.Dir = tm.ProjectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "enabled (status check failed)"
	}
	return strings.TrimSpace(string(output))
}

// AutoSetup is called when hawk starts a session. It:
// 1. Checks if trace is installed (bundled with hawk via brew)
// 2. If installed but not enabled → enables it automatically
// 3. If not installed → skips silently (hawk works fine without it)
func (tm *TraceManager) AutoSetup() error {
	if !tm.IsInstalled() {
		// trace not available — hawk works fine without it
		return nil
	}
	if tm.IsEnabled() {
		// already enabled — nothing to do
		return nil
	}
	// Auto-enable trace for this project
	return tm.Enable()
}

// FormatStatus returns a human-readable status line for hawk's UI.
func (tm *TraceManager) FormatStatus() string {
	if !tm.IsInstalled() {
		return "Session capture: disabled (trace not installed)"
	}
	if !tm.IsEnabled() {
		return "Session capture: disabled (run /trace-enable to activate)"
	}
	return "Session capture: active (trace recording sessions)"
}
