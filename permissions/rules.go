package permissions

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Action represents the permission action to take for a tool invocation.
type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
	ActionAsk   Action = "ask"
)

// Rule defines a single permission rule mapping a tool and argument pattern to an action.
type Rule struct {
	Tool    string `json:"tool"`    // tool name or "*" for all
	Pattern string `json:"pattern"` // glob pattern for arguments (e.g., "/tmp/*", "*.go", "go test*")
	Action  Action `json:"action"`
	Reason  string `json:"reason,omitempty"` // optional explanation
}

// RuleSet holds an ordered collection of permission rules.
type RuleSet struct {
	Rules []Rule
	mu    sync.RWMutex
}

// NewRuleSet creates a new empty RuleSet.
func NewRuleSet() *RuleSet {
	return &RuleSet{
		Rules: []Rule{},
	}
}

// LoadFromFile parses a .hawk/rules file and populates the RuleSet.
func (rs *RuleSet) LoadFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open rules file: %w", err)
	}
	defer f.Close()

	var rules []Rule
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		rule, err := ParseRuleLine(line)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNum, err)
		}
		rules = append(rules, *rule)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading rules file: %w", err)
	}

	rs.mu.Lock()
	rs.Rules = rules
	rs.mu.Unlock()
	return nil
}

// ParseRuleLine parses a single rule line into a Rule.
// Format: <action> <tool> <pattern>
// The pattern may be quoted to include spaces.
func ParseRuleLine(line string) (*Rule, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, fmt.Errorf("empty rule line")
	}

	// Parse the action.
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid rule format, expected: <action> <tool> <pattern>, got: %q", line)
	}

	actionStr := strings.ToLower(parts[0])
	var action Action
	switch actionStr {
	case "allow":
		action = ActionAllow
	case "deny":
		action = ActionDeny
	case "ask":
		action = ActionAsk
	default:
		return nil, fmt.Errorf("unknown action %q, expected allow/deny/ask", parts[0])
	}

	tool := parts[1]

	// Parse the pattern: everything after the tool name.
	// Find where the pattern starts in the original line.
	afterAction := strings.TrimSpace(line[len(parts[0]):])
	afterTool := strings.TrimSpace(afterAction[len(parts[1]):])

	pattern := parsePattern(afterTool)

	return &Rule{
		Tool:    tool,
		Pattern: pattern,
		Action:  action,
	}, nil
}

// parsePattern handles quoted and unquoted patterns.
func parsePattern(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// Evaluate checks the rules in order and returns the action for the given tool and args.
// First matching rule wins. Returns ActionAsk if no rules match.
func (rs *RuleSet) Evaluate(toolName string, args map[string]interface{}) Action {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	subject := extractSubject(toolName, args)

	for _, rule := range rs.Rules {
		if !matchTool(rule.Tool, toolName) {
			continue
		}
		if matchPattern(rule.Pattern, subject) {
			return rule.Action
		}
	}

	return ActionAsk
}

// matchTool checks if a rule's tool field matches the given tool name.
func matchTool(ruleTool, toolName string) bool {
	if ruleTool == "*" {
		return true
	}
	return strings.EqualFold(ruleTool, toolName)
}

// matchPattern checks if a pattern matches the given subject string.
// Supports "*" as match-everything, and filepath.Match glob patterns.
// For patterns ending with "*", also does prefix matching to handle
// cases like "go test*" matching "go test ./...".
func matchPattern(pattern, subject string) bool {
	if pattern == "*" {
		return true
	}

	// Try filepath.Match first for standard glob patterns.
	matched, err := filepath.Match(pattern, subject)
	if err == nil && matched {
		return true
	}

	// For patterns with a trailing *, do prefix matching.
	// filepath.Match requires * to not cross path separators, but
	// for command matching we want "go test*" to match "go test ./pkg".
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		if strings.HasPrefix(subject, prefix) {
			return true
		}
	}

	return false
}

// extractSubject determines what string to match against for a given tool and args.
func extractSubject(toolName string, args map[string]interface{}) string {
	switch strings.ToLower(toolName) {
	case "bash":
		if cmd, ok := args["command"]; ok {
			return fmt.Sprintf("%v", cmd)
		}
	case "read", "write":
		if p, ok := args["file_path"]; ok {
			return fmt.Sprintf("%v", p)
		}
		if p, ok := args["path"]; ok {
			return fmt.Sprintf("%v", p)
		}
	case "edit":
		if p, ok := args["file_path"]; ok {
			return fmt.Sprintf("%v", p)
		}
		if p, ok := args["path"]; ok {
			return fmt.Sprintf("%v", p)
		}
	case "grep", "glob", "ls":
		if p, ok := args["path"]; ok {
			return fmt.Sprintf("%v", p)
		}
		if p, ok := args["pattern"]; ok {
			return fmt.Sprintf("%v", p)
		}
	}

	// Fallback: try common argument names.
	for _, key := range []string{"command", "file_path", "path", "pattern"} {
		if v, ok := args[key]; ok {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

// AddRule appends a rule to the end of the RuleSet.
func (rs *RuleSet) AddRule(rule Rule) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.Rules = append(rs.Rules, rule)
}

// RemoveRule removes the rule at the given index.
func (rs *RuleSet) RemoveRule(index int) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if index < 0 || index >= len(rs.Rules) {
		return fmt.Errorf("index %d out of range [0, %d)", index, len(rs.Rules))
	}

	rs.Rules = append(rs.Rules[:index], rs.Rules[index+1:]...)
	return nil
}

// SaveToFile writes the rules to a file in the .hawk/rules format.
func (rs *RuleSet) SaveToFile(path string) error {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	// Ensure the parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create rules file: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, rule := range rs.Rules {
		line := formatRule(rule)
		if _, err := w.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("write rule: %w", err)
		}
	}
	return w.Flush()
}

// formatRule formats a Rule back into the textual rule line format.
func formatRule(rule Rule) string {
	pattern := rule.Pattern
	// Quote the pattern if it contains spaces.
	if strings.Contains(pattern, " ") {
		pattern = `"` + pattern + `"`
	}

	line := fmt.Sprintf("%-5s %s %s", string(rule.Action), rule.Tool, pattern)
	if rule.Reason != "" {
		line += " # " + rule.Reason
	}
	return line
}
