package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
)

// ErrInvalidMode is returned when an unrecognized sandbox mode string is supplied.
var ErrInvalidMode = errors.New("sandbox: invalid mode (must be strict, workspace, or off)")

// Source represents where a resolved sandbox policy originated.
type Source string

const (
	SourceOverride   Source = "override"
	SourceLog        Source = "log"
	SourceDefault    Source = "default"
	SourceDelegation Source = "delegation"
)

// ResolvedPolicy captures the effective sandbox policy for a session.
type ResolvedPolicy struct {
	Mode          Mode
	WorkspaceRoot string
	Source        Source
}

// Statement returns the concise model-facing statement for this policy.
func (p ResolvedPolicy) Statement() string {
	return FormatPolicyStatement(p.Mode, p.WorkspaceRoot)
}

// FormatPolicyStatement renders the concise model-facing statement for the
// given mode and workspace root.
func FormatPolicyStatement(mode Mode, workspaceRoot string) string {
	switch mode {
	case ModeStrict:
		return "Sandbox policy: strict (read-only). Do not refuse a required modification from this policy alone; try the tool and follow denial/escalation guidance."
	case ModeWorkspace:
		if workspaceRoot != "" {
			return fmt.Sprintf("Sandbox policy: workspace. You may modify files under %s. Some platform temporary areas are writable.", workspaceRoot)
		}
		return "Sandbox policy: workspace. You may modify files under the workspace root. Some platform temporary areas are writable."
	case ModeOff:
		return "Sandbox policy: off. File sandbox does not restrict modifications."
	default:
		return ""
	}
}

// FormatDelegationStatement renders the model-facing statement for a delegated subagent
// informing it that its execution scope is fixed and interactive approvals are disabled.
func FormatDelegationStatement() string {
	return "Delegation policy: this subagent operates with fixed permissions and cannot prompt for interactive approvals. If a task requires wider access or operations outside the allowed sandbox/scope, report the limitation in your final response rather than retrying or asking for confirmation."
}

// InheritDelegatedPolicy captures the explicit sandbox override from the parent
// (if any) and appends it to the child's eventlog with source=delegation.
// If the parent has no explicit override, nothing is stamped on the child.
func InheritDelegatedPolicy(parent, child any) (Mode, bool) {
	override, ok := OverrideOf(parent)
	if !ok || override == "" {
		return "", false
	}
	_ = SetSandboxModeWithSource(child, override, eventlog.SandboxModeSourceDelegation)
	return override, true
}

// CanonicalizeWorkspaceRoot normalizes and resolves symlinks on the workspace
// root so that canonicalization precedes lexical normalization (agreeing with
// process cwd resolution).
func CanonicalizeWorkspaceRoot(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil || cwd == "" {
			return ""
		}
		path = cwd
	}

	// Make path absolute if not already
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}

	// Resolve component by component so symlinks are evaluated before any ".."
	// is processed (process cwd resolution parity).
	cleanSepPath := filepath.ToSlash(path)
	parts := strings.Split(cleanSepPath, "/")

	var curr string
	if strings.HasPrefix(cleanSepPath, "/") {
		curr = "/"
	} else if len(parts) > 0 && strings.Contains(parts[0], ":") {
		// Windows drive letter e.g. "C:"
		curr = parts[0] + `\`
		parts = parts[1:]
	} else {
		curr = string(filepath.Separator)
	}

	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			// Move to parent of current resolved target
			curr = filepath.Dir(curr)
			if eval, err := filepath.EvalSymlinks(curr); err == nil {
				curr = eval
			}
			continue
		}
		next := filepath.Join(curr, part)
		if eval, err := filepath.EvalSymlinks(next); err == nil {
			curr = eval
		} else {
			curr = next
		}
	}

	if abs, err := filepath.Abs(curr); err == nil {
		curr = abs
	}
	return filepath.Clean(curr)
}

// DefaultPolicy is the package-level default policy resolver.
var DefaultPolicy = &SandboxPolicy{}

// SandboxPolicy provides policy management and resolution around sandbox modes.
type SandboxPolicy struct {
	ExplicitOverride Mode
}

// OverrideOf folds the latest sandbox.mode event on the session log.
func (p *SandboxPolicy) OverrideOf(session any) (Mode, bool) {
	return OverrideOf(session)
}

// Resolve resolves the effective sandbox policy: explicit override ?? fold(events) ?? deployment default.
func (p *SandboxPolicy) Resolve(session any, defaultMode Mode) ResolvedPolicy {
	if p != nil && p.ExplicitOverride != "" {
		return ResolvedPolicy{
			Mode:          p.ExplicitOverride,
			WorkspaceRoot: CanonicalizeWorkspaceRoot(extractCwd(session)),
			Source:        SourceOverride,
		}
	}
	return ResolvePolicy(session, defaultMode)
}

// OverrideOf folds the session's event log to find the latest sandbox.mode event
// (last switch wins). It returns the folded mode and true if an event was found.
func OverrideOf(session any) (Mode, bool) {
	events := extractEvents(session)
	if len(events) == 0 {
		return "", false
	}
	// Scan from tail to head (last switch wins)
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Type == eventlog.SandboxMode {
			if f, ok := ev.Data.(eventlog.SandboxModeFact); ok {
				if f.Valid() {
					return Mode(f.Mode), true
				}
			}
		}
	}
	return "", false
}

// SetSandboxMode is THE write path for sandbox mode changes. It validates the
// mode against the closed vocabulary and appends exactly one sandbox.mode event
// to the session's event log.
func SetSandboxMode(session any, mode Mode) error {
	return SetSandboxModeWithSource(session, mode, eventlog.SandboxModeSourceUser)
}

// SetSandboxModeWithSource is the write path with an explicit source (e.g. delegation).
func SetSandboxModeWithSource(session any, mode Mode, source eventlog.SandboxModeSource) error {
	switch mode {
	case ModeStrict, ModeWorkspace, ModeOff:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidMode, mode)
	}

	// Update in-memory mode if session supports it
	if setter, ok := session.(interface{ SetSandboxMode(Mode) }); ok {
		setter.SetSandboxMode(mode)
	}

	log := extractLog(session)
	if log != nil {
		log.AppendSandboxModeWithSource(string(mode), source)
	}
	return nil
}

// ResolvePolicy resolves the effective sandbox policy following the precedence:
// explicit override ?? fold(events) ?? deployment default.
// Workspace root is the canonicalized session cwd.
func ResolvePolicy(session any, defaultMode Mode) ResolvedPolicy {
	// 1. Explicit override if session exposes one
	if getter, ok := session.(interface{ ExplicitSandboxMode() Mode }); ok {
		if m := getter.ExplicitSandboxMode(); m != "" {
			return ResolvedPolicy{
				Mode:          m,
				WorkspaceRoot: CanonicalizeWorkspaceRoot(extractCwd(session)),
				Source:        SourceOverride,
			}
		}
	}

	// 2. Fold events from session journal (last switch wins)
	if mode, ok := OverrideOf(session); ok && mode != "" {
		return ResolvedPolicy{
			Mode:          mode,
			WorkspaceRoot: CanonicalizeWorkspaceRoot(extractCwd(session)),
			Source:        SourceLog,
		}
	}

	// 3. Deployment default
	effMode := defaultMode
	if effMode == "" {
		effMode = ModeWorkspace
	}
	return ResolvedPolicy{
		Mode:          effMode,
		WorkspaceRoot: CanonicalizeWorkspaceRoot(extractCwd(session)),
		Source:        SourceDefault,
	}
}

func extractLog(session any) *eventlog.Log {
	if session == nil {
		return nil
	}
	switch s := session.(type) {
	case *eventlog.Log:
		return s
	case interface{ Journal() *eventlog.Log }:
		return s.Journal()
	case interface{ Log() *eventlog.Log }:
		return s.Log()
	default:
		return nil
	}
}

func extractEvents(session any) []eventlog.Event {
	if session == nil {
		return nil
	}
	switch s := session.(type) {
	case *eventlog.Log:
		return s.Snapshot()
	case []eventlog.Event:
		return s
	case interface{ Snapshot() []eventlog.Event }:
		return s.Snapshot()
	case interface{ Journal() *eventlog.Log }:
		if j := s.Journal(); j != nil {
			return j.Snapshot()
		}
	case interface{ Events() []eventlog.Event }:
		return s.Events()
	}
	return nil
}

func extractCwd(session any) string {
	if session == nil {
		return ""
	}
	switch s := session.(type) {
	case string:
		return s
	case interface{ Cwd() string }:
		return s.Cwd()
	case interface{ WorkingDir() string }:
		return s.WorkingDir()
	case interface{ GetCwd() string }:
		return s.GetCwd()
	case interface{ WorkDir() string }:
		return s.WorkDir()
	default:
		return ""
	}
}
