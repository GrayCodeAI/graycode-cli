package safety

import (
	"strings"
	"sync"
)

// AutonomyProfile holds the derived permission flags for an autonomy level,
// with optional per-flag overrides. It is the single source of truth for
// "should this tool call prompt the user at the current tier?".
//
// The level still derives the default flags, but users can override individual
// flags (e.g. "Full but still ask for network") via AutonomyOverrides in
// settings without changing their tier.
type AutonomyProfile struct {
	Level           AutonomyLevel
	AutoContinue    bool
	AutoApplyEdits  bool
	AutoExecuteBash bool
	AutoCommit      bool
	AutoNetwork     bool // gates WebFetch, WebSearch, external API tools

	// overrides records which flags were explicitly set by the user so the
	// picker/UI can show "customized" and reset can clear them.
	overrides map[string]bool
	mu        sync.RWMutex
}

// ProfileFromLevel derives the default profile for an autonomy level.
func ProfileFromLevel(level AutonomyLevel) *AutonomyProfile {
	cfg := PresetConfig(level)
	return &AutonomyProfile{
		Level:           level,
		AutoContinue:    cfg.AutoContinue,
		AutoApplyEdits:  cfg.AutoApplyEdits,
		AutoExecuteBash: cfg.AutoExecuteBash,
		AutoCommit:      cfg.AutoCommit,
		// AutoNetwork defaults to true for all tiers except Supervised (where
		// everything asks anyway).
		AutoNetwork: level >= AutonomyBasic,
		overrides:   make(map[string]bool),
	}
}

// ApplyOverrides merges a map of flag-name → bool onto the profile. Unknown
// keys are ignored. This is called at session start from settings.AutonomyOverrides.
func (p *AutonomyProfile) ApplyOverrides(overrides map[string]bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.overrides == nil {
		p.overrides = make(map[string]bool)
	}
	for k, v := range overrides {
		p.setFlag(k, v)
	}
}

// Override sets a single flag by name. Returns false if the name is unknown.
func (p *AutonomyProfile) Override(flag string, val bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.setFlag(flag, val)
}

// setFlag is the unlocked helper. Returns false for unknown flag names.
func (p *AutonomyProfile) setFlag(flag string, val bool) bool {
	// Normalize: lowercase, strip spaces/underscores/hyphens so "auto_execute_bash",
	// "auto-execute-bash", "autoExecuteBash" all match.
	norm := strings.ToLower(strings.TrimSpace(flag))
	norm = strings.ReplaceAll(norm, "_", "")
	norm = strings.ReplaceAll(norm, "-", "")
	norm = strings.ReplaceAll(norm, " ", "")
	switch norm {
	case "autocontinue":
		p.AutoContinue = val
	case "autoapplyedits":
		p.AutoApplyEdits = val
	case "autoexecutebash":
		p.AutoExecuteBash = val
	case "autocommit":
		p.AutoCommit = val
	case "autonetwork":
		p.AutoNetwork = val
	default:
		return false
	}
	p.overrides[norm] = true
	return true
}

// IsOverridden reports whether a flag was explicitly set by the user.
func (p *AutonomyProfile) IsOverridden(flag string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	norm := strings.ToLower(strings.TrimSpace(flag))
	norm = strings.ReplaceAll(norm, "_", "")
	norm = strings.ReplaceAll(norm, "-", "")
	norm = strings.ReplaceAll(norm, " ", "")
	return p.overrides[norm]
}

// Overrides returns a copy of the override set (for persistence/display).
func (p *AutonomyProfile) Overrides() map[string]bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make(map[string]bool, len(p.overrides))
	for k, v := range p.overrides {
		out[k] = v
	}
	return out
}

// NeedsPermission decides whether a tool call should prompt the user, consulting
// the profile's flags and overrides. It replaces AutonomyConfig.NeedsPermission
// when a profile is active.
//
// isSafe indicates the specific Bash invocation was classified as safe (e.g.
// read-only git). Network tools are gated by AutoNetwork.
func (p *AutonomyProfile) NeedsPermission(toolName string, isSafe bool) bool {
	p.mu.RLock()
	level := p.Level
	autoBash := p.AutoExecuteBash
	autoNetwork := p.AutoNetwork
	p.mu.RUnlock()

	switch level {
	case AutonomyYOLO:
		return false
	case AutonomyFull:
		// Bash: auto-allow safe commands unless the user overrode bash off.
		if canonicalToolName(toolName) == "Bash" {
			if !autoBash {
				return true // override: always ask for bash
			}
			return !isSafe
		}
		// Network tools respect the override.
		if isNetworkTool(toolName) {
			return !autoNetwork
		}
		return false
	case AutonomySemi:
		if isReadOnlyTool(toolName) {
			return false
		}
		// Writes are auto-allowed at Semi.
		if isWriteTool(toolName) {
			return false
		}
		if isNetworkTool(toolName) {
			return !autoNetwork
		}
		// Bash asks unless explicitly overridden on.
		if canonicalToolName(toolName) == "Bash" {
			return !autoBash
		}
		return true
	case AutonomyBasic:
		if isReadOnlyTool(toolName) {
			return false
		}
		return true
	default: // Supervised
		return true
	}
}

// isNetworkTool reports whether a tool performs outbound network access.
func isNetworkTool(toolName string) bool {
	switch canonicalToolName(toolName) {
	case "WebFetch", "WebSearch", "Browser", "Screenshot", "Download":
		return true
	}
	return false
}

// isWriteTool reports whether a tool creates or modifies files.
func isWriteTool(toolName string) bool {
	switch canonicalToolName(toolName) {
	case "Write", "Edit", "StructuredEdit", "MultiEdit", "FileEdit", "NotebookEdit":
		return true
	}
	return false
}

// isReadOnlyTool reports whether a tool only reads state.
func isReadOnlyTool(toolName string) bool {
	switch canonicalToolName(toolName) {
	case "Read", "LS", "Glob", "Grep", "SmartReader", "CodeSearch", "CodeGraph", "Impact":
		return true
	}
	return false
}
