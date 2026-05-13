package engine

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ReviewBot is a rule-based code review engine that produces structured
// feedback without requiring an LLM call.
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
	Check    func(file string, lines []string, diff []DiffLine) []ReviewComment
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
func (rb *ReviewBot) ReviewDiff(diff string) (*ReviewReport, error) {
	start := time.Now()
	files := parseReviewDiffFiles(diff)

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

	// Build DiffLine slice treating all lines as added.
	var diffLines []DiffLine
	for i, l := range lines {
		diffLines = append(diffLines, DiffLine{
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
		return "\U0001f534"
	case "warning":
		return "\U0001f7e1"
	case "info":
		return "\U0001f7e2"
	default:
		return "⚪"
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
	diffLines []DiffLine
}

// parseReviewDiffFiles extracts file contents and diff lines from a unified diff string.
func parseReviewDiffFiles(diff string) []reviewDiffFile {
	var files []reviewDiffFile
	sections := strings.Split(diff, "diff --git")
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
	lineNo := 0
	inHunk := false
	for _, l := range lines {
		if strings.HasPrefix(l, "@@") {
			inHunk = true
			// Parse new file line number.
			parts := strings.Split(l, "+")
			if len(parts) >= 2 {
				numStr := strings.Split(parts[1], ",")[0]
				n := 0
				for _, ch := range numStr {
					if ch >= '0' && ch <= '9' {
						n = n*10 + int(ch-'0')
					} else {
						break
					}
				}
				if n > 0 {
					lineNo = n - 1
				}
			}
			continue
		}
		if !inHunk {
			continue
		}
		if strings.HasPrefix(l, "+") {
			lineNo++
			content := strings.TrimPrefix(l, "+")
			contentLines = append(contentLines, content)
			f.diffLines = append(f.diffLines, DiffLine{
				Type:      "add",
				Content:   content,
				NewLineNo: lineNo,
			})
		} else if strings.HasPrefix(l, "-") {
			content := strings.TrimPrefix(l, "-")
			f.diffLines = append(f.diffLines, DiffLine{
				Type:    "remove",
				Content: content,
			})
		} else if strings.HasPrefix(l, " ") {
			lineNo++
			content := strings.TrimPrefix(l, " ")
			contentLines = append(contentLines, content)
			f.diffLines = append(f.diffLines, DiffLine{
				Type:      "context",
				Content:   content,
				NewLineNo: lineNo,
			})
		}
	}
	f.content = strings.Join(contentLines, "\n")
	return f
}

// isChangedLine returns true if the given line number appears as an added line in the diff.
func isChangedLine(lineNo int, diff []DiffLine) bool {
	for _, d := range diff {
		if d.Type == "add" && d.NewLineNo == lineNo {
			return true
		}
	}
	return false
}

// ---------- built-in rules ----------

func builtinReviewRules() []ReviewRule {
	return []ReviewRule{
		ruleHardcodedSecrets(),
		ruleSQLInjection(),
		ruleCommandInjection(),
		ruleXSS(),
		ruleNPlusOneQuery(),
		ruleUnboundedAllocation(),
		ruleStringConcatInLoop(),
		ruleErrorIgnored(),
		ruleNilDereferenceRisk(),
		ruleUnclosedResources(),
		ruleRaceCondition(),
		ruleExportedWithoutDocs(),
		ruleInconsistentNaming(),
		ruleMagicNumbers(),
		ruleTestWithoutAssertion(),
		ruleSkippedTests(),
		ruleTestFileWithoutTests(),
		ruleHardcodedIP(),
		ruleTODOsInCode(),
		ruleEmptyErrorHandler(),
		ruleDeferInLoop(),
		ruleUnusedParameter(),
	}
}

// --- Security rules ---

func ruleHardcodedSecrets() ReviewRule {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(password|secret|api_key|apikey|token|private_key)\s*[:=]\s*["']` + "[^\"']{8,}" + `["']`),
		regexp.MustCompile(`(?i)(aws_access_key_id|aws_secret_access_key)\s*[:=]\s*["']`),
		regexp.MustCompile(`(?i)-----BEGIN (RSA |EC )?PRIVATE KEY-----`),
		regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`),
		regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
	}
	return ReviewRule{
		ID:       "SEC001",
		Name:     "Hardcoded secret detected",
		Category: "security",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				for _, pat := range patterns {
					if pat.MatchString(line) {
						comments = append(comments, ReviewComment{
							File:       file,
							Line:       i + 1,
							Severity:   "error",
							Category:   "security",
							Message:    "Hardcoded secret detected",
							Suggestion: "Use environment variable instead",
							RuleID:     "SEC001",
						})
						break
					}
				}
			}
			return comments
		},
	}
}

func ruleSQLInjection() ReviewRule {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(fmt\.Sprintf|".*\+.*")\s*.*?(SELECT|INSERT|UPDATE|DELETE|DROP)\s`),
		regexp.MustCompile(`(?i)query\s*[:=].*\+\s*\w`),
		regexp.MustCompile(`(?i)(Exec|Query|QueryRow)\s*\(\s*fmt\.Sprintf`),
		regexp.MustCompile(`(?i)(Exec|Query|QueryRow)\s*\(\s*"[^"]*"\s*\+`),
	}
	return ReviewRule{
		ID:       "SEC002",
		Name:     "Potential SQL injection",
		Category: "security",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				for _, pat := range patterns {
					if pat.MatchString(line) {
						comments = append(comments, ReviewComment{
							File:       file,
							Line:       i + 1,
							Severity:   "error",
							Category:   "security",
							Message:    "Potential SQL injection via string concatenation",
							Suggestion: "Use parameterized queries with placeholder arguments",
							RuleID:     "SEC002",
						})
						break
					}
				}
			}
			return comments
		},
	}
}

func ruleCommandInjection() ReviewRule {
	pattern := regexp.MustCompile(`(?i)(exec\.Command|os\.system|subprocess\.(call|run|Popen))\s*\(.*\+`)
	patternFmt := regexp.MustCompile(`(?i)(exec\.Command|os\.system)\s*\(\s*fmt\.Sprintf`)
	return ReviewRule{
		ID:       "SEC003",
		Name:     "Potential command injection",
		Category: "security",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				if pattern.MatchString(line) || patternFmt.MatchString(line) {
					comments = append(comments, ReviewComment{
						File:       file,
						Line:       i + 1,
						Severity:   "error",
						Category:   "security",
						Message:    "Potential command injection via string concatenation",
						Suggestion: "Sanitize inputs or use argument lists instead of shell strings",
						RuleID:     "SEC003",
					})
				}
			}
			return comments
		},
	}
}

func ruleXSS() ReviewRule {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)innerHTML\s*=`),
		regexp.MustCompile(`(?i)document\.write\s*\(`),
		regexp.MustCompile(`(?i)fmt\.Fprintf\s*\(\s*w\s*,.*\+`),
		regexp.MustCompile(`(?i)template\.HTML\(`),
	}
	return ReviewRule{
		ID:       "SEC004",
		Name:     "Potential XSS vulnerability",
		Category: "security",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				for _, pat := range patterns {
					if pat.MatchString(line) {
						comments = append(comments, ReviewComment{
							File:       file,
							Line:       i + 1,
							Severity:   "error",
							Category:   "security",
							Message:    "Potential cross-site scripting (XSS) vulnerability",
							Suggestion: "Sanitize or escape user input before rendering",
							RuleID:     "SEC004",
						})
						break
					}
				}
			}
			return comments
		},
	}
}

// --- Performance rules ---

func ruleNPlusOneQuery() ReviewRule {
	queryPat := regexp.MustCompile(`(?i)(\.Query|\.QueryRow|\.Exec)\s*\(`)
	loopPat := regexp.MustCompile(`^\s*(for|range)\s`)
	return ReviewRule{
		ID:       "PERF001",
		Name:     "Potential N+1 query",
		Category: "performance",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			inLoop := false
			loopStart := 0
			braceDepth := 0
			for i, line := range lines {
				if loopPat.MatchString(line) {
					inLoop = true
					loopStart = i + 1
					braceDepth = 0
				}
				if inLoop {
					braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
					if braceDepth <= 0 && i > loopStart {
						inLoop = false
					}
				}
				if inLoop && queryPat.MatchString(line) && isChangedLine(i+1, diff) {
					comments = append(comments, ReviewComment{
						File:       file,
						Line:       i + 1,
						Severity:   "warning",
						Category:   "performance",
						Message:    "Database query inside loop (potential N+1 query)",
						Suggestion: "Batch queries or use a JOIN to fetch all data at once",
						RuleID:     "PERF001",
					})
				}
			}
			return comments
		},
	}
}

func ruleUnboundedAllocation() ReviewRule {
	pattern := regexp.MustCompile(`make\s*\(\s*\[\]\w+\s*,\s*\w+`)
	return ReviewRule{
		ID:       "PERF002",
		Name:     "Potentially unbounded allocation",
		Category: "performance",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				if pattern.MatchString(line) && !strings.Contains(line, "cap") {
					// Check if the size variable might be user-controlled.
					if strings.Contains(line, "len(") || strings.Contains(line, "req.") || strings.Contains(line, "input") {
						comments = append(comments, ReviewComment{
							File:       file,
							Line:       i + 1,
							Severity:   "warning",
							Category:   "performance",
							Message:    "Potentially unbounded allocation based on external input",
							Suggestion: "Add a maximum cap to prevent OOM from large inputs",
							RuleID:     "PERF002",
						})
					}
				}
			}
			return comments
		},
	}
}

func ruleStringConcatInLoop() ReviewRule {
	concatPat := regexp.MustCompile(`\w+\s*\+=\s*("|\w)`)
	loopPat := regexp.MustCompile(`^\s*(for|range)\s`)
	return ReviewRule{
		ID:       "PERF003",
		Name:     "String concatenation in loop",
		Category: "performance",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			inLoop := false
			loopStart := 0
			braceDepth := 0
			for i, line := range lines {
				if loopPat.MatchString(line) {
					inLoop = true
					loopStart = i + 1
					braceDepth = 0
				}
				if inLoop {
					braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
					if braceDepth <= 0 && i > loopStart {
						inLoop = false
					}
				}
				if inLoop && concatPat.MatchString(line) && isChangedLine(i+1, diff) {
					if strings.Contains(line, "string") || strings.Contains(line, `"`) || strings.Contains(line, "str") || strings.Contains(line, "result") || strings.Contains(line, "output") {
						comments = append(comments, ReviewComment{
							File:       file,
							Line:       i + 1,
							Severity:   "warning",
							Category:   "performance",
							Message:    "String concatenation inside loop",
							Suggestion: "Use strings.Builder for better performance",
							RuleID:     "PERF003",
						})
					}
				}
			}
			return comments
		},
	}
}

// --- Correctness rules ---

func ruleErrorIgnored() ReviewRule {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\w+,\s*_\s*:?=\s*\w+.*\(`),
		regexp.MustCompile(`^\s*\w+\.\w+\(.*\)\s*$`),
	}
	errFuncPat := regexp.MustCompile(`(?i)(write|close|flush|send|remove|delete|create)`)
	return ReviewRule{
		ID:       "CORR001",
		Name:     "Error return value ignored",
		Category: "correctness",
		Language: "go",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				// Pattern: val, _ := someFunc()
				if patterns[0].MatchString(line) && errFuncPat.MatchString(line) {
					comments = append(comments, ReviewComment{
						File:       file,
						Line:       i + 1,
						Severity:   "warning",
						Category:   "correctness",
						Message:    "Error return value ignored",
						Suggestion: "if err != nil { return err }",
						RuleID:     "CORR001",
					})
				}
			}
			return comments
		},
	}
}

func ruleNilDereferenceRisk() ReviewRule {
	returnPat := regexp.MustCompile(`return\s+nil\s*,`)
	usePat := regexp.MustCompile(`(\w+)\s*,\s*err\s*:?=`)
	return ReviewRule{
		ID:       "CORR002",
		Name:     "Nil dereference risk",
		Category: "correctness",
		Language: "go",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			_ = returnPat
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				if usePat.MatchString(line) {
					// Check if next non-blank line uses the variable without nil check.
					if i+1 < len(lines) {
						nextLine := strings.TrimSpace(lines[i+1])
						varMatch := usePat.FindStringSubmatch(line)
						if len(varMatch) > 1 && !strings.Contains(nextLine, "err") && !strings.Contains(nextLine, "nil") && strings.Contains(nextLine, varMatch[1]+".") {
							comments = append(comments, ReviewComment{
								File:       file,
								Line:       i + 2,
								Severity:   "warning",
								Category:   "correctness",
								Message:    "Potential nil dereference — value used without checking error",
								Suggestion: "Check err != nil before using " + varMatch[1],
								RuleID:     "CORR002",
							})
						}
					}
				}
			}
			return comments
		},
	}
}

func ruleUnclosedResources() ReviewRule {
	openPat := regexp.MustCompile(`(os\.Open|sql\.Open|net\.Dial|http\.Get)\s*\(`)
	deferPat := regexp.MustCompile(`defer\s+\w+\.Close\(\)`)
	return ReviewRule{
		ID:       "CORR003",
		Name:     "Unclosed resource",
		Category: "correctness",
		Language: "go",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				if openPat.MatchString(line) {
					// Look for defer close within next 5 lines.
					found := false
					for j := i + 1; j < i+6 && j < len(lines); j++ {
						if deferPat.MatchString(lines[j]) || strings.Contains(lines[j], ".Close()") {
							found = true
							break
						}
					}
					if !found {
						comments = append(comments, ReviewComment{
							File:       file,
							Line:       i + 1,
							Severity:   "warning",
							Category:   "correctness",
							Message:    "Resource opened without visible defer Close()",
							Suggestion: "Add defer resource.Close() after error check",
							RuleID:     "CORR003",
						})
					}
				}
			}
			return comments
		},
	}
}

func ruleRaceCondition() ReviewRule {
	goPat := regexp.MustCompile(`^\s*go\s+\w+`)
	sharedPat := regexp.MustCompile(`(shared|global|counter|state)\w*\s*[\+\-]?=`)
	return ReviewRule{
		ID:       "CORR004",
		Name:     "Potential race condition",
		Category: "correctness",
		Language: "go",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			hasGoroutine := false
			for _, line := range lines {
				if goPat.MatchString(line) {
					hasGoroutine = true
					break
				}
			}
			if !hasGoroutine {
				return nil
			}
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				if sharedPat.MatchString(line) {
					// Check if there's a mutex lock nearby.
					hasMutex := false
					for j := maxInt(0, i-5); j < i; j++ {
						if strings.Contains(lines[j], "Lock()") || strings.Contains(lines[j], "RLock()") {
							hasMutex = true
							break
						}
					}
					if !hasMutex {
						comments = append(comments, ReviewComment{
							File:       file,
							Line:       i + 1,
							Severity:   "warning",
							Category:   "correctness",
							Message:    "Potential race condition — shared state modified without visible lock",
							Suggestion: "Protect with sync.Mutex or use atomic operations",
							RuleID:     "CORR004",
						})
					}
				}
			}
			return comments
		},
	}
}

// --- Style rules ---

func ruleExportedWithoutDocs() ReviewRule {
	exportedFunc := regexp.MustCompile(`^func\s+([A-Z]\w*)`)
	exportedMethod := regexp.MustCompile(`^func\s+\(\w+\s+\*?\w+\)\s+([A-Z]\w*)`)
	exportedType := regexp.MustCompile(`^type\s+([A-Z]\w*)`)
	commentPat := regexp.MustCompile(`^//\s*`)
	return ReviewRule{
		ID:       "STY001",
		Name:     "Exported symbol without documentation",
		Category: "style",
		Language: "go",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			// Skip test files.
			if strings.HasSuffix(file, "_test.go") {
				return nil
			}
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				var name string
				if m := exportedMethod.FindStringSubmatch(line); len(m) > 1 {
					name = m[1]
				} else if m := exportedFunc.FindStringSubmatch(line); len(m) > 1 {
					name = m[1]
				} else if m := exportedType.FindStringSubmatch(line); len(m) > 1 {
					name = m[1]
				}
				if name == "" {
					continue
				}
				// Check previous line for comment.
				hasDoc := false
				if i > 0 && commentPat.MatchString(strings.TrimSpace(lines[i-1])) {
					hasDoc = true
				}
				if !hasDoc {
					comments = append(comments, ReviewComment{
						File:       file,
						Line:       i + 1,
						Severity:   "info",
						Category:   "style",
						Message:    "Exported function missing documentation",
						Suggestion: fmt.Sprintf("Add godoc comment: // %s ...", name),
						RuleID:     "STY001",
					})
				}
			}
			return comments
		},
	}
}

func ruleInconsistentNaming() ReviewRule {
	snakePat := regexp.MustCompile(`\b[a-z]+_[a-z]+\b`)
	varDecl := regexp.MustCompile(`(var|:=)\s+`)
	return ReviewRule{
		ID:       "STY002",
		Name:     "Inconsistent naming convention",
		Category: "style",
		Language: "go",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				if varDecl.MatchString(line) && snakePat.MatchString(line) {
					// Ignore struct tags and strings.
					trimmed := removeStrings(line)
					if snakePat.MatchString(trimmed) {
						comments = append(comments, ReviewComment{
							File:       file,
							Line:       i + 1,
							Severity:   "info",
							Category:   "style",
							Message:    "Snake_case identifier in Go code (use camelCase)",
							Suggestion: "Rename to camelCase per Go conventions",
							RuleID:     "STY002",
						})
					}
				}
			}
			return comments
		},
	}
}

func ruleMagicNumbers() ReviewRule {
	numPat := regexp.MustCompile(`[^0-9\.]([2-9]\d{2,}|[1-9]\d{3,})([^0-9\.]|$)`)
	ignorePat := regexp.MustCompile(`(const|http\.|port|timeout|test|spec|0x|version|v\d)`)
	return ReviewRule{
		ID:       "STY003",
		Name:     "Magic number",
		Category: "style",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				if ignorePat.MatchString(line) {
					continue
				}
				if numPat.MatchString(line) {
					comments = append(comments, ReviewComment{
						File:       file,
						Line:       i + 1,
						Severity:   "info",
						Category:   "style",
						Message:    "Magic number — consider extracting as a named constant",
						Suggestion: "Define a const with a descriptive name",
						RuleID:     "STY003",
					})
				}
			}
			return comments
		},
	}
}

// --- Testing rules ---

func ruleTestWithoutAssertion() ReviewRule {
	testFunc := regexp.MustCompile(`^func\s+Test\w+\(`)
	assertion := regexp.MustCompile(`(assert\.|require\.|t\.(Error|Fatal|Fail|Check|Log)|if .* != |expect\(|should)`)
	return ReviewRule{
		ID:       "TEST001",
		Name:     "Test without assertion",
		Category: "testing",
		Language: "go",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			if !strings.HasSuffix(file, "_test.go") {
				return nil
			}
			var comments []ReviewComment
			inTest := false
			testStart := 0
			braceDepth := 0
			hasAssertion := false
			for i, line := range lines {
				if testFunc.MatchString(line) {
					if inTest && !hasAssertion && isChangedLine(testStart, diff) {
						comments = append(comments, ReviewComment{
							File:       file,
							Line:       testStart,
							Severity:   "warning",
							Category:   "testing",
							Message:    "Test function without any assertion",
							Suggestion: "Add assertions to validate expected behavior",
							RuleID:     "TEST001",
						})
					}
					inTest = true
					testStart = i + 1
					braceDepth = 0
					hasAssertion = false
				}
				if inTest {
					braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
					if assertion.MatchString(line) {
						hasAssertion = true
					}
					if braceDepth <= 0 && i > testStart {
						if !hasAssertion && isChangedLine(testStart, diff) {
							comments = append(comments, ReviewComment{
								File:       file,
								Line:       testStart,
								Severity:   "warning",
								Category:   "testing",
								Message:    "Test function without any assertion",
								Suggestion: "Add assertions to validate expected behavior",
								RuleID:     "TEST001",
							})
						}
						inTest = false
					}
				}
			}
			return comments
		},
	}
}

func ruleSkippedTests() ReviewRule {
	skipPat := regexp.MustCompile(`t\.Skip\(`)
	return ReviewRule{
		ID:       "TEST002",
		Name:     "Skipped test",
		Category: "testing",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			if !strings.HasSuffix(file, "_test.go") {
				return nil
			}
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				if skipPat.MatchString(line) {
					comments = append(comments, ReviewComment{
						File:       file,
						Line:       i + 1,
						Severity:   "info",
						Category:   "testing",
						Message:    "Test explicitly skipped",
						Suggestion: "Remove t.Skip() or add a TODO with timeline to re-enable",
						RuleID:     "TEST002",
					})
				}
			}
			return comments
		},
	}
}

func ruleTestFileWithoutTests() ReviewRule {
	testFunc := regexp.MustCompile(`^func\s+Test\w+\(`)
	return ReviewRule{
		ID:       "TEST003",
		Name:     "Test file without test functions",
		Category: "testing",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			if !strings.HasSuffix(file, "_test.go") {
				return nil
			}
			hasTest := false
			for _, line := range lines {
				if testFunc.MatchString(line) {
					hasTest = true
					break
				}
			}
			if !hasTest && len(diff) > 0 {
				return []ReviewComment{{
					File:       file,
					Line:       1,
					Severity:   "warning",
					Category:   "testing",
					Message:    "Test file does not contain any test functions",
					Suggestion: "Add test functions or remove the _test.go suffix",
					RuleID:     "TEST003",
				}}
			}
			return nil
		},
	}
}

// --- Additional rules ---

func ruleHardcodedIP() ReviewRule {
	ipPat := regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	ignorePat := regexp.MustCompile(`(127\.0\.0\.1|0\.0\.0\.0|localhost|test|example|spec)`)
	return ReviewRule{
		ID:       "SEC005",
		Name:     "Hardcoded IP address",
		Category: "security",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				if ipPat.MatchString(line) && !ignorePat.MatchString(line) {
					comments = append(comments, ReviewComment{
						File:       file,
						Line:       i + 1,
						Severity:   "warning",
						Category:   "security",
						Message:    "Hardcoded IP address",
						Suggestion: "Use configuration or environment variable for IP addresses",
						RuleID:     "SEC005",
					})
				}
			}
			return comments
		},
	}
}

func ruleTODOsInCode() ReviewRule {
	todoPat := regexp.MustCompile(`(?i)(TODO|FIXME|HACK|XXX|TEMP)\b`)
	return ReviewRule{
		ID:       "STY004",
		Name:     "TODO/FIXME comment",
		Category: "style",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				if todoPat.MatchString(line) {
					comments = append(comments, ReviewComment{
						File:       file,
						Line:       i + 1,
						Severity:   "info",
						Category:   "style",
						Message:    "TODO/FIXME comment — track in issue tracker",
						Suggestion: "Create an issue and reference its ID in the comment",
						RuleID:     "STY004",
					})
				}
			}
			return comments
		},
	}
}

func ruleEmptyErrorHandler() ReviewRule {
	catchPat := regexp.MustCompile(`if\s+err\s*!=\s*nil\s*\{`)
	return ReviewRule{
		ID:       "CORR005",
		Name:     "Empty error handler",
		Category: "correctness",
		Language: "go",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				if catchPat.MatchString(line) {
					// Check if the next line is just a closing brace.
					if i+1 < len(lines) {
						next := strings.TrimSpace(lines[i+1])
						if next == "}" || next == "// ignore" {
							comments = append(comments, ReviewComment{
								File:       file,
								Line:       i + 1,
								Severity:   "warning",
								Category:   "correctness",
								Message:    "Empty error handler — error is silently swallowed",
								Suggestion: "Log the error or return it to the caller",
								RuleID:     "CORR005",
							})
						}
					}
				}
			}
			return comments
		},
	}
}

func ruleDeferInLoop() ReviewRule {
	loopPat := regexp.MustCompile(`^\s*for\s`)
	deferPat := regexp.MustCompile(`^\s*defer\s`)
	return ReviewRule{
		ID:       "CORR006",
		Name:     "Defer inside loop",
		Category: "correctness",
		Language: "go",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			inLoop := false
			loopStart := 0
			braceDepth := 0
			for i, line := range lines {
				if loopPat.MatchString(line) {
					inLoop = true
					loopStart = i
					braceDepth = 0
				}
				if inLoop {
					braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
					if braceDepth <= 0 && i > loopStart {
						inLoop = false
					}
				}
				if inLoop && deferPat.MatchString(line) && isChangedLine(i+1, diff) {
					comments = append(comments, ReviewComment{
						File:       file,
						Line:       i + 1,
						Severity:   "warning",
						Category:   "correctness",
						Message:    "defer inside loop — deferred calls won't execute until function returns",
						Suggestion: "Move resource cleanup into the loop body or extract to a function",
						RuleID:     "CORR006",
					})
				}
			}
			return comments
		},
	}
}

func ruleUnusedParameter() ReviewRule {
	funcPat := regexp.MustCompile(`^func\s+(?:\(\w+\s+\*?\w+\)\s+)?\w+\(([^)]+)\)`)
	return ReviewRule{
		ID:       "STY005",
		Name:     "Potentially unused parameter",
		Category: "style",
		Language: "go",
		Check: func(file string, lines []string, diff []DiffLine) []ReviewComment {
			var comments []ReviewComment
			for i, line := range lines {
				if !isChangedLine(i+1, diff) {
					continue
				}
				m := funcPat.FindStringSubmatch(line)
				if len(m) < 2 {
					continue
				}
				params := parseParams(m[1])
				// Scan function body for parameter usage.
				braceDepth := 0
				bodyStart := i
				for j := i; j < len(lines); j++ {
					braceDepth += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
					if braceDepth > 0 {
						bodyStart = j + 1
						break
					}
				}
				bodyEnd := bodyStart
				bodyDepth := 1
				for j := bodyStart; j < len(lines); j++ {
					bodyDepth += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
					if bodyDepth <= 0 {
						bodyEnd = j
						break
					}
				}
				body := strings.Join(lines[bodyStart:minInt(bodyEnd, len(lines))], "\n")
				for _, p := range params {
					if p == "_" || p == "" {
						continue
					}
					if !strings.Contains(body, p) {
						comments = append(comments, ReviewComment{
							File:       file,
							Line:       i + 1,
							Severity:   "info",
							Category:   "style",
							Message:    fmt.Sprintf("Parameter '%s' appears unused in function body", p),
							Suggestion: "Remove unused parameter or prefix with _",
							RuleID:     "STY005",
						})
						break // report once per function
					}
				}
			}
			return comments
		},
	}
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
