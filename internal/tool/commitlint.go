package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

// CommitLinter validates commit messages against configurable rules following
// the commitlint specification.
type CommitLinter struct {
	Rules []CommitRule
	mu    sync.RWMutex
}

// CommitRule represents a single linting rule.
type CommitRule struct {
	Name       string
	Level      string      // "error", "warning", "disabled"
	Applicable string      // "always", "never"
	Value      interface{} // rule-specific value (int, string, []string, etc.)
}

// LintResult contains the outcome of linting a commit message.
type LintResult struct {
	Valid    bool
	Errors   []string
	Warnings []string
	Input    string
}

// ParsedCommit holds the parsed components of a conventional commit message.
type ParsedCommit struct {
	Type     string
	Scope    string
	Subject  string
	Body     string
	Footer   string
	Breaking bool
}

// defaultTypeEnum defines the allowed commit types.
var defaultTypeEnum = []string{
	"feat", "fix", "docs", "style", "refactor", "perf",
	"test", "build", "ci", "chore", "revert",
}

// NewCommitLinter creates a CommitLinter with default conventional commit rules.
func NewCommitLinter() *CommitLinter {
	return &CommitLinter{
		Rules: []CommitRule{
			{Name: "type-enum", Level: "error", Applicable: "always", Value: defaultTypeEnum},
			{Name: "type-case", Level: "error", Applicable: "always", Value: "lowercase"},
			{Name: "scope-case", Level: "error", Applicable: "always", Value: "lowercase"},
			{Name: "subject-max-length", Level: "error", Applicable: "always", Value: 72},
			{Name: "subject-empty", Level: "error", Applicable: "never", Value: nil},
			{Name: "body-max-line-length", Level: "warning", Applicable: "always", Value: 100},
			{Name: "header-max-length", Level: "error", Applicable: "always", Value: 100},
			{Name: "footer-max-line-length", Level: "warning", Applicable: "always", Value: 100},
		},
	}
}

// LoadFromProject reads commitlint configuration from a project directory.
// It looks for commitlint.config.js, .commitlintrc.json, and .commitlintrc.yml.
func (cl *CommitLinter) LoadFromProject(projectDir string) error {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	// Try .commitlintrc.json first (easiest to parse with stdlib).
	jsonPath := filepath.Join(projectDir, ".commitlintrc.json")
	// #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if data, err := os.ReadFile(jsonPath); err == nil {
		return cl.parseJSONConfig(data)
	}

	// Try commitlint.config.js (extract rules via regex).
	jsPath := filepath.Join(projectDir, "commitlint.config.js")
	// #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if data, err := os.ReadFile(jsPath); err == nil {
		return cl.parseJSConfig(data)
	}

	// Try .commitlintrc.yml (basic YAML parsing).
	ymlPath := filepath.Join(projectDir, ".commitlintrc.yml")
	// #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if data, err := os.ReadFile(ymlPath); err == nil {
		return cl.parseYAMLConfig(data)
	}

	// Also try .commitlintrc.yaml
	yamlPath := filepath.Join(projectDir, ".commitlintrc.yaml")
	// #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if data, err := os.ReadFile(yamlPath); err == nil {
		return cl.parseYAMLConfig(data)
	}

	return fmt.Errorf("no commitlint configuration found in %s", projectDir)
}

// Lint validates a commit message against the configured rules.
func (cl *CommitLinter) Lint(message string) *LintResult {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	result := &LintResult{
		Valid: true,
		Input: message,
	}

	if strings.TrimSpace(message) == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "commit message is empty")
		return result
	}

	parsed := ParseCommitMessage(message)

	for _, rule := range cl.Rules {
		if rule.Level == "disabled" {
			continue
		}

		violation := cl.checkRule(rule, parsed, message)
		if violation == "" {
			continue
		}

		switch rule.Level {
		case "error":
			result.Errors = append(result.Errors, violation)
			result.Valid = false
		case "warning":
			result.Warnings = append(result.Warnings, violation)
		}
	}

	return result
}

// ParseCommitMessage parses a commit message into its constituent parts.
func ParseCommitMessage(msg string) *ParsedCommit {
	parsed := &ParsedCommit{}

	msg = strings.TrimSpace(msg)
	if msg == "" {
		return parsed
	}

	// Split into header, body, footer.
	parts := strings.SplitN(msg, "\n\n", 3)
	header := parts[0]

	if len(parts) > 1 {
		// Check if the last part is a footer (contains trailer lines).
		if len(parts) == 3 {
			parsed.Body = strings.TrimSpace(parts[1])
			parsed.Footer = strings.TrimSpace(parts[2])
		} else {
			// Two parts: could be body or footer.
			candidate := strings.TrimSpace(parts[1])
			if isFooter(candidate) {
				parsed.Footer = candidate
			} else {
				parsed.Body = candidate
			}
		}
	}

	// Check for breaking change indicator.
	if strings.Contains(header, "!:") {
		parsed.Breaking = true
	}
	if strings.Contains(msg, "BREAKING CHANGE:") || strings.Contains(msg, "BREAKING-CHANGE:") {
		parsed.Breaking = true
	}

	// Parse header: type(scope): subject or type: subject or type!: subject
	headerPattern := regexp.MustCompile(`^([a-zA-Z]+)(?:\(([^)]*)\))?(!)?\s*:\s*(.*)$`)
	matches := headerPattern.FindStringSubmatch(header)
	if matches != nil {
		parsed.Type = matches[1]
		parsed.Scope = matches[2]
		if matches[3] == "!" {
			parsed.Breaking = true
		}
		parsed.Subject = matches[4]
	} else {
		// If header doesn't match conventional format, treat entire header as subject.
		colonIdx := strings.Index(header, ":")
		if colonIdx > 0 {
			parsed.Type = strings.TrimSpace(header[:colonIdx])
			parsed.Subject = strings.TrimSpace(header[colonIdx+1:])
		} else {
			parsed.Subject = header
		}
	}

	return parsed
}

// FixMessage auto-fixes common issues in a commit message.
func (cl *CommitLinter) FixMessage(message string) string {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	if strings.TrimSpace(message) == "" {
		return message
	}

	lines := strings.Split(message, "\n")
	header := lines[0]

	parsed := ParseCommitMessage(message)

	// Fix 1: Capitalize type -> lowercase.
	if parsed.Type != "" {
		lowered := strings.ToLower(parsed.Type)
		if lowered != parsed.Type {
			parsed.Type = lowered
		}
	}

	// Fix 1b: Scope -> lowercase.
	if parsed.Scope != "" {
		lowered := strings.ToLower(parsed.Scope)
		if lowered != parsed.Scope {
			parsed.Scope = lowered
		}
	}

	// Fix 2: Subject too long -> truncate.
	maxSubject := 72
	for _, rule := range cl.Rules {
		if rule.Name == "subject-max-length" {
			if v, ok := rule.Value.(int); ok {
				maxSubject = v
			}
		}
	}
	if len(parsed.Subject) > maxSubject {
		// Rune-safe truncation: never split a multibyte UTF-8 sequence.
		if runes := []rune(parsed.Subject); len(runes) > maxSubject {
			parsed.Subject = string(runes[:maxSubject-3]) + "..."
		}
	}

	// Fix 3: Missing type -> infer from body/subject.
	if parsed.Type == "" {
		parsed.Type = inferTypeFromContent(parsed.Subject, parsed.Body)
	}

	// Rebuild header.
	if parsed.Type != "" {
		if parsed.Scope != "" {
			header = fmt.Sprintf("%s(%s): %s", parsed.Type, parsed.Scope, parsed.Subject)
		} else {
			header = fmt.Sprintf("%s: %s", parsed.Type, parsed.Subject)
		}
	}

	// Enforce header-max-length.
	maxHeader := 100
	for _, rule := range cl.Rules {
		if rule.Name == "header-max-length" {
			if v, ok := rule.Value.(int); ok {
				maxHeader = v
			}
		}
	}
	if len(header) > maxHeader {
		// Rune-safe truncation: never split a multibyte UTF-8 sequence.
		if runes := []rune(header); len(runes) > maxHeader {
			header = string(runes[:maxHeader-3]) + "..."
		}
	}

	// Rebuild message.
	lines[0] = header
	return strings.Join(lines, "\n")
}

// FormatLintResult produces a human-readable lint report.
func FormatLintResult(result *LintResult) string {
	var sb strings.Builder

	if result.Valid {
		sb.WriteString(icons.CheckBold() + " Commit lint passed\n")
	} else {
		sb.WriteString(icons.Alert() + " Commit lint:\n")
	}

	for _, e := range result.Errors {
		sb.WriteString(fmt.Sprintf(icons.CloseThick()+" %s\n", e))
	}
	for _, w := range result.Warnings {
		sb.WriteString(fmt.Sprintf(icons.Alert()+" %s\n", w))
	}

	return sb.String()
}

// --- Internal rule checking ---

func (cl *CommitLinter) checkRule(rule CommitRule, parsed *ParsedCommit, rawMsg string) string {
	switch rule.Name {
	case "type-enum":
		return cl.checkTypeEnum(rule, parsed)
	case "type-case":
		return cl.checkTypeCase(rule, parsed)
	case "scope-case":
		return cl.checkScopeCase(rule, parsed)
	case "subject-max-length":
		return cl.checkSubjectMaxLength(rule, parsed)
	case "subject-empty":
		return cl.checkSubjectEmpty(rule, parsed)
	case "body-max-line-length":
		return cl.checkBodyMaxLineLength(rule, parsed)
	case "header-max-length":
		return cl.checkHeaderMaxLength(rule, parsed, rawMsg)
	case "footer-max-line-length":
		return cl.checkFooterMaxLineLength(rule, parsed)
	default:
		return ""
	}
}

func (cl *CommitLinter) checkTypeEnum(rule CommitRule, parsed *ParsedCommit) string {
	if parsed.Type == "" {
		return "type-enum: type is empty"
	}

	allowed, ok := rule.Value.([]string)
	if !ok {
		return ""
	}

	for _, t := range allowed {
		if parsed.Type == t {
			if rule.Applicable == "never" {
				return fmt.Sprintf("type-enum: %q must not be one of %v", parsed.Type, allowed)
			}
			return ""
		}
	}

	if rule.Applicable == "always" {
		return fmt.Sprintf("type-enum: %q is not in %v", parsed.Type, allowed)
	}
	return ""
}

func (cl *CommitLinter) checkTypeCase(rule CommitRule, parsed *ParsedCommit) string {
	if parsed.Type == "" {
		return ""
	}

	caseType, _ := rule.Value.(string)
	if caseType == "" {
		caseType = "lowercase"
	}

	switch caseType {
	case "lowercase":
		if parsed.Type != strings.ToLower(parsed.Type) {
			if rule.Applicable == "always" {
				return fmt.Sprintf("type-case: type %q must be lowercase", parsed.Type)
			}
		} else if rule.Applicable == "never" {
			return fmt.Sprintf("type-case: type %q must not be lowercase", parsed.Type)
		}
	case "uppercase":
		if parsed.Type != strings.ToUpper(parsed.Type) {
			if rule.Applicable == "always" {
				return fmt.Sprintf("type-case: type %q must be uppercase", parsed.Type)
			}
		}
	}
	return ""
}

func (cl *CommitLinter) checkScopeCase(rule CommitRule, parsed *ParsedCommit) string {
	if parsed.Scope == "" {
		return ""
	}

	caseType, _ := rule.Value.(string)
	if caseType == "" {
		caseType = "lowercase"
	}

	switch caseType {
	case "lowercase":
		if parsed.Scope != strings.ToLower(parsed.Scope) {
			if rule.Applicable == "always" {
				return fmt.Sprintf("scope-case: scope %q must be lowercase", parsed.Scope)
			}
		} else if rule.Applicable == "never" {
			return fmt.Sprintf("scope-case: scope %q must not be lowercase", parsed.Scope)
		}
	case "uppercase":
		if parsed.Scope != strings.ToUpper(parsed.Scope) {
			if rule.Applicable == "always" {
				return fmt.Sprintf("scope-case: scope %q must be uppercase", parsed.Scope)
			}
		}
	}
	return ""
}

func (cl *CommitLinter) checkSubjectMaxLength(rule CommitRule, parsed *ParsedCommit) string {
	maxLen := 72
	switch v := rule.Value.(type) {
	case int:
		maxLen = v
	case float64:
		maxLen = int(v)
	}

	if len(parsed.Subject) > maxLen {
		if rule.Applicable == "always" {
			return fmt.Sprintf("subject-max-length: subject is %d chars, max %d", len(parsed.Subject), maxLen)
		}
	}
	return ""
}

func (cl *CommitLinter) checkSubjectEmpty(rule CommitRule, parsed *ParsedCommit) string {
	isEmpty := strings.TrimSpace(parsed.Subject) == ""

	if rule.Applicable == "never" && isEmpty {
		return "subject-empty: subject must not be empty"
	}
	if rule.Applicable == "always" && !isEmpty {
		return "subject-empty: subject must be empty"
	}
	return ""
}

func (cl *CommitLinter) checkBodyMaxLineLength(rule CommitRule, parsed *ParsedCommit) string {
	if parsed.Body == "" {
		return ""
	}

	maxLen := 100
	switch v := rule.Value.(type) {
	case int:
		maxLen = v
	case float64:
		maxLen = int(v)
	}

	lines := strings.Split(parsed.Body, "\n")
	for _, line := range lines {
		if len(line) > maxLen {
			if rule.Applicable == "always" {
				return fmt.Sprintf("body-max-line-length: body line is %d chars, max %d", len(line), maxLen)
			}
		}
	}
	return ""
}

func (cl *CommitLinter) checkHeaderMaxLength(rule CommitRule, parsed *ParsedCommit, rawMsg string) string {
	maxLen := 100
	switch v := rule.Value.(type) {
	case int:
		maxLen = v
	case float64:
		maxLen = int(v)
	}

	// Header is the first line of the raw message.
	header := strings.Split(rawMsg, "\n")[0]
	if len(header) > maxLen {
		if rule.Applicable == "always" {
			return fmt.Sprintf("header-max-length: header is %d chars, max %d", len(header), maxLen)
		}
	}
	return ""
}

func (cl *CommitLinter) checkFooterMaxLineLength(rule CommitRule, parsed *ParsedCommit) string {
	if parsed.Footer == "" {
		return ""
	}

	maxLen := 100
	switch v := rule.Value.(type) {
	case int:
		maxLen = v
	case float64:
		maxLen = int(v)
	}

	lines := strings.Split(parsed.Footer, "\n")
	for _, line := range lines {
		if len(line) > maxLen {
			if rule.Applicable == "always" {
				return fmt.Sprintf("footer-max-line-length: footer line is %d chars, max %d", len(line), maxLen)
			}
		}
	}
	return ""
}

// --- Config parsing ---

func (cl *CommitLinter) parseJSONConfig(data []byte) error {
	var config struct {
		Extends []string                   `json:"extends"`
		Rules   map[string]json.RawMessage `json:"rules"`
	}

	// Try parsing extends as both string and []string.
	var rawConfig map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return fmt.Errorf("parsing commitlint JSON config: %w", err)
	}

	if extendsRaw, ok := rawConfig["extends"]; ok {
		var extendsStr string
		var extendsArr []string
		if json.Unmarshal(extendsRaw, &extendsStr) == nil {
			config.Extends = []string{extendsStr}
		} else if json.Unmarshal(extendsRaw, &extendsArr) == nil {
			config.Extends = extendsArr
		}
	}

	if rulesRaw, ok := rawConfig["rules"]; ok {
		if err := json.Unmarshal(rulesRaw, &config.Rules); err != nil {
			return fmt.Errorf("parsing commitlint rules: %w", err)
		}
	}

	// Apply extends (base rules).
	for _, ext := range config.Extends {
		if strings.Contains(ext, "config-conventional") {
			// Already using conventional defaults, no-op.
			break
		}
	}

	// Parse each rule: format is [level, applicable, value] or [level, applicable].
	for name, raw := range config.Rules {
		var tuple []json.RawMessage
		if err := json.Unmarshal(raw, &tuple); err != nil {
			continue
		}
		if len(tuple) < 2 {
			continue
		}

		rule := CommitRule{Name: name}

		// Parse level.
		var levelNum float64
		var levelStr string
		if json.Unmarshal(tuple[0], &levelNum) == nil {
			switch int(levelNum) {
			case 0:
				rule.Level = "disabled"
			case 1:
				rule.Level = "warning"
			case 2:
				rule.Level = "error"
			}
		} else if json.Unmarshal(tuple[0], &levelStr) == nil {
			rule.Level = levelStr
		}

		// Parse applicable.
		var applicable string
		if json.Unmarshal(tuple[1], &applicable) == nil {
			rule.Applicable = applicable
		}

		// Parse value if present.
		if len(tuple) > 2 {
			var strVal string
			var numVal float64
			var arrVal []string

			if json.Unmarshal(tuple[2], &arrVal) == nil {
				rule.Value = arrVal
			} else if json.Unmarshal(tuple[2], &strVal) == nil {
				rule.Value = strVal
			} else if json.Unmarshal(tuple[2], &numVal) == nil {
				rule.Value = int(numVal)
			}
		}

		cl.updateRule(rule)
	}

	return nil
}

func (cl *CommitLinter) parseJSConfig(data []byte) error {
	content := string(data)

	// Check for extends.
	extendsPattern := regexp.MustCompile(`extends\s*:\s*\[([^\]]*)\]`)
	if matches := extendsPattern.FindStringSubmatch(content); matches != nil {
		extends := matches[1]
		if strings.Contains(extends, "config-conventional") {
			// Keep defaults.
		}
	}

	// Parse rules object.
	rulesPattern := regexp.MustCompile(`rules\s*:\s*\{([^}]*(?:\{[^}]*\}[^}]*)*)\}`)
	rulesMatch := rulesPattern.FindStringSubmatch(content)
	if rulesMatch == nil {
		return nil // No rules found, use defaults.
	}

	rulesContent := rulesMatch[1]

	// Extract individual rules: 'rule-name': [level, applicable, value]
	// Use a pattern that handles nested brackets for array values.
	rulePattern := regexp.MustCompile(`['"]([^'"]+)['"]\s*:\s*\[`)
	ruleStarts := rulePattern.FindAllStringSubmatchIndex(rulesContent, -1)

	type ruleMatch struct {
		name      string
		valuePart string
	}
	var ruleMatches []ruleMatch
	for _, loc := range ruleStarts {
		name := rulesContent[loc[2]:loc[3]]
		// Find matching closing bracket, accounting for nesting.
		start := loc[1] // position after the opening '['
		depth := 1
		end := start
		for end < len(rulesContent) && depth > 0 {
			switch rulesContent[end] {
			case '[':
				depth++
			case ']':
				depth--
			}
			end++
		}
		if depth == 0 {
			ruleMatches = append(ruleMatches, ruleMatch{
				name:      name,
				valuePart: rulesContent[start : end-1],
			})
		}
	}

	for _, m := range ruleMatches {
		name := m.name
		valuePart := m.valuePart

		rule := CommitRule{Name: name}

		// Split by comma (simplified).
		parts := splitRuleValue(valuePart)
		if len(parts) < 2 {
			continue
		}

		// Level.
		level := strings.TrimSpace(parts[0])
		switch level {
		case "0":
			rule.Level = "disabled"
		case "1":
			rule.Level = "warning"
		case "2":
			rule.Level = "error"
		default:
			rule.Level = strings.Trim(level, "'\"")
		}

		// Applicable.
		rule.Applicable = strings.Trim(strings.TrimSpace(parts[1]), "'\"")

		// Value.
		if len(parts) > 2 {
			val := strings.TrimSpace(strings.Join(parts[2:], ","))
			rule.Value = parseJSValue(val)
		}

		cl.updateRule(rule)
	}

	return nil
}

func (cl *CommitLinter) parseYAMLConfig(data []byte) error {
	lines := strings.Split(string(data), "\n")

	inRules := false
	var currentRule string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for extends.
		if strings.HasPrefix(trimmed, "extends:") {
			if strings.Contains(trimmed, "config-conventional") {
				// Keep defaults.
			}
			continue
		}

		// Detect rules section.
		if trimmed == "rules:" {
			inRules = true
			continue
		}

		if !inRules {
			continue
		}

		// Detect new top-level key (end of rules).
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && !strings.HasPrefix(line, "  ") {
			break
		}

		// Rule name (indented key with colon).
		if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "-") {
			currentRule = strings.TrimSuffix(trimmed, ":")
			continue
		}

		// Simple inline rule: rule-name: [2, always, value]
		if strings.Contains(trimmed, ":") && strings.Contains(trimmed, "[") {
			colonIdx := strings.Index(trimmed, ":")
			ruleName := strings.TrimSpace(trimmed[:colonIdx])
			ruleValue := strings.TrimSpace(trimmed[colonIdx+1:])
			cl.parseYAMLRuleInline(ruleName, ruleValue)
			continue
		}

		_ = currentRule
	}

	return nil
}

func (cl *CommitLinter) parseYAMLRuleInline(name, value string) {
	// Parse [level, applicable, value] format.
	value = strings.Trim(value, "[]")
	parts := strings.SplitN(value, ",", 3)
	if len(parts) < 2 {
		return
	}

	rule := CommitRule{Name: name}

	level := strings.TrimSpace(parts[0])
	switch level {
	case "0":
		rule.Level = "disabled"
	case "1":
		rule.Level = "warning"
	case "2":
		rule.Level = "error"
	}

	rule.Applicable = strings.TrimSpace(parts[1])

	if len(parts) > 2 {
		val := strings.TrimSpace(parts[2])
		// Try to parse as number.
		var num int
		if _, err := fmt.Sscanf(val, "%d", &num); err == nil {
			rule.Value = num
		} else {
			rule.Value = strings.Trim(val, "'\"")
		}
	}

	cl.updateRule(rule)
}

func (cl *CommitLinter) updateRule(rule CommitRule) {
	for i, existing := range cl.Rules {
		if existing.Name == rule.Name {
			cl.Rules[i] = rule
			return
		}
	}
	cl.Rules = append(cl.Rules, rule)
}

// --- Helpers ---

func isFooter(text string) bool {
	// Footers typically contain "key: value" or "BREAKING CHANGE:" patterns.
	lines := strings.Split(text, "\n")
	trailerPattern := regexp.MustCompile(`^[A-Za-z-]+:\s`)
	breakingPattern := regexp.MustCompile(`^BREAKING[- ]CHANGE:`)
	for _, line := range lines {
		if trailerPattern.MatchString(line) || breakingPattern.MatchString(line) {
			return true
		}
	}
	return false
}

func inferTypeFromContent(subject, body string) string {
	combined := strings.ToLower(subject + " " + body)

	if strings.Contains(combined, "fix") || strings.Contains(combined, "bug") ||
		strings.Contains(combined, "patch") || strings.Contains(combined, "resolve") {
		return "fix"
	}
	if strings.Contains(combined, "test") || strings.Contains(combined, "spec") {
		return "test"
	}
	if strings.Contains(combined, "add") || strings.Contains(combined, "new") ||
		strings.Contains(combined, "feature") || strings.Contains(combined, "implement") {
		return "feat"
	}
	if strings.Contains(combined, "doc") || strings.Contains(combined, "readme") {
		return "docs"
	}
	if strings.Contains(combined, "refactor") || strings.Contains(combined, "restructure") {
		return "refactor"
	}
	if strings.Contains(combined, "format") || strings.Contains(combined, "lint") {
		return "style"
	}
	if strings.Contains(combined, "perf") || strings.Contains(combined, "optim") {
		return "perf"
	}

	return "chore"
}

func splitRuleValue(s string) []string {
	var parts []string
	depth := 0
	current := strings.Builder{}

	for _, ch := range s {
		switch ch {
		case '[':
			depth++
			current.WriteRune(ch)
		case ']':
			depth--
			current.WriteRune(ch)
		case ',':
			if depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func parseJSValue(val string) interface{} {
	val = strings.TrimSpace(val)

	// Array.
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		inner := val[1 : len(val)-1]
		items := strings.Split(inner, ",")
		var result []string
		for _, item := range items {
			item = strings.TrimSpace(item)
			item = strings.Trim(item, "'\"")
			if item != "" {
				result = append(result, item)
			}
		}
		return result
	}

	// Number.
	var num int
	if _, err := fmt.Sscanf(val, "%d", &num); err == nil {
		return num
	}

	// String.
	return strings.Trim(val, "'\"")
}

// isLowerCase checks if a string is all lowercase (ignoring non-letter chars).
func isLowerCase(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// Ensure isLowerCase is used (avoid unused import/func lint errors).
var _ = isLowerCase
