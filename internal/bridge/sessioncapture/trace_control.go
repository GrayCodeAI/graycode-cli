// Package sessioncapture provides hawk's interface to trace.
//
// trace is a fully independent tool (own repo, own binary, works with any agent).
// hawk can optionally manage it via slash commands:
//
//	/trace-enable   — enable session capture
//	/trace-disable  — disable session capture
//	/trace-status   — show capture status
//
// hawk NEVER auto-enables trace. The user must explicitly opt in.
package sessioncapture

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TraceControl lets hawk manage trace when the user asks.
// trace remains independent — hawk just provides UI convenience.
type TraceControl struct {
	ProjectDir string
}

// NewTraceControl creates a controller for the given project.
func NewTraceControl(projectDir string) *TraceControl {
	return &TraceControl{ProjectDir: projectDir}
}

// IsInstalled checks if trace binary exists in PATH.
func (tc *TraceControl) IsInstalled() bool {
	_, err := exec.LookPath("trace")
	return err == nil
}

// IsEnabled checks if trace is active in this project.
func (tc *TraceControl) IsEnabled() bool {
	settingsPath := filepath.Join(tc.ProjectDir, ".trace", "settings.json")
	_, err := os.Stat(settingsPath)
	return err == nil
}

// Enable activates trace for this project. User must explicitly call this.
func (tc *TraceControl) Enable() (string, error) {
	if !tc.IsInstalled() {
		return "", fmt.Errorf("trace is not installed. Install with: brew install GrayCodeAI/tap/trace")
	}
	if tc.IsEnabled() {
		return "Session capture is already enabled.", nil
	}

	cmd := exec.Command("trace", "enable", "--agent", "hawk")
	cmd.Dir = tc.ProjectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to enable trace: %s", strings.TrimSpace(string(output)))
	}
	return "Session capture enabled. All future sessions will be recorded.", nil
}

// Disable deactivates trace for this project. User must explicitly call this.
func (tc *TraceControl) Disable() (string, error) {
	if !tc.IsInstalled() {
		return "trace is not installed — nothing to disable.", nil
	}
	if !tc.IsEnabled() {
		return "Session capture is already disabled.", nil
	}

	cmd := exec.Command("trace", "disable")
	cmd.Dir = tc.ProjectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to disable trace: %s", strings.TrimSpace(string(output)))
	}
	return "Session capture disabled. Sessions will no longer be recorded.", nil
}

// Status returns current trace state for display in hawk.
func (tc *TraceControl) Status() string {
	if !tc.IsInstalled() {
		return "Session capture: not available (trace not installed)\n  Install: brew install GrayCodeAI/tap/trace"
	}
	if !tc.IsEnabled() {
		return "Session capture: disabled\n  Enable: /trace-enable"
	}

	// Get detailed status from trace
	cmd := exec.Command("trace", "status")
	cmd.Dir = tc.ProjectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "Session capture: enabled (running)"
	}
	return "Session capture: enabled\n" + strings.TrimSpace(string(output))
}
