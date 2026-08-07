package engine

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/sandbox"
)

// IsolationProfile is the single user-facing story for how Hawk isolates
// tool execution. It unifies:
//
//   - OS sandbox policy (seatbelt / unshare) used by Bash WrapCommand
//   - container-first execution (Docker sandbox image)
//   - path-guard sandbox mode on ToolContext
//
// ApplyIsolationProfile is the only API faces should use instead of setting
// SandboxMode and container flags independently.
type IsolationProfile struct {
	// OSMode is off | workspace | strict. Empty is treated as off.
	OSMode sandbox.Mode `json:"os_mode"`
	// ContainerRequired when true disables tools until a container executor is running.
	ContainerRequired bool `json:"container_required"`
	// Label is optional human name for status UI (e.g. "safe", "dev", "locked").
	Label string `json:"label,omitempty"`
}

// Named isolation presets for progressive trust.
var (
	// IsolationDev is host-friendly: no OS wrap, container optional.
	IsolationDev = IsolationProfile{OSMode: sandbox.ModeOff, ContainerRequired: false, Label: "dev"}
	// IsolationWorkspace wraps shell in workspace sandbox; container optional.
	IsolationWorkspace = IsolationProfile{OSMode: sandbox.ModeWorkspace, ContainerRequired: false, Label: "workspace"}
	// IsolationStrict is read-only OS policy for exploration.
	IsolationStrict = IsolationProfile{OSMode: sandbox.ModeStrict, ContainerRequired: false, Label: "strict"}
	// IsolationContainer prefers Docker isolation for all tools that support it.
	IsolationContainer = IsolationProfile{OSMode: sandbox.ModeWorkspace, ContainerRequired: true, Label: "container"}
)

// ParseIsolationProfile accepts preset names or "os=workspace,container=1".
func ParseIsolationProfile(s string) (IsolationProfile, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "", "dev", "off", "host":
		return IsolationDev, nil
	case "workspace", "ws":
		return IsolationWorkspace, nil
	case "strict", "ro", "read-only", "readonly":
		return IsolationStrict, nil
	case "container", "docker":
		return IsolationContainer, nil
	}
	// Key=value form: os=workspace,container=true
	p := IsolationDev
	p.Label = "custom"
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return IsolationProfile{}, fmt.Errorf("isolation: invalid token %q", part)
		}
		k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
		switch k {
		case "os", "mode", "sandbox":
			switch v {
			case "off", "none", "dev", "host":
				p.OSMode = sandbox.ModeOff
			case "workspace", "ws":
				p.OSMode = sandbox.ModeWorkspace
			case "strict":
				p.OSMode = sandbox.ModeStrict
			default:
				return IsolationProfile{}, fmt.Errorf("isolation: unknown os mode %q", v)
			}
		case "container", "docker":
			p.ContainerRequired = v == "1" || v == "true" || v == "yes" || v == "on"
		case "label":
			p.Label = v
		default:
			return IsolationProfile{}, fmt.Errorf("isolation: unknown key %q", k)
		}
	}
	return p, nil
}

// String returns a short status label.
func (p IsolationProfile) String() string {
	if p.Label != "" {
		return p.Label
	}
	osMode := string(p.OSMode)
	if osMode == "" {
		osMode = "off"
	}
	if p.ContainerRequired {
		return fmt.Sprintf("os=%s,container=required", osMode)
	}
	return fmt.Sprintf("os=%s", osMode)
}

// ShortLabel returns a compact single-word label suitable for the
// status bar control plane chip. For the four named presets it
// returns the canonical short form; for custom profiles it falls
// back to the first word of String().
func (p IsolationProfile) ShortLabel() string {
	switch {
	case p == IsolationDev:
		return "dev"
	case p == IsolationWorkspace:
		return "workspace"
	case p == IsolationStrict:
		return "strict"
	case p == IsolationContainer:
		return "container"
	}
	// For custom profiles, extract the first meaningful word.
	s := p.String()
	if idx := strings.Index(s, ","); idx != -1 {
		s = s[:idx]
	}
	if s == "" {
		return "off"
	}
	return s
}

// Normalize fills empty OSMode as off.
func (p IsolationProfile) Normalize() IsolationProfile {
	if p.OSMode == "" {
		p.OSMode = sandbox.ModeOff
	}
	return p
}

// ApplyIsolationProfile applies the unified isolation story to permission + tools.
func (s *Session) ApplyIsolationProfile(p IsolationProfile) {
	if s == nil {
		return
	}
	p = p.Normalize()
	s.mu.Lock()
	s.isolation = p
	s.mu.Unlock()
	if s.PermSvc() != nil {
		s.PermSvc().SetSandboxMode(p.OSMode)
	}
	if s.Tools() != nil {
		s.Tools().SetContainerRequired(p.ContainerRequired)
	}
}

// Isolation returns the active isolation profile (zero value = dev/off).
func (s *Session) Isolation() IsolationProfile {
	if s == nil {
		return IsolationDev
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.isolation.OSMode == "" && !s.isolation.ContainerRequired && s.isolation.Label == "" {
		// Derive from live services when never explicitly set.
		p := IsolationDev
		if s.perms != nil {
			p.OSMode = s.perms.SandboxMode()
			if p.OSMode == "" {
				p.OSMode = sandbox.ModeOff
			}
		}
		return p
	}
	return s.isolation.Normalize()
}
