package code

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

type CodeAction struct {
	ID          string
	Title       string
	Description string
	File        string
	Line        int
	Category    string
	Priority    int
	Fix         string
	Confidence  float64
}

type ActionDetector struct {
	Rules []ActionRule
	mu    sync.RWMutex
}

type ActionRule struct {
	ID          string
	Name        string
	Category    string
	Language    string
	Pattern     *regexp.Regexp
	Antipattern *regexp.Regexp
	Priority    int
	Message     string
	FixTemplate string
}

func NewActionDetector() *ActionDetector {
	ad := &ActionDetector{
		Rules: builtinRules(),
	}
	return ad
}

func builtinRules() []ActionRule {
	return []ActionRule{
		{
			ID:          "go-err-wrap",
			Name:        "Wrap error with context",
			Category:    "refactor",
			Language:    "go",
			Pattern:     regexp.MustCompile(`if err != nil \{\s*return err\s*\}`),
			Antipattern: regexp.MustCompile(`fmt\.Errorf\(.+%w`),
			Priority:    3,
			Message:     "Wrap error with context using fmt.Errorf",
			FixTemplate: `return fmt.Errorf("{{.FuncName}}: %w", err)`,
		},
		{
			ID:          "go-interface-any",
			Name:        "Use any instead of interface{}",
			Category:    "refactor",
			Language:    "go",
			Pattern:     regexp.MustCompile(`interface\{\}`),
			Priority:    5,
			Message:     "Use 'any' instead of 'interface{}' (Go 1.18+)",
			FixTemplate: "any",
		},
		{
			ID:          "go-ioutil-readfile",
			Name:        "Replace deprecated ioutil.ReadFile",
			Category:    "refactor",
			Language:    "go",
			Pattern:     regexp.MustCompile(`ioutil\.ReadFile`),
			Priority:    3,
			Message:     "ioutil.ReadFile is deprecated; use os.ReadFile",
			FixTemplate: "os.ReadFile",
		},
		{
			ID:          "go-ioutil-readall",
			Name:        "Replace deprecated ioutil.ReadAll",
			Category:    "refactor",
			Language:    "go",
			Pattern:     regexp.MustCompile(`ioutil\.ReadAll`),
			Priority:    3,
			Message:     "ioutil.ReadAll is deprecated; use io.ReadAll",
			FixTemplate: "io.ReadAll",
		},
		{
			ID:          "go-ioutil-writefile",
			Name:        "Replace deprecated ioutil.WriteFile",
			Category:    "refactor",
			Language:    "go",
			Pattern:     regexp.MustCompile(`ioutil\.WriteFile`),
			Priority:    3,
			Message:     "ioutil.WriteFile is deprecated; use os.WriteFile",
			FixTemplate: "os.WriteFile",
		},
		{
			ID:       "go-naked-return",
			Name:     "Avoid naked returns in long functions",
			Category: "refactor",
			Language: "go",
			Pattern:  regexp.MustCompile(`(?m)^\s*return\s*$`),
			Priority: 5,
			Message:  "Naked return in function; consider using named returns explicitly",
		},
		{
			ID:          "go-append-loop",
			Name:        "Pre-allocate slice",
			Category:    "performance",
			Language:    "go",
			Pattern:     regexp.MustCompile(`for .+ range .+\{[^}]*append\(`),
			Antipattern: regexp.MustCompile(`make\(\[\]`),
			Priority:    3,
			Message:     "Append in loop without pre-allocation; consider make([]T, 0, n)",
			FixTemplate: "make([]T, 0, len(items))",
		},
		{
			ID:          "go-string-concat-loop",
			Name:        "Use strings.Builder for concatenation",
			Category:    "performance",
			Language:    "go",
			Pattern:     regexp.MustCompile(`for .+\{[^}]*\+=\s*".+"`),
			Priority:    3,
			Message:     "String concatenation in loop; use strings.Builder",
			FixTemplate: "var sb strings.Builder",
		},
		{
			ID:       "go-regexp-in-func",
			Name:     "Move regexp.MustCompile to package level",
			Category: "performance",
			Language: "go",
			Pattern:  regexp.MustCompile(`func .+\{[^}]*regexp\.MustCompile`),
			Priority: 1,
			Message:  "regexp.MustCompile inside function; move to package-level var",
		},
		{
			ID:          "go-map-no-ok",
			Name:        "Use comma-ok pattern for map access",
			Category:    "performance",
			Language:    "go",
			Pattern:     regexp.MustCompile(`[a-zA-Z_]\w*\[[^\]]+\]\s*[^,=]`),
			Antipattern: regexp.MustCompile(`,\s*ok\s*:?=`),
			Priority:    5,
			Message:     "Map access without ok check; consider comma-ok pattern",
		},
		{
			ID:          "py-bare-except",
			Name:        "Avoid bare except",
			Category:    "fix",
			Language:    "python",
			Pattern:     regexp.MustCompile(`(?m)^\s*except\s*:`),
			Antipattern: regexp.MustCompile(`except\s+\w`),
			Priority:    1,
			Message:     "Bare except catches all exceptions including SystemExit; use 'except Exception:'",
			FixTemplate: "except Exception:",
		},
		{
			ID:          "py-type-check",
			Name:        "Use isinstance for type checking",
			Category:    "refactor",
			Language:    "python",
			Pattern:     regexp.MustCompile(`type\(\w+\)\s*==\s*\w+`),
			Priority:    3,
			Message:     "Use isinstance(x, T) instead of type(x) == T",
			FixTemplate: "isinstance(x, T)",
		},
		{
			ID:          "py-mutable-default",
			Name:        "Avoid mutable default argument",
			Category:    "fix",
			Language:    "python",
			Pattern:     regexp.MustCompile(`def \w+\([^)]*=\s*(\[\]|\{\})`),
			Priority:    1,
			Message:     "Mutable default argument; use None and set inside function body",
			FixTemplate: "=None",
		},
		{
			ID:          "py-open-no-with",
			Name:        "Use context manager for file operations",
			Category:    "fix",
			Language:    "python",
			Pattern:     regexp.MustCompile(`(?m)^\s*\w+\s*=\s*open\(`),
			Antipattern: regexp.MustCompile(`with\s+open\(`),
			Priority:    1,
			Message:     "open() without 'with' statement; use context manager",
			FixTemplate: "with open(...) as f:",
		},
		{
			ID:       "py-string-format-percent",
			Name:     "Use f-string or .format()",
			Category: "style",
			Language: "python",
			Pattern:  regexp.MustCompile(`"[^"]*%[sd]" %`),
			Priority: 5,
			Message:  "Use f-string or .format() instead of % formatting",
		},
		{
			ID:          "ts-any-type",
			Name:        "Avoid any type",
			Category:    "refactor",
			Language:    "typescript",
			Pattern:     regexp.MustCompile(`:\s*any\b`),
			Priority:    3,
			Message:     "Avoid 'any' type; use a proper type or 'unknown'",
			FixTemplate: ": unknown",
		},
		{
			ID:          "ts-loose-equality-null",
			Name:        "Use strict equality for null checks",
			Category:    "fix",
			Language:    "typescript",
			Pattern:     regexp.MustCompile(`==\s*null`),
			Antipattern: regexp.MustCompile(`===\s*null`),
			Priority:    3,
			Message:     "Use === null or optional chaining instead of == null",
			FixTemplate: "=== null",
		},
		{
			ID:       "ts-console-log",
			Name:     "Remove console.log in production",
			Category: "style",
			Language: "typescript",
			Pattern:  regexp.MustCompile(`console\.log\(`),
			Priority: 5,
			Message:  "console.log in production code; use a logger or remove",
		},
		{
			ID:       "ts-callback-hell",
			Name:     "Refactor nested callbacks",
			Category: "refactor",
			Language: "typescript",
			Pattern:  regexp.MustCompile(`\.\s*then\([^)]*\.\s*then\(`),
			Priority: 3,
			Message:  "Nested .then() callbacks; consider async/await",
		},
		{
			ID:       "ts-non-null-assertion",
			Name:     "Avoid non-null assertion",
			Category: "fix",
			Language: "typescript",
			Pattern:  regexp.MustCompile(`\w+!\.\w+`),
			Priority: 3,
			Message:  "Non-null assertion operator; add proper null check",
		},
		{
			ID:       "todo-comment",
			Name:     "Address TODO/FIXME/HACK comment",
			Category: "refactor",
			Language: "",
			Pattern:  regexp.MustCompile(`(?i)(TODO|FIXME|HACK|XXX)\b`),
			Priority: 5,
			Message:  "TODO/FIXME/HACK comment found; consider addressing it",
		},
		{
			ID:       "magic-number",
			Name:     "Extract magic number to constant",
			Category: "style",
			Language: "",
			Pattern:  regexp.MustCompile(`(?m)(?:==|!=|<=|>=|<|>)\s*\d{3,}`),
			Priority: 5,
			Message:  "Magic number detected; extract to a named constant",
		},
		{
			ID:       "long-function",
			Name:     "Function is too long",
			Category: "refactor",
			Language: "",
			Pattern:  regexp.MustCompile(`(?s)(func |def |function )\w+[^{]*\{[^\}]{999,}\}`),
			Priority: 3,
			Message:  "Function exceeds 50 lines; consider splitting into smaller functions",
		},
		{
			ID:       "deep-nesting",
			Name:     "Reduce nesting depth",
			Category: "refactor",
			Language: "",
			Pattern:  regexp.MustCompile(`(?m)^(\t{5,}|\s{20,})\S`),
			Priority: 3,
			Message:  "Deep nesting (>4 levels); consider early returns or extracting functions",
		},
		{
			ID:       "hardcoded-credential",
			Name:     "Possible hardcoded credential",
			Category: "security",
			Language: "",
			Pattern:  regexp.MustCompile(`(?i)(password|secret|api_key|token)\s*[:=]\s*["'][^"']+["']`),
			Priority: 1,
			Message:  "Possible hardcoded credential; use environment variables or secret manager",
		},
		{
			ID:       "sql-injection",
			Name:     "Possible SQL injection",
			Category: "security",
			Language: "",
			Pattern:  regexp.MustCompile(`(?i)(SELECT|INSERT|UPDATE|DELETE)\s+.+\+\s*\w+`),
			Priority: 1,
			Message:  "String concatenation in SQL query; use parameterized queries",
		},
	}
}

func (ad *ActionDetector) Detect(path, content string) []CodeAction {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	lang := detectLanguageFromPath(path)
	lines := strings.Split(content, "\n")
	var actions []CodeAction

	for _, rule := range ad.Rules {
		if rule.Language != "" && rule.Language != lang {
			continue
		}

		if isMultilinePattern(rule) {
			matches := rule.Pattern.FindAllStringIndex(content, -1)
			for _, m := range matches {
				if rule.Antipattern != nil {
					start := m[0]
					if start > 200 {
						start = m[0] - 200
					} else {
						start = 0
					}
					end := m[1] + 200
					if end > len(content) {
						end = len(content)
					}
					ctx := content[start:end]
					if rule.Antipattern.MatchString(ctx) {
						continue
					}
				}

				line := countNewlines(content[:m[0]]) + 1
				fix := generateFix(rule, content[m[0]:m[1]])
				actions = append(actions, CodeAction{
					ID:          rule.ID,
					Title:       rule.Name,
					Description: rule.Message,
					File:        path,
					Line:        line,
					Category:    rule.Category,
					Priority:    rule.Priority,
					Fix:         fix,
					Confidence:  computeConfidence(rule),
				})
			}
			continue
		}

		for i, line := range lines {
			if !rule.Pattern.MatchString(line) {
				continue
			}
			if rule.Antipattern != nil && rule.Antipattern.MatchString(line) {
				continue
			}

			fix := generateFix(rule, rule.Pattern.FindString(line))
			actions = append(actions, CodeAction{
				ID:          rule.ID,
				Title:       rule.Name,
				Description: rule.Message,
				File:        path,
				Line:        i + 1,
				Category:    rule.Category,
				Priority:    rule.Priority,
				Fix:         fix,
				Confidence:  computeConfidence(rule),
			})
		}
	}

	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Priority != actions[j].Priority {
			return actions[i].Priority < actions[j].Priority
		}
		return actions[i].Line < actions[j].Line
	})

	return actions
}

func (ad *ActionDetector) DetectForDiff(diff string) []CodeAction {
	added := extractAddedLines(diff)
	if added.path == "" {
		return nil
	}
	return ad.Detect(added.path, added.content)
}

func FormatSuggestions(actions []CodeAction, maxDisplay int) string {
	if len(actions) == 0 {
		return ""
	}

	var sb strings.Builder
	file := actions[0].File
	sb.WriteString(fmt.Sprintf("Suggestions for %s:\n", file))

	count := len(actions)
	if maxDisplay > 0 && count > maxDisplay {
		count = maxDisplay
	}

	for i := 0; i < count; i++ {
		a := actions[i]
		icon := categoryIcon(a.Category)
		sb.WriteString(fmt.Sprintf("%s [%s] L%d: %s\n", icon, a.Category, a.Line, a.Title))
		if a.Fix != "" {
			sb.WriteString(fmt.Sprintf("   - %s\n", a.Fix))
		}
	}

	if len(actions) > count {
		sb.WriteString(fmt.Sprintf("   ... and %d more suggestions\n", len(actions)-count))
	}

	return sb.String()
}

func ApplyFix(action CodeAction, content string) (string, error) {
	if action.Fix == "" {
		return "", fmt.Errorf("no fix available for action %s", action.ID)
	}
	if action.Line <= 0 {
		return "", fmt.Errorf("invalid line number %d", action.Line)
	}

	lines := strings.Split(content, "\n")
	if action.Line > len(lines) {
		return "", fmt.Errorf("line %d exceeds file length %d", action.Line, len(lines))
	}

	idx := action.Line - 1
	original := lines[idx]

	detector := NewActionDetector()
	var rule *ActionRule
	for i := range detector.Rules {
		if detector.Rules[i].ID == action.ID {
			rule = &detector.Rules[i]
			break
		}
	}

	if rule != nil && rule.Pattern != nil {
		replaced := rule.Pattern.ReplaceAllString(original, action.Fix)
		if replaced != original {
			lines[idx] = replaced
			return strings.Join(lines, "\n"), nil
		}
	}

	indent := extractIndent(original)
	lines[idx] = indent + strings.TrimSpace(action.Fix)
	return strings.Join(lines, "\n"), nil
}

func detectLanguageFromPath(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".go"):
		return "go"
	case strings.HasSuffix(lower, ".py"):
		return "python"
	case strings.HasSuffix(lower, ".ts"), strings.HasSuffix(lower, ".tsx"):
		return "typescript"
	case strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".jsx"):
		return "typescript"
	case strings.HasSuffix(lower, ".rs"):
		return "rust"
	case strings.HasSuffix(lower, ".rb"):
		return "ruby"
	default:
		return ""
	}
}

func isMultilinePattern(rule ActionRule) bool {
	p := rule.Pattern.String()
	return strings.Contains(p, "(?s)") || strings.Contains(p, `\{[^}]*`) || strings.Contains(p, `\{[^\}]`)
}

func countNewlines(s string) int {
	return strings.Count(s, "\n")
}

func generateFix(rule ActionRule, matched string) string {
	if rule.FixTemplate == "" {
		return ""
	}

	if !strings.Contains(rule.FixTemplate, "{{") {
		return rule.FixTemplate
	}

	tmpl, err := template.New("fix").Parse(rule.FixTemplate)
	if err != nil {
		return rule.FixTemplate
	}

	data := map[string]string{
		"Matched":  matched,
		"FuncName": "operation",
	}

	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return rule.FixTemplate
	}
	return sb.String()
}

func computeConfidence(rule ActionRule) float64 {
	switch rule.Priority {
	case 1:
		return 0.9
	case 2:
		return 0.8
	case 3:
		return 0.7
	case 4:
		return 0.6
	default:
		return 0.5
	}
}

func categoryIcon(category string) string {
	switch category {
	case "refactor":
		return "FIX:"
	case "performance":
		return icons.Bolt()
	case "security":
		return "LOCK:"
	case "style":
		return "STYLE:"
	case "fix":
		return "BUG:"
	default:
		return icons.Brain()
	}
}

func extractIndent(line string) string {
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			return line[:i]
		}
	}
	return line
}

type diffLines struct {
	path    string
	content string
}

func extractAddedLines(diff string) diffLines {
	lines := strings.Split(diff, "\n")
	var path string
	var added []string
	var currentLine int

	for _, line := range lines {
		if strings.HasPrefix(line, "+++ b/") {
			path = strings.TrimPrefix(line, "+++ b/")
			continue
		}
		if strings.HasPrefix(line, "@@") {
			parts := strings.Split(line, "+")
			if len(parts) >= 2 {
				numStr := strings.Split(parts[1], ",")[0]
				n, err := strconv.Atoi(numStr)
				if err == nil {
					currentLine = n
				}
			}
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added = append(added, strings.TrimPrefix(line, "+"))
			currentLine++
		} else if !strings.HasPrefix(line, "-") {
			currentLine++
		}
	}

	_ = currentLine

	return diffLines{
		path:    path,
		content: strings.Join(added, "\n"),
	}
}
