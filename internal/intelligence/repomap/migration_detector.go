// Package repomap: migration_detector.go applies a set of MigrationRule
// patterns to a project and reports MigrationOpportunities - patterns that
// could be updated to a newer API, a more idiomatic construct, or a more
// secure/performant alternative. AutoFix can rewrite selected patterns
// in-place. Results feed the health score's "deprecated APIs" dimension.
package repomap

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// MigrationOpportunity represents a detected code pattern that could be updated.
type MigrationOpportunity struct {
	File        string
	Line        int
	OldPattern  string
	NewPattern  string
	Reason      string
	Priority    string // "high", "medium", "low"
	AutoFixable bool
	Category    string // "deprecated", "idiom", "security", "performance"
}

// MigrationDetector scans source files to identify migration opportunities.
type MigrationDetector struct {
	Rules []MigrationRule
	mu    sync.RWMutex
}

// MigrationRule defines a pattern to detect and suggest replacement.
type MigrationRule struct {
	ID          string
	Language    string
	OldPattern  *regexp.Regexp
	NewPattern  string
	Reason      string
	Priority    string
	AutoFixable bool
	Category    string
	Since       string // version when old pattern became deprecated
}

// NewMigrationDetector creates a MigrationDetector with built-in rules.
func NewMigrationDetector() *MigrationDetector {
	md := &MigrationDetector{}
	md.Rules = builtinRules()
	return md
}

func builtinRules() []MigrationRule {
	return []MigrationRule{
		// Go: ioutil deprecations (Go 1.16+)
		{
			ID:          "go-ioutil-readfile",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`ioutil\.ReadFile`),
			NewPattern:  "os.ReadFile",
			Reason:      "ioutil.ReadFile deprecated since Go 1.16",
			Priority:    "high",
			AutoFixable: true,
			Category:    "deprecated",
			Since:       "Go 1.16",
		},
		{
			ID:          "go-ioutil-writefile",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`ioutil\.WriteFile`),
			NewPattern:  "os.WriteFile",
			Reason:      "ioutil.WriteFile deprecated since Go 1.16",
			Priority:    "high",
			AutoFixable: true,
			Category:    "deprecated",
			Since:       "Go 1.16",
		},
		{
			ID:          "go-ioutil-tempdir",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`ioutil\.TempDir`),
			NewPattern:  "os.MkdirTemp",
			Reason:      "ioutil.TempDir deprecated since Go 1.16",
			Priority:    "high",
			AutoFixable: true,
			Category:    "deprecated",
			Since:       "Go 1.16",
		},
		{
			ID:          "go-ioutil-readall",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`ioutil\.ReadAll`),
			NewPattern:  "io.ReadAll",
			Reason:      "ioutil.ReadAll deprecated since Go 1.16",
			Priority:    "high",
			AutoFixable: true,
			Category:    "deprecated",
			Since:       "Go 1.16",
		},
		{
			ID:          "go-ioutil-tempfile",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`ioutil\.TempFile`),
			NewPattern:  "os.CreateTemp",
			Reason:      "ioutil.TempFile deprecated since Go 1.16",
			Priority:    "high",
			AutoFixable: true,
			Category:    "deprecated",
			Since:       "Go 1.16",
		},
		{
			ID:          "go-ioutil-readdir",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`ioutil\.ReadDir`),
			NewPattern:  "os.ReadDir",
			Reason:      "ioutil.ReadDir deprecated since Go 1.16",
			Priority:    "high",
			AutoFixable: true,
			Category:    "deprecated",
			Since:       "Go 1.16",
		},
		{
			ID:          "go-ioutil-nopclose",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`ioutil\.NopCloser`),
			NewPattern:  "io.NopCloser",
			Reason:      "ioutil.NopCloser deprecated since Go 1.16",
			Priority:    "high",
			AutoFixable: true,
			Category:    "deprecated",
			Since:       "Go 1.16",
		},
		{
			ID:          "go-ioutil-discard",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`ioutil\.Discard`),
			NewPattern:  "io.Discard",
			Reason:      "ioutil.Discard deprecated since Go 1.16",
			Priority:    "high",
			AutoFixable: true,
			Category:    "deprecated",
			Since:       "Go 1.16",
		},
		// Go: interface{} -> any (Go 1.18+)
		{
			ID:          "go-interface-any",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`interface\{\}`),
			NewPattern:  "any",
			Reason:      "interface{} can be replaced with any (Go 1.18+)",
			Priority:    "medium",
			AutoFixable: true,
			Category:    "idiom",
			Since:       "Go 1.18",
		},
		// Go: sort.Slice -> slices.Sort (Go 1.21+)
		{
			ID:          "go-sort-slice",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`sort\.Slice\(`),
			NewPattern:  "slices.Sort(",
			Reason:      "consider slices.Sort for type-safe sorting (Go 1.21+)",
			Priority:    "medium",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "Go 1.21",
		},
		// Go: sync.Mutex in struct without pointer (potential copy)
		{
			ID:          "go-mutex-value",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`mu\s+sync\.Mutex`),
			NewPattern:  "mu sync.Mutex (ensure struct is not copied)",
			Reason:      "sync.Mutex in struct without pointer may cause copy issues",
			Priority:    "medium",
			AutoFixable: false,
			Category:    "security",
			Since:       "",
		},
		// Go: strings.Title deprecated (Go 1.18+)
		{
			ID:          "go-strings-title",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`strings\.Title\(`),
			NewPattern:  "cases.Title(language.English).String(",
			Reason:      "strings.Title deprecated since Go 1.18",
			Priority:    "high",
			AutoFixable: false,
			Category:    "deprecated",
			Since:       "Go 1.18",
		},
		// Go: io/ioutil import
		{
			ID:          "go-import-ioutil",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`"io/ioutil"`),
			NewPattern:  `remove "io/ioutil" import`,
			Reason:      "io/ioutil package deprecated since Go 1.16",
			Priority:    "high",
			AutoFixable: false,
			Category:    "deprecated",
			Since:       "Go 1.16",
		},
		// Go: errors.New + fmt.Sprintf -> fmt.Errorf
		{
			ID:          "go-errors-sprintf",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`errors\.New\(fmt\.Sprintf\(`),
			NewPattern:  "fmt.Errorf(",
			Reason:      "use fmt.Errorf instead of errors.New(fmt.Sprintf(...))",
			Priority:    "low",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "",
		},
		// Go: context.Background in tests -> context.TODO or test-specific
		{
			ID:          "go-http-handle-deprecated",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`http\.HandleFunc\(`),
			NewPattern:  "http.NewServeMux().HandleFunc(",
			Reason:      "avoid DefaultServeMux for better isolation",
			Priority:    "low",
			AutoFixable: false,
			Category:    "security",
			Since:       "",
		},
		// Go: rand.Seed deprecated (Go 1.20+)
		{
			ID:          "go-rand-seed",
			Language:    "go",
			OldPattern:  regexp.MustCompile(`rand\.Seed\(`),
			NewPattern:  "remove rand.Seed (automatic since Go 1.20)",
			Reason:      "rand.Seed deprecated since Go 1.20",
			Priority:    "high",
			AutoFixable: false,
			Category:    "deprecated",
			Since:       "Go 1.20",
		},

		// Python rules
		{
			ID:          "py-os-path-join",
			Language:    "python",
			OldPattern:  regexp.MustCompile(`os\.path\.join\(`),
			NewPattern:  "pathlib.Path(...) / ...",
			Reason:      "prefer pathlib.Path for modern path handling",
			Priority:    "low",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "Python 3.4",
		},
		{
			ID:          "py-format-string",
			Language:    "python",
			OldPattern:  regexp.MustCompile(`"[^"]*"\s*\.format\(`),
			NewPattern:  "f-string",
			Reason:      "f-strings are more readable and performant",
			Priority:    "low",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "Python 3.6",
		},
		{
			ID:          "py-format-string-single",
			Language:    "python",
			OldPattern:  regexp.MustCompile(`'[^']*'\s*\.format\(`),
			NewPattern:  "f-string",
			Reason:      "f-strings are more readable and performant",
			Priority:    "low",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "Python 3.6",
		},
		{
			ID:          "py-dict-has-key",
			Language:    "python",
			OldPattern:  regexp.MustCompile(`\.has_key\(`),
			NewPattern:  "key in dict",
			Reason:      "dict.has_key() removed in Python 3",
			Priority:    "high",
			AutoFixable: false,
			Category:    "deprecated",
			Since:       "Python 3.0",
		},
		{
			ID:          "py-print-statement",
			Language:    "python",
			OldPattern:  regexp.MustCompile(`(?m)^print\s+[^(]`),
			NewPattern:  "print(...)",
			Reason:      "print statement removed in Python 3",
			Priority:    "high",
			AutoFixable: false,
			Category:    "deprecated",
			Since:       "Python 3.0",
		},
		{
			ID:          "py-urllib2",
			Language:    "python",
			OldPattern:  regexp.MustCompile(`import\s+urllib2`),
			NewPattern:  "import urllib.request",
			Reason:      "urllib2 removed in Python 3",
			Priority:    "high",
			AutoFixable: false,
			Category:    "deprecated",
			Since:       "Python 3.0",
		},
		{
			ID:          "py-raw-input",
			Language:    "python",
			OldPattern:  regexp.MustCompile(`raw_input\(`),
			NewPattern:  "input(",
			Reason:      "raw_input renamed to input in Python 3",
			Priority:    "high",
			AutoFixable: true,
			Category:    "deprecated",
			Since:       "Python 3.0",
		},
		{
			ID:          "py-type-comment",
			Language:    "python",
			OldPattern:  regexp.MustCompile(`#\s*type:\s*\(`),
			NewPattern:  "native type annotations",
			Reason:      "type comments superseded by native annotations",
			Priority:    "low",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "Python 3.5",
		},
		{
			ID:          "py-typing-optional",
			Language:    "python",
			OldPattern:  regexp.MustCompile(`typing\.Optional\[`),
			NewPattern:  "X | None",
			Reason:      "use X | None syntax (Python 3.10+)",
			Priority:    "low",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "Python 3.10",
		},

		// JavaScript/TypeScript rules
		{
			ID:          "js-var-usage",
			Language:    "javascript",
			OldPattern:  regexp.MustCompile(`(?m)^\s*var\s+`),
			NewPattern:  "const or let",
			Reason:      "var has function-scope issues; prefer const/let",
			Priority:    "medium",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "ES6",
		},
		{
			ID:          "js-require",
			Language:    "javascript",
			OldPattern:  regexp.MustCompile(`(?m)^(const|let|var)\s+\w+\s*=\s*require\(`),
			NewPattern:  "import ... from '...'",
			Reason:      "prefer ES modules import over CommonJS require",
			Priority:    "medium",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "ES6",
		},
		{
			ID:          "js-then-catch",
			Language:    "javascript",
			OldPattern:  regexp.MustCompile(`\.then\([^)]*\)\s*\.catch\(`),
			NewPattern:  "async/await with try/catch",
			Reason:      "async/await is more readable than .then().catch()",
			Priority:    "low",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "ES2017",
		},
		{
			ID:          "js-moment",
			Language:    "javascript",
			OldPattern:  regexp.MustCompile(`require\(['"]moment['"]\)|from\s+['"]moment['"]`),
			NewPattern:  "dayjs or native Intl",
			Reason:      "moment.js is in maintenance mode; use dayjs or Intl",
			Priority:    "medium",
			AutoFixable: false,
			Category:    "deprecated",
			Since:       "2020",
		},
		{
			ID:          "js-callback-hell",
			Language:    "javascript",
			OldPattern:  regexp.MustCompile(`function\s*\([^)]*err[^)]*\)\s*\{`),
			NewPattern:  "async/await or Promises",
			Reason:      "callback patterns can be replaced with async/await",
			Priority:    "low",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "ES2017",
		},
		{
			ID:          "ts-any-type",
			Language:    "typescript",
			OldPattern:  regexp.MustCompile(`:\s*any\b`),
			NewPattern:  "specific type or unknown",
			Reason:      "avoid any; use unknown or a specific type for type safety",
			Priority:    "low",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "",
		},
		{
			ID:          "js-substr",
			Language:    "javascript",
			OldPattern:  regexp.MustCompile(`\.substr\(`),
			NewPattern:  ".slice(",
			Reason:      "String.prototype.substr is deprecated",
			Priority:    "medium",
			AutoFixable: true,
			Category:    "deprecated",
			Since:       "ES2022",
		},
		{
			ID:          "js-arguments-object",
			Language:    "javascript",
			OldPattern:  regexp.MustCompile(`\barguments\[`),
			NewPattern:  "...rest parameters",
			Reason:      "use rest parameters instead of arguments object",
			Priority:    "low",
			AutoFixable: false,
			Category:    "idiom",
			Since:       "ES6",
		},
	}
}

// languageForFile returns the language identifier based on file extension.
func languageForFile(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".jsx", ".mjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	default:
		return ""
	}
}

// Scan walks projectDir and applies matching rules to each source file.
func (md *MigrationDetector) Scan(projectDir string) ([]MigrationOpportunity, error) {
	var allOpps []MigrationOpportunity
	var mu sync.Mutex

	err := filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible files
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		lang := languageForFile(path)
		if lang == "" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(projectDir, path)
		if relPath == "" {
			relPath = path
		}

		opps := md.ScanFile(relPath, string(content))
		if len(opps) > 0 {
			mu.Lock()
			allOpps = append(allOpps, opps...)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("migration scan failed: %w", err)
	}

	// Sort by priority then file
	sort.Slice(allOpps, func(i, j int) bool {
		pi := priorityRank(allOpps[i].Priority)
		pj := priorityRank(allOpps[j].Priority)
		if pi != pj {
			return pi < pj
		}
		if allOpps[i].File != allOpps[j].File {
			return allOpps[i].File < allOpps[j].File
		}
		return allOpps[i].Line < allOpps[j].Line
	})

	return allOpps, nil
}

func priorityRank(p string) int {
	switch p {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	default:
		return 3
	}
}

// ScanFile applies all matching rules to the given file content.
func (md *MigrationDetector) ScanFile(path, content string) []MigrationOpportunity {
	lang := languageForFile(path)
	if lang == "" {
		return nil
	}

	md.mu.RLock()
	rules := md.Rules
	md.mu.RUnlock()

	lines := strings.Split(content, "\n")
	var opps []MigrationOpportunity

	for _, rule := range rules {
		if !languageMatches(rule.Language, lang) {
			continue
		}
		for lineNum, line := range lines {
			if rule.OldPattern.MatchString(line) {
				opp := MigrationOpportunity{
					File:        path,
					Line:        lineNum + 1,
					OldPattern:  rule.OldPattern.String(),
					NewPattern:  rule.NewPattern,
					Reason:      rule.Reason,
					Priority:    rule.Priority,
					AutoFixable: rule.AutoFixable,
					Category:    rule.Category,
				}
				opps = append(opps, opp)
			}
		}
	}

	return opps
}

// languageMatches checks if the rule language applies to the file language.
// TypeScript files also match JavaScript rules.
func languageMatches(ruleLang, fileLang string) bool {
	if ruleLang == fileLang {
		return true
	}
	if fileLang == "typescript" && ruleLang == "javascript" {
		return true
	}
	return false
}

// FormatOpportunities produces a human-readable summary of migration opportunities.
func FormatOpportunities(opps []MigrationOpportunity) string {
	if len(opps) == 0 {
		return "Migration Opportunities (0 found): none detected."
	}

	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "Migration Opportunities (%d found):\n", len(opps))
	b.WriteString(strings.Repeat("═", 35))
	b.WriteString("\n")

	// Group by priority
	groups := map[string][]MigrationOpportunity{}
	for _, opp := range opps {
		groups[opp.Priority] = append(groups[opp.Priority], opp)
	}

	autoFixCount := 0
	for _, opp := range opps {
		if opp.AutoFixable {
			autoFixCount++
		}
	}

	for _, prio := range []string{"high", "medium", "low"} {
		items := groups[prio]
		if len(items) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(&b, "\n%s (%d):\n", strings.ToUpper(prio), len(items))
		for _, item := range items {
			// Extract short old pattern for display
			oldDisplay := shortPattern(item.OldPattern)
			reason := ""
			if item.Reason != "" {
				reason = " (" + item.Reason + ")"
			}
			fmt.Fprintf(&b, "  %s:%d — %s → %s%s\n",
				item.File, item.Line, oldDisplay, item.NewPattern, reason)
		}
	}

	_, _ = fmt.Fprintf(&b, "\nAuto-fixable: %d/%d\n", autoFixCount, len(opps))

	return b.String()
}

// shortPattern extracts a readable version of the regex pattern for display.
func shortPattern(pattern string) string {
	// Remove common regex escaping for display
	s := pattern
	s = strings.ReplaceAll(s, `\(`, "(")
	s = strings.ReplaceAll(s, `\)`, ")")
	s = strings.ReplaceAll(s, `\{`, "{")
	s = strings.ReplaceAll(s, `\}`, "}")
	s = strings.ReplaceAll(s, `\[`, "[")
	s = strings.ReplaceAll(s, `\]`, "]")
	s = strings.ReplaceAll(s, `\.`, ".")
	// Remove leading anchors and non-capturing groups
	s = strings.TrimPrefix(s, "(?m)")
	s = strings.TrimPrefix(s, "^")
	s = strings.TrimLeft(s, `\s*`)
	// Trim trailing regex parts like \s+ or similar
	if idx := strings.Index(s, `\s`); idx > 0 {
		s = s[:idx]
	}
	if len(s) > 50 {
		s = s[:50] + "..."
	}
	return s
}

// AutoFix applies the regex replacement for an auto-fixable opportunity.
func AutoFix(opp MigrationOpportunity, content string) (string, error) {
	if !opp.AutoFixable {
		return "", fmt.Errorf("opportunity is not auto-fixable: %s at %s:%d", opp.OldPattern, opp.File, opp.Line)
	}

	re, err := regexp.Compile(opp.OldPattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern %q: %w", opp.OldPattern, err)
	}

	lines := strings.Split(content, "\n")
	if opp.Line < 1 || opp.Line > len(lines) {
		return "", fmt.Errorf("line %d out of range (file has %d lines)", opp.Line, len(lines))
	}

	targetLine := opp.Line - 1
	lines[targetLine] = re.ReplaceAllString(lines[targetLine], opp.NewPattern)

	return strings.Join(lines, "\n"), nil
}

// AddRule adds a custom migration rule to the detector.
func (md *MigrationDetector) AddRule(rule MigrationRule) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.Rules = append(md.Rules, rule)
}
