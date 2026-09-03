package review

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/engine/diff"
)

// ReviewBot is a rule-based code review engine that produces structured
// feedback without requiring an LLM call.
//
// The built-in rule set (builtinReviewRules and the rule* constructors) lives
// in review_bot_rules.go; this file holds the engine, formatting, diff parsing,
// and shared helpers.
type ReviewBot struct {
	Rules    []ReviewRule
	Severity string // minimum severity to report: "error", "warning", "info"
	mu       sync.RWMutex
}

// ReviewRule defines a single review check that can be applied to code.
type ReviewRule struct {
	ID       string
	Name     string
	Category string // "security", "performance", "correctness", "style", "testing"
	Language string
	Check    func(file string, lines []string, diffLines []diff.DiffLine) []ReviewComment
}

// ReviewComment represents a single piece of review feedback.
type ReviewComment struct {
	File       string
	Line       int
	Severity   string // "error", "warning", "info"
	Category   string
	Message    string
	Suggestion string
	RuleID     string
}

// ReviewReport summarizes the results of a code review.
type ReviewReport struct {
	Comments      []ReviewComment
	FilesReviewed int
	IssuesFound   int
	BySeverity    map[string]int
	Duration      time.Duration
}

// severityLevel returns numeric weight for severity comparison.
func severityLevel(s string) int {
	switch s {
	case "error":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

// NewReviewBot creates a ReviewBot pre-loaded with 20+ built-in rules.
func NewReviewBot() *ReviewBot {
	rb := &ReviewBot{
		Severity: "info",
	}
	rb.Rules = builtinReviewRules()
	return rb
}

// ReviewDiff parses a unified diff and reviews only changed lines.
func (rb *ReviewBot) ReviewDiff(diffInput string) (*ReviewReport, error) {
	start := time.Now()
	files := parseReviewDiffFiles(diffInput)

	rb.mu.RLock()
	rules := rb.Rules
	minSev := rb.Severity
	rb.mu.RUnlock()

	var allComments []ReviewComment
	for _, f := range files {
		lines := strings.Split(f.content, "\n")
		for _, rule := range rules {
			if rule.Language != "" && !matchesLanguage(f.path, rule.Language) {
				continue
			}
			comments := rule.Check(f.path, lines, f.diffLines)
			allComments = append(allComments, comments...)
		}
	}

	filtered := FilterBySeverity(allComments, minSev)
	report := &ReviewReport{
		Comments:      filtered,
		FilesReviewed: len(files),
		IssuesFound:   len(filtered),
		BySeverity:    countBySeverity(filtered),
		Duration:      time.Since(start),
	}
	return report, nil
}

// ReviewFile reviews a full file's content.
func (rb *ReviewBot) ReviewFile(path, content string) (*ReviewReport, error) {
	start := time.Now()
	lines := strings.Split(content, "\n")

	// Build diff.DiffLine slice treating all lines as added.
	var diffLines []diff.DiffLine
	for i, l := range lines {
		diffLines = append(diffLines, diff.DiffLine{
			Type:      "add",
			Content:   l,
			NewLineNo: i + 1,
		})
	}

	rb.mu.RLock()
	rules := rb.Rules
	minSev := rb.Severity
	rb.mu.RUnlock()

	var allComments []ReviewComment
	for _, rule := range rules {
		if rule.Language != "" && !matchesLanguage(path, rule.Language) {
			continue
		}
		comments := rule.Check(path, lines, diffLines)
		allComments = append(allComments, comments...)
	}

	filtered := FilterBySeverity(allComments, minSev)
	report := &ReviewReport{
		Comments:      filtered,
		FilesReviewed: 1,
		IssuesFound:   len(filtered),
		BySeverity:    countBySeverity(filtered),
		Duration:      time.Since(start),
	}
	return report, nil
}

// FormatReport produces a human-readable summary of a review report.
func FormatReport(report *ReviewReport) string {
	if report == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Code Review (%d files, %d issues):\n", report.FilesReviewed, report.IssuesFound))
	sb.WriteString(strings.Repeat("─", 35))
	sb.WriteString("\n")

	for _, c := range report.Comments {
		icon := severityIcon(c.Severity)
		sb.WriteString(fmt.Sprintf("%s %s:%d [%s] %s\n", icon, c.File, c.Line, c.Category, c.Message))
		if c.Suggestion != "" {
			sb.WriteString(fmt.Sprintf("   Suggestion: %s\n", c.Suggestion))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// FormatInline produces GitHub-style inline review comments.
func FormatInline(comments []ReviewComment) string {
	var sb strings.Builder
	for _, c := range comments {
		sb.WriteString(fmt.Sprintf("### %s:%d\n", c.File, c.Line))
		sb.WriteString(fmt.Sprintf("**[%s]** %s — %s\n", strings.ToUpper(c.Severity), c.Category, c.Message))
		if c.Suggestion != "" {
			sb.WriteString(fmt.Sprintf("```suggestion\n%s\n```\n", c.Suggestion))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// FilterBySeverity returns only comments at or above the specified minimum severity.
func FilterBySeverity(comments []ReviewComment, minSeverity string) []ReviewComment {
	minLevel := severityLevel(minSeverity)
	var result []ReviewComment
	for _, c := range comments {
		if severityLevel(c.Severity) >= minLevel {
			result = append(result, c)
		}
	}
	return result
}

// ---------- helpers ----------

func severityIcon(s string) string {
	switch s {
	case "error":
		return "CRIT"
	case "warning":
		return "MED"
	case "info":
		return "LOW"
	default:
		return "-"
	}
}

func countBySeverity(comments []ReviewComment) map[string]int {
	m := map[string]int{"error": 0, "warning": 0, "info": 0}
	for _, c := range comments {
		m[c.Severity]++
	}
	return m
}

func matchesLanguage(file, lang string) bool {
	ext := ""
	if idx := strings.LastIndex(file, "."); idx >= 0 {
		ext = file[idx:]
	}
	switch lang {
	case "go":
		return ext == ".go"
	case "python":
		return ext == ".py"
	case "javascript", "js":
		return ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx"
	case "java":
		return ext == ".java"
	case "ruby":
		return ext == ".rb"
	default:
		return true
	}
}

// reviewDiffFile represents a parsed file from a unified diff for review.
type reviewDiffFile struct {
	path      string
	content   string
	diffLines []diff.DiffLine
}

// parseReviewDiffFiles extracts file contents and diff lines from a unified diff string.
func parseReviewDiffFiles(diffInput string) []reviewDiffFile {
	var files []reviewDiffFile
	sections := strings.Split(diffInput, "diff --git")
	for _, section := range sections {
		if strings.TrimSpace(section) == "" {
			continue
		}
		f := parseReviewDiffSection(section)
		if f.path != "" {
			files = append(files, f)
		}
	}
	return files
}

func parseReviewDiffSection(section string) reviewDiffFile {
	lines := strings.Split(section, "\n")
	var f reviewDiffFile

	// Extract file path from +++ line.
	for _, l := range lines {
		if strings.HasPrefix(l, "+++ b/") {
			f.path = strings.TrimPrefix(l, "+++ b/")
			break
		} else if strings.HasPrefix(l, "+++ ") {
			f.path = strings.TrimPrefix(l, "+++ ")
			break
		}
	}

	// Parse hunks for content.
	var contentLines []string
	contentLineNo := 0
	inHunk := false
	for _, l := range lines {
		if strings.HasPrefix(l, "@@") {
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		if strings.HasPrefix(l, "+") {
			contentLineNo++
			content := strings.TrimPrefix(l, "+")
			contentLines = append(contentLines, content)
			f.diffLines = append(f.diffLines, diff.DiffLine{
				Type:      "add",
				Content:   content,
				NewLineNo: contentLineNo,
			})
		} else if strings.HasPrefix(l, "-") {
			content := strings.TrimPrefix(l, "-")
			f.diffLines = append(f.diffLines, diff.DiffLine{
				Type:    "remove",
				Content: content,
			})
		} else if strings.HasPrefix(l, " ") {
			contentLineNo++
			content := strings.TrimPrefix(l, " ")
			contentLines = append(contentLines, content)
			f.diffLines = append(f.diffLines, diff.DiffLine{
				Type:      "context",
				Content:   content,
				NewLineNo: contentLineNo,
			})
		}
	}
	f.content = strings.Join(contentLines, "\n")
	return f
}

// isChangedLine returns true if the given line number appears as an added line in the diff.
func isChangedLine(lineNo int, diff []diff.DiffLine) bool {
	for _, d := range diff {
		if d.Type == "add" && d.NewLineNo == lineNo {
			return true
		}
	}
	return false
}

// ---------- utility ----------

func parseParams(paramStr string) []string {
	var names []string
	parts := strings.Split(paramStr, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		fields := strings.Fields(p)
		if len(fields) >= 1 {
			name := fields[0]
			// Skip if it looks like a type only (e.g., "int", "string").
			if len(fields) >= 2 || strings.Contains(name, ".") {
				if len(fields) >= 2 {
					names = append(names, fields[0])
				}
			}
		}
	}
	return names
}

func removeStrings(line string) string {
	// Remove content between quotes.
	re := regexp.MustCompile(`"[^"]*"`)
	return re.ReplaceAllString(line, `""`)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
