package safety

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	contracts "github.com/GrayCodeAI/eagle/policy"
	"github.com/GrayCodeAI/graycode-cli/internal/permissions"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
)

// PermissionRequest is sent from engine to TUI when a tool needs approval.
type PermissionRequest struct {
	contracts.PermissionRequest
	Response chan bool
}

// PermissionMemory stores always-allow and always-deny rules.
type PermissionMemory struct {
	mu         sync.RWMutex
	allowRules []string // patterns like "bash:go test*", "file_write:*.go"
	denyRules  []string
	allowAll   map[string]bool // tool names that are always allowed
}

// RuleSnapshot is an immutable copy of remembered permission rules.
type RuleSnapshot struct {
	AllowRules []string
	DenyRules  []string
	AllowAll   map[string]bool
}

func NewPermissionMemory() *PermissionMemory {
	return &PermissionMemory{allowAll: make(map[string]bool)}
}

// Snapshot returns a deep copy that can safely be used by one evaluation.
func (pm *PermissionMemory) Snapshot() RuleSnapshot {
	if pm == nil {
		return RuleSnapshot{AllowAll: map[string]bool{}}
	}
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	allowAll := make(map[string]bool, len(pm.allowAll))
	for name, allowed := range pm.allowAll {
		allowAll[name] = allowed
	}
	return RuleSnapshot{AllowRules: append([]string(nil), pm.allowRules...), DenyRules: append([]string(nil), pm.denyRules...), AllowAll: allowAll}
}

// NewPermissionMemoryFromSnapshot creates an independent rule store.
func NewPermissionMemoryFromSnapshot(snapshot RuleSnapshot) *PermissionMemory {
	allowAll := make(map[string]bool, len(snapshot.AllowAll))
	for name, allowed := range snapshot.AllowAll {
		allowAll[name] = allowed
	}
	return &PermissionMemory{allowRules: append([]string(nil), snapshot.AllowRules...), denyRules: append([]string(nil), snapshot.DenyRules...), allowAll: allowAll}
}

// Grants returns the remembered allow/deny rules as canonical permissions.Grant
// slice. allowAll entries become tool-wide allow grants; allowRules/denyRules
// become tool:pattern grants. Source is set so UnifiedGrants can rank user
// rules above auto-learned ones.
func (pm *PermissionMemory) Grants() []permissions.Grant {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var out []permissions.Grant
	for tool := range pm.allowAll {
		out = append(out, permissions.Grant{
			Tool:    tool,
			Pattern: "*",
			Allow:   true,
			Source:  permissions.SourceUserAllow,
			Label:   "from settings",
		})
	}
	for _, rule := range pm.allowRules {
		tool, pattern := parseRuleSpec(rule)
		out = append(out, permissions.Grant{
			Tool:    tool,
			Pattern: pattern,
			Allow:   true,
			Source:  permissions.SourceUserAllow,
			Label:   "from settings",
		})
	}
	for _, rule := range pm.denyRules {
		tool, pattern := parseRuleSpec(rule)
		out = append(out, permissions.Grant{
			Tool:    tool,
			Pattern: pattern,
			Allow:   false,
			Source:  permissions.SourceUserDeny,
			Label:   "from settings",
		})
	}
	return out
}

// Reset clears all allow/deny memory so the active rule set can be rebuilt.
func (pm *PermissionMemory) Reset() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.allowRules = nil
	pm.denyRules = nil
	pm.allowAll = make(map[string]bool)
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
func ToolNeedsPermission(name string, args map[string]interface{}) bool {
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

// ToolSummary generates a human-readable summary of what a tool call will do.
// This short form is also used for AutoMode / memory rule matching — keep it stable.
func ToolSummary(name string, args map[string]interface{}) string {
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

// FormatPermissionDisplay builds the multi-line body shown in the TUI permission
// box. toolName and summary are display inputs; summary should remain ToolSummary
// so AutoMode still matches after the user answers.
func FormatPermissionDisplay(toolName, summary string) string {
	policy := ToolPolicyFor(toolName)
	risk := string(policy.DefaultRisk)
	if risk == "" {
		risk = string(RiskMedium)
	}
	// Escalate Bash to high when the summary still looks like a shell command
	// that was forced through a prompt (suspicious commands).
	if canonicalToolName(toolName) == "Bash" && risk != string(RiskHigh) {
		if tool.IsSuspicious(summary) {
			risk = string(RiskHigh)
		}
	}
	why := permissionWhyLine(toolName, RiskLevel(risk), policy)
	if strings.TrimSpace(summary) == "" {
		summary = toolName
	}
	return fmt.Sprintf("[%s risk] %s\n%s\n%s", strings.ToUpper(risk), canonicalToolName(toolName), summary, why)
}

func permissionWhyLine(toolName string, risk RiskLevel, policy ToolPolicy) string {
	switch risk {
	case RiskHigh:
		if canonicalToolName(toolName) == "Bash" {
			return "Why: shell can change your system — review the command before allowing."
		}
		return "Why: high-impact action needs your confirmation."
	case RiskMedium:
		if hasCapability(policy, CapabilityFilesystemWrite) || hasCapability(policy, CapabilityFilesystemDelete) {
			return "Why: this can modify or delete project files."
		}
		return "Why: this can change project state."
	default:
		return "Why: current autonomy settings require confirmation for this tool."
	}
}

func hasCapability(policy ToolPolicy, want Capability) bool {
	for _, c := range policy.Capabilities {
		if c == want {
			return true
		}
	}
	return false
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
	case "code_match", "codematch", "match_code":
		return "CodeMatch"
	case "fuzzy_find", "fuzzyfind", "ffind":
		return "FuzzyFind"
	case "batch_exec", "batchexec":
		return "BatchExec"
	case "toolset":
		return "Toolset"
	case "tool_health", "toolhealth", "tools_health":
		return "ToolHealth"
	case "project_verify", "projectverify", "verify_project":
		return "ProjectVerify"
	case "app_verify", "appverify", "verify_app":
		return "AppVerify"
	case "generate_media", "generatemedia", "media":
		return "GenerateMedia"
	case "dependency_audit", "dependencyaudit", "deps":
		return "DependencyAudit"
	case "git_history", "githistory", "git-history":
		return "GitHistory"
	case "github", "gh":
		return "GitHub"
	case "sql", "sql_query":
		return "SQL"
	case "agent", "task":
		return "Agent"
	case "ask_user", "askuser", "askuserquestion":
		return "AskUserQuestion"
	case "todo", "todowrite":
		return "TodoWrite"
	case "lsp":
		return "LSP"
	case "specify":
		return "Specify"
	case "plan":
		return "Plan"
	case "tasks":
		return "Tasks"
	case "approve_implementation", "approveimplementation":
		return "ApproveImplementation"
	case "spec_status", "specstatus":
		return "SpecStatus"
	case "spec_edit", "specedit":
		return "SpecEdit"
	case "spec_list", "speclist":
		return "SpecList"
	case "spec_reset", "specreset":
		return "SpecReset"
	case "spec_config", "specconfig":
		return "SpecConfig"
	case "clarify":
		return "Clarify"
	case "analyze":
		return "Analyze"
	case "checklist":
		return "Checklist"
	case "constitution":
		return "Constitution"
	case "converge":
		return "Converge"
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

// permissionAuditLog is a fixed-size ring buffer of recent permission
// decisions, surfaced via "/autonomy audit". It is safe for concurrent use.
type permissionAuditLog struct {
	mu      sync.Mutex
	entries []auditEntry
	head    int
	full    bool
}

// auditEntry records one permission decision for the audit trail.
type auditEntry struct {
	Time    time.Time
	Tool    string
	Summary string
	Outcome DecisionOutcome
	Reason  DecisionReason
}

// newPermissionAuditLog creates a ring buffer holding the most recent cap
// decisions.
func newPermissionAuditLog(cap int) *permissionAuditLog {
	if cap <= 0 {
		cap = 256
	}
	return &permissionAuditLog{entries: make([]auditEntry, cap)}
}

// record appends a decision to the ring buffer.
func (l *permissionAuditLog) record(tool, summary string, outcome DecisionOutcome, reason DecisionReason) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries[l.head] = auditEntry{
		Time:    time.Now(),
		Tool:    tool,
		Summary: summary,
		Outcome: outcome,
		Reason:  reason,
	}
	l.head++
	if l.head >= len(l.entries) {
		l.head = 0
		l.full = true
	}
}

// Recent returns the most recent n entries in chronological order (oldest
// first). If n exceeds the buffer size, the entire buffer is returned.
func (l *permissionAuditLog) Recent(n int) []auditEntry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	size := len(l.entries)
	if !l.full {
		size = l.head
	}
	if n <= 0 || n > size {
		n = size
	}
	out := make([]auditEntry, 0, n)
	// Oldest entry index.
	start := l.head - n
	if start < 0 {
		start += len(l.entries)
	}
	for i := 0; i < n; i++ {
		idx := (start + i) % len(l.entries)
		out = append(out, l.entries[idx])
	}
	return out
}

// Format returns a human-readable audit trail for display.
func (l *permissionAuditLog) Format(n int) string {
	if l == nil {
		return "Audit log disabled."
	}
	entries := l.Recent(n)
	if len(entries) == 0 {
		return "No permission decisions recorded yet."
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Permission Audit (last %d):\n", len(entries)))
	for _, e := range entries {
		b.WriteString(fmt.Sprintf("  [%s] %s %s → %s (%s)\n",
			e.Time.Format("15:04:05"), e.Tool, e.Summary, e.Outcome, e.Reason))
	}
	return b.String()
}
