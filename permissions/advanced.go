package permissions

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Pre-compiled safe/unsafe patterns for performance.
var (
	safeGitRe    = regexp.MustCompile(`^git\s+(status|log|diff|show|branch)`)
	safeLsRe     = regexp.MustCompile(`^ls\s+`)
	safeCatRe    = regexp.MustCompile(`^cat\s+`)
	safeEchoRe   = regexp.MustCompile(`^echo\s+`)
	safeGoRe     = regexp.MustCompile(`^go\s+(version|env|mod)`)
	safeNodeRe   = regexp.MustCompile(`^node\s+--version`)
	safePythonRe = regexp.MustCompile(`^python\s+--version`)

	unsafeRmRe   = regexp.MustCompile(`rm\s+-rf\s+/`)
	unsafeCurlRe = regexp.MustCompile(`curl\s+.*\|\s*(sh|bash)`)
	unsafeWgetRe = regexp.MustCompile(`wget\s+.*\|\s*(sh|bash)`)
	unsafeEvalRe = regexp.MustCompile(`eval\s+`)
	unsafeSudoRe = regexp.MustCompile(`sudo\s+`)
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
func (c *Classifier) Classify(command string) string {
	for _, re := range c.unsafePatterns {
		if re.MatchString(command) {
			return "unsafe"
		}
	}
	for _, re := range c.safePatterns {
		if re.MatchString(command) {
			return "safe"
		}
	}
	return "unknown"
}
