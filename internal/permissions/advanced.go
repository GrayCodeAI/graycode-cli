package permissions

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Pre-compiled safe/unsafe patterns for performance.
var (
	safeGitRe    = regexp.MustCompile(`^git\s+(status|log|diff|show|branch)\b`)
	safeLsRe     = regexp.MustCompile(`^ls(\s+|$)`)
	safeCatRe    = regexp.MustCompile(`^cat\s+`)
	safeEchoRe   = regexp.MustCompile(`^echo(\s+|$)`)
	safePwdRe    = regexp.MustCompile(`^pwd(\s+|$)`)
	safeCdRe     = regexp.MustCompile(`^cd(\s+|$)`)
	safeGoRe     = regexp.MustCompile(`^go\s+(version|env|mod)\b`)
	safeNodeRe   = regexp.MustCompile(`^node\s+--version`)
	safePythonRe = regexp.MustCompile(`^python3?\s+--version`)

	unsafeRmRe   = regexp.MustCompile(`rm\s+-rf\s+/`)
	unsafeCurlRe = regexp.MustCompile(`curl\s+.*\|\s*(sh|bash)`)
	unsafeWgetRe = regexp.MustCompile(`wget\s+.*\|\s*(sh|bash)`)
	unsafeEvalRe = regexp.MustCompile(`eval\s+`)
	unsafeSudoRe = regexp.MustCompile(`sudo\s+`)

	// Read-only git subcommands that are safe to auto-approve.
	safeGitSubcommands = map[string]bool{
		"status": true,
		"log":    true,
		"diff":   true,
		"show":   true,
		"branch": true,
	}
)

// AutoModeState tracks auto-allow decisions for learning user preferences.
type AutoModeState struct {
	mu         sync.RWMutex
	allowList  map[string]bool // tool patterns that are always allowed
	denyList   map[string]bool // tool patterns that are always denied
	askHistory []AskRecord     // history of permission asks
}

// AskRecord records a permission decision.
type AskRecord struct {
	ToolName string
	Summary  string
	Allowed  bool
	Count    int
}

// NewAutoModeState creates a new auto-mode state.
func NewAutoModeState() *AutoModeState {
	return &AutoModeState{
		allowList: make(map[string]bool),
		denyList:  make(map[string]bool),
	}
}

// Record records a permission decision.
func (a *AutoModeState) Record(toolName, summary string, allowed bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := toolName + ":" + summary
	if allowed {
		a.allowList[key] = true
	} else {
		a.denyList[key] = true
	}

	// Update history
	found := false
	for i := range a.askHistory {
		if a.askHistory[i].ToolName == toolName && a.askHistory[i].Summary == summary {
			a.askHistory[i].Allowed = allowed
			a.askHistory[i].Count++
			found = true
			break
		}
	}
	if !found {
		a.askHistory = append(a.askHistory, AskRecord{ToolName: toolName, Summary: summary, Allowed: allowed, Count: 1})
	}
}

// ShouldAutoAllow checks if a tool should be automatically allowed.
func (a *AutoModeState) ShouldAutoAllow(toolName, summary string) (bool, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Check exact match
	key := toolName + ":" + summary
	if a.allowList[key] {
		return true, true
	}
	if a.denyList[key] {
		return false, true
	}

	// Check pattern match for Bash commands.
	// Deny is checked before allow at every level so a specific deny
	// (e.g. "go test ./secret") beats a broad allow (e.g. "go *").
	if toolName == "Bash" {
		trimmed := strings.TrimSpace(summary)

		// --- Deny checks (all deny mechanisms beat all allow mechanisms) ---
		// 1. Wildcard deny patterns.
		for pattern := range a.denyList {
			if strings.HasPrefix(pattern, "Bash:") {
				cmdPattern := strings.TrimPrefix(pattern, "Bash:")
				if matchBashPattern(cmdPattern, summary) {
					return false, true
				}
			}
		}
		// 2. Semantic deny (prefix-based command-family deny).
		if matched, allowed := a.semanticMatch(trimmed); matched && !allowed {
			return false, true
		}
		// 3. Git-specific hard-deny for destructive subcommands.
		if strings.HasPrefix(trimmed, "git ") && !isSafeGitCommand(summary) {
			// Only auto-deny if there's a broad "git *" allow that would
			// otherwise match. Specific git allows (e.g. "git status") are
			// checked in the allow pass below.
			if a.hasBroadAllow("git") {
				return false, true
			}
		}

		// --- Allow checks ---
		// 4. Wildcard allow patterns.
		for pattern := range a.allowList {
			if strings.HasPrefix(pattern, "Bash:") {
				cmdPattern := strings.TrimPrefix(pattern, "Bash:")
				if matchBashPattern(cmdPattern, summary) {
					return true, true
				}
			}
		}
		// 5. Semantic allow (prefix-based command-family allow).
		if matched, allowed := a.semanticMatch(trimmed); matched && allowed {
			return true, true
		}
	}

	return false, false
}

// semanticMatch performs prefix-based command-family matching. It extracts
// the command prefix (e.g. "go test" from "go test ./foo") and checks whether
// any learned pattern is a prefix of the command or vice versa. This enables
// "allow once, trust the family" behavior without requiring wildcards.
// Deny patterns are checked before allow patterns (deny beats allow).
func (a *AutoModeState) semanticMatch(cmd string) (matched, allowed bool) {
	// Extract the base command prefix (first 1-3 tokens).
	prefix := commandPrefix(cmd)

	// Check deny patterns first (deny beats allow).
	for pattern := range a.denyList {
		if !strings.HasPrefix(pattern, "Bash:") {
			continue
		}
		p := strings.TrimPrefix(pattern, "Bash:")
		p = strings.TrimSpace(p)
		p = strings.TrimSuffix(p, "*")
		p = strings.TrimSuffix(p, " ")
		if p == "" {
			continue
		}
		if strings.HasPrefix(cmd, p) || strings.HasPrefix(prefix, p) {
			return true, false
		}
	}
	// Then check allow patterns.
	for pattern := range a.allowList {
		if !strings.HasPrefix(pattern, "Bash:") {
			continue
		}
		p := strings.TrimPrefix(pattern, "Bash:")
		p = strings.TrimSpace(p)
		p = strings.TrimSuffix(p, "*")
		p = strings.TrimSuffix(p, " ")
		if p == "" {
			continue
		}
		// A prefix match only auto-allows when the command is "bounded":
		// the approved prefix must be followed by nothing, whitespace (more
		// args of the same command), or end-of-input — never by a shell
		// metacharacter. Otherwise approving "git status" would also approve
		// "git status; rm -rf /" or "git status && curl evil.com ...".
		if (strings.HasPrefix(cmd, p) || strings.HasPrefix(prefix, p)) && commandIsBounded(cmd, p) {
			return true, true
		}
	}
	return false, false
}

// commandIsBounded reports whether cmd, having matched approved prefix p, does
// not continue with a shell metacharacter that would start a new command. It
// permits the remainder to be empty, whitespace-led (further arguments), or a
// redirection that is part of the same simple command; it rejects ; && || | $
// ( and backtick, which all introduce a separate command.
func commandIsBounded(cmd, prefix string) bool {
	rest := strings.TrimPrefix(cmd, prefix)
	if rest == "" {
		return true
	}
	next := rest[0]
	// Whitespace continues the same command with more arguments.
	if next == ' ' || next == '\t' {
		return true
	}
	// Anything else (;, &, |, $, `(, newline, etc.) starts a new command.
	return false
}

// hasBroadAllow reports whether there's a wildcard allow pattern for the
// given command prefix (e.g. "git" matches "Bash:git *").
func (a *AutoModeState) hasBroadAllow(prefix string) bool {
	for pattern := range a.allowList {
		if !strings.HasPrefix(pattern, "Bash:") {
			continue
		}
		p := strings.TrimPrefix(pattern, "Bash:")
		p = strings.TrimSpace(p)
		p = strings.TrimSuffix(p, "*")
		p = strings.TrimSuffix(p, " ")
		if p == prefix {
			return true
		}
	}
	return false
}

// commandPrefix extracts the meaningful prefix of a command (the base
// subcommand without arguments). e.g. "go test ./foo" -> "go test".
func commandPrefix(cmd string) string {
	fields := strings.Fields(cmd)
	// Return first 2 tokens for multi-word commands (go test, npm install),
	// or 1 token for simple commands (ls, git).
	if len(fields) >= 2 {
		// Check for common multi-word prefixes.
		twoWord := fields[0] + " " + fields[1]
		multiWordPrefixes := []string{
			"go test", "go build", "go run", "npm test",
			"npm run", "npm install", "pip install", "git status", "git log",
			"git diff", "git show", "git branch", "cargo test", "cargo build",
			"docker build", "docker run", "make test", "bundle exec",
		}
		for _, mw := range multiWordPrefixes {
			if strings.EqualFold(twoWord, mw) {
				return twoWord
			}
		}
	}
	if len(fields) >= 1 {
		return fields[0]
	}
	return cmd
}

// Grants returns the auto-learned decisions as canonical Grant slice. Learned
// denies are included so UnifiedGrants can enforce deny > allow precedence over
// broad learned-allow patterns.
func (a *AutoModeState) Grants() []Grant {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var out []Grant
	for key := range a.allowList {
		tool, pattern := splitGrantKey(key)
		out = append(out, Grant{
			Tool:    tool,
			Pattern: pattern,
			Allow:   true,
			Source:  SourceAutoLearned,
			Label:   "learned",
		})
	}
	for key := range a.denyList {
		tool, pattern := splitGrantKey(key)
		out = append(out, Grant{
			Tool:    tool,
			Pattern: pattern,
			Allow:   false,
			Source:  SourceAutoLearned,
			Label:   "learned",
		})
	}
	return out
}

// splitGrantKey splits an AutoModeState key ("Bash:go test ./...") into tool and
// pattern. Keys without a ":" are treated as tool-wide ("Bash" → "Bash","*").
func splitGrantKey(key string) (tool, pattern string) {
	if idx := strings.Index(key, ":"); idx >= 0 {
		return key[:idx], key[idx+1:]
	}
	return key, "*"
}

// matchBashPattern checks if a bash command matches a pattern.
func matchBashPattern(pattern, command string) bool {
	// Simple prefix matching with wildcard support
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(command, prefix)
	}
	return pattern == command
}

// BypassKillswitch disables permission checks globally. It now supports
// per-category scoping and automatic expiry so the break-glass path is
// narrower and self-limiting. The bool methods (Enable/Disable/IsEnabled)
// remain for backward compat: Enable() scopes to all categories with no
// expiry (session-long); the new BypassGrant struct is the recommended API.
type BypassKillswitch struct {
	enabled bool
	// grant is the structured bypass (scope + expiry + reason). When nil the
	// bypass behaves as legacy (all categories, session-long).
	grant *BypassGrant
	mu    sync.RWMutex
}

// BypassGrant is a structured bypass with scope, expiry, and justification.
// Scope is a list of tool categories ("bash", "network", "filesystem"); empty
// means all categories. ExpiresAt is zero for session-long. Reason is a
// required justification surfaced in audit logs.
type BypassGrant struct {
	Enabled   bool      `json:"enabled"`
	Scope     []string  `json:"scope,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

// NewBypassKillswitch creates a new bypass killswitch.
func NewBypassKillswitch() *BypassKillswitch {
	return &BypassKillswitch{}
}

// Enable enables the bypass killswitch (legacy: all categories, session-long).
func (b *BypassKillswitch) Enable() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.enabled = true
}

// Disable disables the bypass killswitch.
func (b *BypassKillswitch) Disable() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.enabled = false
	b.grant = nil
}

// IsEnabled checks if the bypass killswitch is enabled (legacy compat).
func (b *BypassKillswitch) IsEnabled() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.enabled
}

// EnableScoped enables the bypass for the given scope with an optional expiry.
// A reason is required for audit. If expiresAt is zero, the bypass lasts for
// the session. Passing an empty scope enables all categories.
func (b *BypassKillswitch) EnableScoped(scope []string, expiresAt time.Time, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.enabled = true
	b.grant = &BypassGrant{
		Enabled:   true,
		Scope:     scope,
		ExpiresAt: expiresAt,
		Reason:    reason,
	}
}

// Grant returns a copy of the current bypass grant (nil if legacy/unset).
func (b *BypassKillswitch) Grant() *BypassGrant {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.grant == nil {
		return nil
	}
	g := *b.grant
	if len(b.grant.Scope) > 0 {
		g.Scope = append([]string(nil), b.grant.Scope...)
	}
	return &g
}

// IsExpired reports whether a time-bound bypass has expired. A session-long
// bypass (zero ExpiresAt) never expires.
func (g *BypassGrant) IsExpired(now time.Time) bool {
	return !g.ExpiresAt.IsZero() && !now.Before(g.ExpiresAt)
}

// Covers reports whether the bypass covers a tool category. Empty scope means
// all categories.
func (g *BypassGrant) Covers(category string) bool {
	if len(g.Scope) == 0 {
		return true
	}
	for _, s := range g.Scope {
		if s == category {
			return true
		}
	}
	return false
}

// toolCategory maps a tool name to a bypass scope category. Local to the
// permissions package (does not import safety to avoid a cycle).
// ToolCategory maps a tool name to a bypass scope category. Exported so the
// permission engine can scope bypass grants without importing safety.
func ToolCategory(toolName string) string {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "bash":
		return "bash"
	case "webfetch", "websearch", "browser", "screenshot", "download":
		return "network"
	case "write", "edit", "structurededit", "multiedit", "fileedit", "notebookedit", "delete":
		return "filesystem"
	default:
		return "other"
	}
}

// ShadowedRuleDetector detects when permission rules shadow each other.
type ShadowedRuleDetector struct{}

// DetectShadowedRules finds shadowed permission rules.
func (d *ShadowedRuleDetector) DetectShadowedRules(allowRules, denyRules []string) []string {
	var warnings []string
	for _, allow := range allowRules {
		for _, deny := range denyRules {
			if d.isShadowed(allow, deny) {
				warnings = append(warnings, fmt.Sprintf("allow rule %q is shadowed by deny rule %q", allow, deny))
			}
		}
	}
	return warnings
}

// isShadowed checks if an allow rule is shadowed by a deny rule.
func (d *ShadowedRuleDetector) isShadowed(allow, deny string) bool {
	// Parse rules
	allowTool, allowPattern := parseRule(allow)
	denyTool, denyPattern := parseRule(deny)

	// Same tool with broader deny pattern
	if allowTool == denyTool {
		if denyPattern == "*" && allowPattern != "*" {
			return true
		}
		if strings.HasPrefix(allowPattern, denyPattern) {
			return true
		}
	}
	return false
}

func parseRule(rule string) (tool, pattern string) {
	if idx := strings.Index(rule, "("); idx >= 0 && strings.HasSuffix(rule, ")") {
		return rule[:idx], rule[idx+1 : len(rule)-1]
	}
	if idx := strings.Index(rule, ":"); idx >= 0 {
		return rule[:idx], rule[idx+1:]
	}
	return rule, "*"
}

// Classifier classifies commands as safe or dangerous.
type Classifier struct {
	safePatterns   []*regexp.Regexp
	unsafePatterns []*regexp.Regexp
}

// NewClassifier creates a new permission classifier.
func NewClassifier() *Classifier {
	return &Classifier{
		safePatterns: []*regexp.Regexp{
			safeGitRe,
			safeLsRe,
			safeCatRe,
			safeEchoRe,
			safePwdRe,
			safeCdRe,
			safeGoRe,
			safeNodeRe,
			safePythonRe,
		},
		unsafePatterns: []*regexp.Regexp{
			unsafeRmRe,
			unsafeCurlRe,
			unsafeWgetRe,
			unsafeEvalRe,
			unsafeSudoRe,
		},
	}
}

// Classify classifies a command as safe, unsafe, or unknown.
// Compound commands (cd && git -C <path> status) are safe only when every
// segment is independently safe — matching common agent shapes in Docker
// sessions that bind-mount the host project at the same absolute path.
func (c *Classifier) Classify(command string) string {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return "unknown"
	}
	for _, re := range c.unsafePatterns {
		if re.MatchString(cmd) {
			return "unsafe"
		}
	}
	segments := splitSafeCommandSegments(cmd)
	if len(segments) == 0 {
		return "unknown"
	}
	for _, seg := range segments {
		if !c.isSafeSegment(seg) {
			return "unknown"
		}
	}
	return "safe"
}

func (c *Classifier) isSafeSegment(segment string) bool {
	seg := strings.TrimSpace(unwrapShell(segment))
	if seg == "" {
		return true
	}
	if isSafeGitCommand(seg) {
		return true
	}
	for _, re := range c.safePatterns {
		if re.MatchString(seg) {
			return true
		}
	}
	return false
}

// splitSafeCommandSegments splits on && / ; while ignoring quoted separators.
func splitSafeCommandSegments(command string) []string {
	var (
		parts   []string
		current strings.Builder
		quote   rune
		escaped bool
	)
	flush := func() {
		part := strings.TrimSpace(current.String())
		current.Reset()
		if part != "" {
			parts = append(parts, part)
		}
	}
	runes := []rune(command)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if quote != 0 {
			current.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			current.WriteRune(r)
			continue
		}
		if r == ';' {
			flush()
			continue
		}
		if r == '&' && i+1 < len(runes) && runes[i+1] == '&' {
			flush()
			i++
			continue
		}
		current.WriteRune(r)
	}
	flush()
	return parts
}

// isSafeGitCommand reports whether cmd is a read-only git invocation, including
// forms like `git -C /abs/path status` and `/usr/bin/git status --porcelain`.
func isSafeGitCommand(cmd string) bool {
	tokens := tokenize(strings.TrimSpace(cmd))
	start := 0
	for start < len(tokens) && envPrefixRe.MatchString(tokens[start]) {
		start++
	}
	if start >= len(tokens) {
		return false
	}
	if normalizePath(tokens[start]) == "env" {
		start++
		for start < len(tokens) && envPrefixRe.MatchString(tokens[start]) {
			start++
		}
		if start >= len(tokens) {
			return false
		}
	}
	if normalizePath(stripQuotes(tokens[start])) != "git" {
		return false
	}
	i := start + 1
	for i < len(tokens) {
		tok := stripQuotes(tokens[i])
		switch {
		case tok == "-C" || tok == "-c":
			if i+1 >= len(tokens) {
				return false
			}
			i += 2
		case tok == "--git-dir" || tok == "--work-tree":
			if i+1 >= len(tokens) {
				return false
			}
			i += 2
		case strings.HasPrefix(tok, "--git-dir=") || strings.HasPrefix(tok, "--work-tree="):
			i++
		case strings.HasPrefix(tok, "-"):
			// Other global flags (e.g. --no-pager) take no path argument.
			i++
		default:
			return safeGitSubcommands[tok]
		}
	}
	return false
}
