package streaming

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// FormatRule defines a single formatting rule that can be applied to responses.
type FormatRule struct {
	Name    string
	Pattern *regexp.Regexp
	Fix     func(string) string
	Enabled bool
}

// FormattedResponse holds the result of formatting a response.
type FormattedResponse struct {
	Original   string
	Formatted  string
	Changes    []string
	TokensDiff int
}

// ResponseFormatter post-processes LLM responses to ensure consistent
// formatting, fix common issues, and enhance readability.
type ResponseFormatter struct {
	Rules []FormatRule
	mu    sync.RWMutex
}

// NewResponseFormatter creates a ResponseFormatter with built-in rules.
func NewResponseFormatter() *ResponseFormatter {
	rf := &ResponseFormatter{}

	rf.Rules = []FormatRule{
		{
			Name:    "fix_unclosed_code_fences",
			Pattern: regexp.MustCompile("(?s)```"),
			Fix: func(text string) string {
				return FixCodeFences(text)
			},
			Enabled: true,
		},
		{
			Name:    "fix_code_fence_language_labels",
			Pattern: regexp.MustCompile("(?m)^```[A-Z]"),
			Fix: func(text string) string {
				re := regexp.MustCompile("(?m)^```([A-Z][a-zA-Z]*)")
				return re.ReplaceAllStringFunc(text, func(match string) string {
					return "```" + strings.ToLower(match[3:])
				})
			},
			Enabled: true,
		},
		{
			Name:    "remove_filler_prefixes",
			Pattern: regexp.MustCompile(`(?i)^(Sure[,!]?\s*|Certainly[,!]?\s*|Of course[,!]?\s*|Here'?s?\s)`),
			Fix: func(text string) string {
				return RemoveFluff(text)
			},
			Enabled: true,
		},
		{
			Name:    "remove_trailing_offers",
			Pattern: regexp.MustCompile(`(?i)(Let me know if|Is there anything else|Hope this helps|Feel free to ask)`),
			Fix: func(text string) string {
				re := regexp.MustCompile(`(?im)\n*(Let me know if[^\n]*|Is there anything else[^\n]*|Hope this helps[^\n]*!?|Feel free to ask[^\n]*)\s*$`)
				return strings.TrimRight(re.ReplaceAllString(text, ""), "\n") + "\n"
			},
			Enabled: true,
		},
		{
			Name:    "normalize_bullet_styles",
			Pattern: regexp.MustCompile(`(?m)^[\s]*([\*•])\s`),
			Fix: func(text string) string {
				return NormalizeLists(text)
			},
			Enabled: true,
		},
		{
			Name:    "fix_double_blank_lines",
			Pattern: regexp.MustCompile(`\n{3,}`),
			Fix: func(text string) string {
				re := regexp.MustCompile(`\n{3,}`)
				return re.ReplaceAllString(text, "\n\n")
			},
			Enabled: true,
		},
		{
			Name:    "fix_trailing_whitespace",
			Pattern: regexp.MustCompile(`(?m)[ \t]+$`),
			Fix: func(text string) string {
				re := regexp.MustCompile(`(?m)[ \t]+$`)
				return re.ReplaceAllString(text, "")
			},
			Enabled: true,
		},
		{
			Name:    "normalize_heading_levels",
			Pattern: regexp.MustCompile(`(?m)^###`),
			Fix: func(text string) string {
				return normalizeHeadings(text)
			},
			Enabled: true,
		},
		{
			Name:    "remove_self_referential",
			Pattern: regexp.MustCompile(`(?i)(As an AI|As a language model|As an artificial intelligence)`),
			Fix: func(text string) string {
				re := regexp.MustCompile(`(?im)^[^\n]*(?:As an AI|As a language model|As an artificial intelligence)[^\n]*\n?`)
				return re.ReplaceAllString(text, "")
			},
			Enabled: true,
		},
		{
			Name:    "fix_broken_markdown_links",
			Pattern: regexp.MustCompile(`\[[^\]]*\]\s+\(`),
			Fix: func(text string) string {
				return FixMarkdown(text)
			},
			Enabled: true,
		},
	}

	return rf
}

// Format applies all enabled rules to the response and tracks changes.
func (rf *ResponseFormatter) Format(response string) *FormattedResponse {
	rf.mu.RLock()
	defer rf.mu.RUnlock()

	result := &FormattedResponse{
		Original:  response,
		Formatted: response,
		Changes:   []string{},
	}

	for _, rule := range rf.Rules {
		if !rule.Enabled {
			continue
		}
		if rule.Pattern.MatchString(result.Formatted) {
			before := result.Formatted
			result.Formatted = rule.Fix(result.Formatted)
			if before != result.Formatted {
				result.Changes = append(result.Changes, describeChange(rule.Name, before, result.Formatted))
			}
		}
	}

	result.TokensDiff = EstimateTokenSavings(result.Original, result.Formatted)
	return result
}

// FixCodeFences counts opening/closing ``` and adds missing ones.
// It also ensures language labels on opening fences.
func FixCodeFences(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	inFence := false
	fenceCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if !inFence {
				inFence = true
				fenceCount++
			} else {
				inFence = false
				fenceCount++
			}
		}
		result = append(result, line)
	}

	// If we have an unclosed fence, close it
	if inFence {
		result = append(result, "```")
	}

	return strings.Join(result, "\n")
}

// RemoveFluff strips filler phrases that waste tokens.
func RemoveFluff(text string) string {
	// Remove common filler prefixes
	prefixes := []struct {
		pattern *regexp.Regexp
	}{
		{regexp.MustCompile(`(?i)^Sure[,!]?\s*I('ll| will| can)\s+`)},
		{regexp.MustCompile(`(?i)^Sure[,!]?\s+`)},
		{regexp.MustCompile(`(?i)^Certainly[,!]?\s*I('ll| will| can)\s+`)},
		{regexp.MustCompile(`(?i)^Certainly[,!]?\s+`)},
		{regexp.MustCompile(`(?i)^Of course[,!]?\s*I('ll| will| can)\s+`)},
		{regexp.MustCompile(`(?i)^Of course[,!]?\s+`)},
		{regexp.MustCompile(`(?i)^Here's\s+(the|a|an|my)\s+`)},
		{regexp.MustCompile(`(?i)^Here is\s+(the|a|an|my)\s+`)},
	}

	result := text
	for _, p := range prefixes {
		if p.pattern.MatchString(result) {
			result = p.pattern.ReplaceAllString(result, "")
			// Capitalize the first letter of what remains
			if len(result) > 0 {
				result = strings.ToUpper(result[:1]) + result[1:]
			}
			break
		}
	}

	return result
}

// NormalizeLists converts mixed bullet styles to consistent dashes
// and ensures proper indentation.
func NormalizeLists(text string) string {
	lines := strings.Split(text, "\n")
	var result []string

	bulletRe := regexp.MustCompile(`^(\s*)([\*•])\s`)

	for _, line := range lines {
		if bulletRe.MatchString(line) {
			line = bulletRe.ReplaceAllString(line, "${1}- ")
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// FixMarkdown fixes broken markdown links and unclosed bold/italic.
func FixMarkdown(text string) string {
	// Fix broken links: [text] (url) → [text](url)
	brokenLink := regexp.MustCompile(`\]\s+\(`)
	text = brokenLink.ReplaceAllString(text, "](")

	// Fix unclosed bold markers
	text = fixUnclosedMarkers(text, "**")

	// Fix unclosed italic markers (single *)
	text = fixUnclosedMarkers(text, "*")

	return text
}

// fixUnclosedMarkers closes unclosed markdown markers like ** or *.
func fixUnclosedMarkers(text string, marker string) string {
	lines := strings.Split(text, "\n")
	var result []string

	for _, line := range lines {
		count := strings.Count(line, marker)
		// For **, we need to handle it differently than *
		if marker == "*" {
			// Don't count ** as two *
			doubleCount := strings.Count(line, "**")
			count = count - (doubleCount * 2)
		}
		if count%2 != 0 {
			line = line + marker
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// normalizeHeadings ensures heading levels don't skip (e.g., h1 → h3 without h2).
func normalizeHeadings(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	headingRe := regexp.MustCompile(`^(#{1,6})\s`)

	maxLevel := 0
	// First pass: find what heading levels are actually used
	usedLevels := make(map[int]bool)
	for _, line := range lines {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			usedLevels[level] = true
			if level > maxLevel {
				maxLevel = level
			}
		}
	}

	// Build level mapping: compress gaps
	sortedLevels := []int{}
	for i := 1; i <= 6; i++ {
		if usedLevels[i] {
			sortedLevels = append(sortedLevels, i)
		}
	}

	levelMap := make(map[int]int)
	for i, level := range sortedLevels {
		levelMap[level] = i + 1
	}

	// Second pass: remap
	for _, line := range lines {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			oldLevel := len(m[1])
			newLevel := levelMap[oldLevel]
			line = strings.Repeat("#", newLevel) + line[oldLevel:]
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// EstimateTokenSavings estimates the difference in token count between
// original and formatted text. Uses a simple ~4 chars per token heuristic.
func EstimateTokenSavings(original, formatted string) int {
	origTokens := len(original) / 4
	fmtTokens := len(formatted) / 4
	return origTokens - fmtTokens
}

// AddRule adds a new formatting rule to the formatter.
func (rf *ResponseFormatter) AddRule(rule FormatRule) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.Rules = append(rf.Rules, rule)
}

// EnableRule enables a rule by name.
func (rf *ResponseFormatter) EnableRule(name string) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	for i := range rf.Rules {
		if rf.Rules[i].Name == name {
			rf.Rules[i].Enabled = true
			return
		}
	}
}

// DisableRule disables a rule by name.
func (rf *ResponseFormatter) DisableRule(name string) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	for i := range rf.Rules {
		if rf.Rules[i].Name == name {
			rf.Rules[i].Enabled = false
			return
		}
	}
}

// FormatChanges returns a human-readable summary of the changes made.
func FormatChanges(result *FormattedResponse) string {
	if len(result.Changes) == 0 {
		return "No changes applied."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Response formatted (%d changes):\n", len(result.Changes)))
	for _, change := range result.Changes {
		sb.WriteString(fmt.Sprintf("- %s\n", change))
	}
	if result.TokensDiff > 0 {
		sb.WriteString(fmt.Sprintf("Token savings: ~%d\n", result.TokensDiff))
	}
	return sb.String()
}

// describeChange generates a human-readable description of a rule application.
func describeChange(ruleName, before, after string) string {
	switch ruleName {
	case "fix_unclosed_code_fences":
		return "Fixed unclosed code fence"
	case "fix_code_fence_language_labels":
		return "Fixed code fence language label to lowercase"
	case "remove_filler_prefixes":
		return "Removed filler prefix"
	case "remove_trailing_offers":
		return "Removed trailing offer phrase"
	case "normalize_bullet_styles":
		count := countBulletChanges(before, after)
		return fmt.Sprintf("Normalized %d bullet points", count)
	case "fix_double_blank_lines":
		return "Fixed double blank lines"
	case "fix_trailing_whitespace":
		return "Fixed trailing whitespace"
	case "normalize_heading_levels":
		return "Normalized heading levels"
	case "remove_self_referential":
		return "Removed self-referential phrase"
	case "fix_broken_markdown_links":
		return "Fixed broken markdown links"
	default:
		return fmt.Sprintf("Applied rule: %s", ruleName)
	}
}

// countBulletChanges counts how many bullet styles were normalized.
func countBulletChanges(before, after string) int {
	bulletRe := regexp.MustCompile(`(?m)^[\s]*([\*•])\s`)
	matches := bulletRe.FindAllString(before, -1)
	return len(matches)
}
