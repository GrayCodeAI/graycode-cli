package engine

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/tool"
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
	mu         sync.RWMutex
	allowRules []string // patterns like "bash:go test*", "file_write:*.go"
	denyRules  []string
	allowAll   map[string]bool // tool names that are always allowed
}

func NewPermissionMemory() *PermissionMemory {
	return &PermissionMemory{allowAll: make(map[string]bool)}
}

// PermissionMode controls how permission prompts are handled.
type PermissionMode string

const (
	PermissionModeDefault           PermissionMode = "default"
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
	PermissionModeDontAsk           PermissionMode = "dontAsk"
	PermissionModePlan              PermissionMode = "plan"
)

// SetPermissionMode applies an archive-compatible permission mode.
func (s *Session) SetPermissionMode(mode string) error {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		s.Mode = PermissionModeDefault
		return nil
	}
	switch PermissionMode(mode) {
	case PermissionModeDefault, PermissionModeAcceptEdits, PermissionModeBypassPermissions, PermissionModeDontAsk, PermissionModePlan:
		s.Mode = PermissionMode(mode)
		return nil
	default:
		return fmt.Errorf("invalid permission mode %q (valid: default, acceptEdits, bypassPermissions, dontAsk, plan)", mode)
	}
}

// SetMaxTurns caps the number of model turns in the agent loop.
func (s *Session) SetMaxTurns(turns int) error {
	if turns < 0 {
		return fmt.Errorf("max turns must be non-negative")
	}
	s.MaxTurns = turns
	return nil
}

// SetMaxBudgetUSD caps estimated API spend for this session.
func (s *Session) SetMaxBudgetUSD(amount float64) error {
	if amount < 0 {
		return fmt.Errorf("max budget must be non-negative")
	}
	s.MaxBudgetUSD = amount
	return nil
}

// AlwaysAllow marks a tool as always allowed.
func (pm *PermissionMemory) AlwaysAllow(toolName string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.allowAll[canonicalToolName(toolName)] = true
}

// AlwaysAllowPattern adds a pattern rule (e.g. "bash:go *").
func (pm *PermissionMemory) AlwaysAllowPattern(pattern string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.allowRules = append(pm.allowRules, normalizeRuleSpec(pattern))
}

// AlwaysDeny marks a tool as always denied.
func (pm *PermissionMemory) AlwaysDeny(toolName string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.denyRules = append(pm.denyRules, canonicalToolName(toolName)+":*")
}

// AlwaysDenyPattern adds a deny pattern rule.
func (pm *PermissionMemory) AlwaysDenyPattern(pattern string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.denyRules = append(pm.denyRules, normalizeRuleSpec(pattern))
}

// AllowSpec applies an archive-style permission rule, e.g. "Bash(git:*)".
func (pm *PermissionMemory) AllowSpec(spec string) {
	toolName, pattern := parseRuleSpec(spec)
	if pattern == "" {
		pm.AlwaysAllow(toolName)
		return
	}
	pm.AlwaysAllowPattern(toolName + ":" + pattern)
}

// DenySpec applies an archive-style deny rule, e.g. "Write(*.env)".
func (pm *PermissionMemory) DenySpec(spec string) {
	toolName, pattern := parseRuleSpec(spec)
	if pattern == "" {
		pm.AlwaysDeny(toolName)
		return
	}
	pm.AlwaysDenyPattern(toolName + ":" + pattern)
}

// Check returns: true=allowed, false=denied, nil=ask user.
func (pm *PermissionMemory) Check(toolName string, summary string) *bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	toolName = canonicalToolName(toolName)

	for _, rule := range pm.denyRules {
		parts := strings.SplitN(rule, ":", 2)
		if len(parts) == 2 && parts[0] == toolName {
			if matchRulePattern(parts[1], summary) {
				f := false
				return &f
			}
		}
	}

	if pm.allowAll[toolName] {
		t := true
		return &t
	}

	for _, rule := range pm.allowRules {
		parts := strings.SplitN(rule, ":", 2)
		if len(parts) == 2 && parts[0] == toolName {
			if matchRulePattern(parts[1], summary) {
				t := true
				return &t
			}
		}
	}

	return nil // ask user
}

// toolNeedsPermission returns true for tools that modify state.
func toolNeedsPermission(name string, args map[string]interface{}) bool {
	switch canonicalToolName(name) {
	case "Write", "Edit", "NotebookEdit":
		return true
	case "Bash":
		// Check if the command is suspicious
		if cmd, ok := args["command"].(string); ok {
			return tool.IsSuspicious(cmd)
		}
		return true // fail-closed: if we can't parse, ask
	default:
		return false
	}
}

// toolSummary generates a human-readable summary of what a tool call will do.
func toolSummary(name string, args map[string]interface{}) string {
	switch canonicalToolName(name) {
	case "Bash":
		if cmd, ok := args["command"].(string); ok {
			if len(cmd) > 120 {
				cmd = cmd[:120] + "..."
			}
			return cmd
		}
	case "Write":
		if p, ok := pathArgument(args); ok {
			return p
		}
	case "Edit":
		if p, ok := pathArgument(args); ok {
			return p
		}
	case "NotebookEdit":
		if p, ok := pathArgument(args); ok {
			return p
		}
	}
	return name
}

func (s *Session) needsPermission(name string, args map[string]interface{}) bool {
	if s.Mode == PermissionModePlan && canonicalToolName(name) == "ExitPlanMode" {
		return true
	}
	return toolNeedsPermission(name, args)
}

func (s *Session) modeDecision(name string) *bool {
	toolName := canonicalToolName(name)
	switch s.Mode {
	case PermissionModeBypassPermissions:
		return boolPtr(true)
	case PermissionModeDontAsk:
		return boolPtr(false)
	case PermissionModePlan:
		if toolName == "ExitPlanMode" {
			return nil
		}
		return boolPtr(false)
	case PermissionModeAcceptEdits:
		if toolName == "Write" || toolName == "Edit" || toolName == "NotebookEdit" {
			return boolPtr(true)
		}
	}
	return nil
}

func (s *Session) applyToolState(name string) {
	switch canonicalToolName(name) {
	case "EnterPlanMode":
		s.Mode = PermissionModePlan
	case "ExitPlanMode":
		s.Mode = PermissionModeDefault
	}
}

func (s *Session) exceededBudget() bool {
	return s.MaxBudgetUSD > 0 && s.Cost.Total() > s.MaxBudgetUSD
}

func boolPtr(v bool) *bool {
	return &v
}

func pathArgument(args map[string]interface{}) (string, bool) {
	if p, ok := args["path"].(string); ok && p != "" {
		return p, true
	}
	if p, ok := args["file_path"].(string); ok && p != "" {
		return p, true
	}
	return "", false
}

func canonicalToolName(name string) string {
	switch strings.ToLower(name) {
	case "bash":
		return "Bash"
	case "file_read", "read":
		return "Read"
	case "file_write", "write":
		return "Write"
	case "file_edit", "edit":
		return "Edit"
	case "ls":
		return "LS"
	case "glob":
		return "Glob"
	case "grep":
		return "Grep"
	case "web_fetch", "webfetch":
		return "WebFetch"
	case "web_search", "websearch":
		return "WebSearch"
	case "agent", "task":
		return "Agent"
	case "ask_user", "askuserquestion":
		return "AskUserQuestion"
	case "todo", "todowrite":
		return "TodoWrite"
	case "lsp":
		return "LSP"
	case "enter_plan_mode", "enterplanmode":
		return "EnterPlanMode"
	case "exit_plan_mode", "exitplanmode":
		return "ExitPlanMode"
	case "notebook_edit", "notebookedit":
		return "NotebookEdit"
	case "config":
		return "Config"
	case "brief", "sendusermessage":
		return "SendUserMessage"
	default:
		return name
	}
}

func parseRuleSpec(spec string) (toolName, pattern string) {
	spec = strings.TrimSpace(spec)
	if open := strings.Index(spec, "("); open > 0 && strings.HasSuffix(spec, ")") {
		return spec[:open], spec[open+1 : len(spec)-1]
	}
	if parts := strings.SplitN(spec, ":", 2); len(parts) == 2 {
		return parts[0], parts[1]
	}
	return spec, ""
}

func normalizeRuleSpec(spec string) string {
	toolName, pattern := parseRuleSpec(spec)
	return canonicalToolName(toolName) + ":" + normalizeRulePattern(pattern)
}

func normalizeRulePattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if strings.HasSuffix(pattern, ":*") {
		return strings.TrimSuffix(pattern, ":*") + " *"
	}
	return pattern
}

func matchRulePattern(pattern, summary string) bool {
	if pattern == "*" {
		return true
	}
	if matched, _ := filepath.Match(pattern, summary); matched {
		return true
	}
	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, " *")
		return summary == prefix || strings.HasPrefix(summary, prefix+" ")
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(summary, pattern[:len(pattern)-1])
	}
	return pattern == summary
}
