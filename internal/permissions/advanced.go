package permissions

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
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

	// Check pattern match for Bash commands
	if toolName == "Bash" {
		for pattern := range a.allowList {
			if strings.HasPrefix(pattern, "Bash:") {
				cmdPattern := strings.TrimPrefix(pattern, "Bash:")
				if matchBashPattern(cmdPattern, summary) {
					// Narrow the auto-allow for git patterns: a broad
					// "Bash:git:*" must not auto-approve destructive git
					// subcommands like push --force / reset --hard / clean -f.
					// Only the safe read-only subcommands pass (Phase 3).
					if strings.HasPrefix(strings.TrimSpace(summary), "git ") && !isSafeGitCommand(summary) {
						return false, true
					}
					return true, true
				}
			}
		}
	}

	return false, false
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

// BypassKillswitch disables permission checks globally.
type BypassKillswitch struct {
	enabled bool
	mu      sync.RWMutex
}

// NewBypassKillswitch creates a new bypass killswitch.
func NewBypassKillswitch() *BypassKillswitch {
	return &BypassKillswitch{}
}

// Enable enables the bypass killswitch.
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
}

// IsEnabled checks if the bypass killswitch is enabled.
func (b *BypassKillswitch) IsEnabled() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.enabled
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
