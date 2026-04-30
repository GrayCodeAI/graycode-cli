package engine

import (
	"path/filepath"
	"strings"
	"sync"
)

// PermissionRequest is sent from engine to TUI when a tool needs approval.
type PermissionRequest struct {
	ToolName string
	ToolID   string
	Summary  string
	Response chan bool
}

// PermissionMemory stores always-allow and always-deny rules.
type PermissionMemory struct {
	mu          sync.RWMutex
	allowRules  []string // patterns like "bash:go test*", "file_write:*.go"
	denyRules   []string
	allowAll    map[string]bool // tool names that are always allowed
}

func NewPermissionMemory() *PermissionMemory {
	return &PermissionMemory{allowAll: make(map[string]bool)}
}

// AlwaysAllow marks a tool as always allowed.
func (pm *PermissionMemory) AlwaysAllow(toolName string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.allowAll[toolName] = true
}

// AlwaysAllowPattern adds a pattern rule (e.g. "bash:go *").
func (pm *PermissionMemory) AlwaysAllowPattern(pattern string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.allowRules = append(pm.allowRules, pattern)
}

// Check returns: true=allowed, false=denied, nil=ask user.
func (pm *PermissionMemory) Check(toolName string, summary string) *bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.allowAll[toolName] {
		t := true
		return &t
	}

	for _, rule := range pm.allowRules {
		parts := strings.SplitN(rule, ":", 2)
		if len(parts) == 2 && parts[0] == toolName {
			if matched, _ := filepath.Match(parts[1], summary); matched {
				t := true
				return &t
			}
			// Also check prefix match
			if strings.HasPrefix(summary, parts[1]) {
				t := true
				return &t
			}
		}
	}

	for _, rule := range pm.denyRules {
		parts := strings.SplitN(rule, ":", 2)
		if len(parts) == 2 && parts[0] == toolName {
			if matched, _ := filepath.Match(parts[1], summary); matched {
				f := false
				return &f
			}
		}
	}

	return nil // ask user
}

// toolNeedsPermission returns true for tools that modify state.
func toolNeedsPermission(name string) bool {
	switch name {
	case "bash", "file_write", "file_edit":
		return true
	default:
		return false
	}
}

// toolSummary generates a human-readable summary of what a tool call will do.
func toolSummary(name string, args map[string]interface{}) string {
	switch name {
	case "bash":
		if cmd, ok := args["command"].(string); ok {
			if len(cmd) > 120 {
				cmd = cmd[:120] + "..."
			}
			return cmd
		}
	case "file_write":
		if p, ok := args["path"].(string); ok {
			return p
		}
	case "file_edit":
		if p, ok := args["path"].(string); ok {
			return p
		}
	}
	return name
}
