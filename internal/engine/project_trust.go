package engine

import (
	"fmt"
	"os"

	"github.com/GrayCodeAI/hawk/internal/flags"
	"github.com/GrayCodeAI/hawk/internal/trust"
)

// ProjectTrustStatus summarizes folder-trust for the working directory.
// Used by TUI status, /status, and /start onboarding.
type ProjectTrustStatus struct {
	Path     string
	Trusted  bool
	Enforced bool
	// Blocked is true when enforcement is on and the path is not trusted —
	// project hooks/MCP/plugins from the repo will not load.
	Blocked bool
}

// ProjectTrust returns trust state for cwd (or path if non-empty).
func ProjectTrust(path string) ProjectTrustStatus {
	if path == "" {
		path, _ = os.Getwd()
	}
	st := ProjectTrustStatus{Path: path, Enforced: flags.FolderTrust()}
	store, err := trust.Open("")
	if err != nil {
		// Fail closed on read errors when enforcement is on.
		st.Trusted = false
		st.Blocked = st.Enforced
		return st
	}
	st.Trusted = store.IsTrusted(path)
	st.Blocked = st.Enforced && !st.Trusted
	return st
}

// TrustProject marks path trusted with reason (empty path = cwd).
func TrustProject(path, reason string) error {
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	store, err := trust.Open("")
	if err != nil {
		return err
	}
	if reason == "" {
		reason = "user approved via hawk"
	}
	return store.Trust(path, reason)
}

// UntrustProject removes trust for path (empty = cwd).
func UntrustProject(path string) error {
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	store, err := trust.Open("")
	if err != nil {
		return err
	}
	return store.Untrust(path)
}

// String is a short status label for HUD.
func (t ProjectTrustStatus) String() string {
	if !t.Enforced {
		return "trust:off"
	}
	if t.Trusted {
		return "trusted"
	}
	return "untrusted"
}

// Detail is multi-line copy for /status and /start.
func (t ProjectTrustStatus) Detail() string {
	if !t.Enforced {
		return fmt.Sprintf("Folder trust enforcement is off (path %s).", t.Path)
	}
	if t.Trusted {
		return fmt.Sprintf("Project trusted: %s\nProject hooks, MCP, and plugins may load.", t.Path)
	}
	return fmt.Sprintf("Project NOT trusted: %s\nProject-scoped hooks/MCP/plugins are blocked (RCE mitigation).\nRun: /trust add   or   hawk trust add", t.Path)
}
