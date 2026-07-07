package context

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	homepkg "github.com/GrayCodeAI/hawk/internal/home"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

// managedSource is the synthetic source name for IT-managed (org policy) rules.
const managedSource = "managed"

// defaultManagedPaths returns the platform-specific IT-managed policy file
// locations. These are intended to be writable only by administrators and take
// precedence over all user/project rules, so they cannot be overridden.
func defaultManagedPaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/Library/Application Support/HawkCode/HAWK.md"}
	default:
		// Linux and other unix-like systems.
		return []string{"/etc/hawk-code/HAWK.md"}
	}
}

// htmlCommentRe matches HTML block comments (<!-- ... -->), including multi-line
// spans. It is non-greedy so adjacent comments are stripped independently.
var htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)

// stripHTMLComments removes all HTML block comments (<!-- ... -->) from the
// given content before it is injected as rule/instruction context.
func stripHTMLComments(s string) string {
	return htmlCommentRe.ReplaceAllString(s, "")
}

// RuleSource defines a source of rule files with precedence.
type RuleSource struct {
	Name     string // directory or file name (e.g., ".agents/rules", "AGENTS.md")
	Priority int    // lower = higher priority
	IsDir    bool   // true = scan directory recursively for *.md files
}

// DefaultRuleSources lists rule sources in priority order.
var DefaultRuleSources = []RuleSource{
	{".agents/rules", 1, true},
	{".omo/rules", 2, true},
	{".claude/rules", 3, true},
	{".cursor/rules", 4, true},
	{".github/instructions", 5, true},
	{"AGENTS.md", 10, false},
	{"HAWK.md", 11, false},
	{"CLAUDE.md", 12, false},
	{"CONTEXT.md", 13, false},
	{".github/copilot-instructions.md", 14, false},
}

// Rule represents a discovered rule file with precedence metadata.
type Rule struct {
	Path     string
	Content  string
	Hash     string
	Source   string // e.g., ".agents/rules", "AGENTS.md"
	Local    bool   // true if found in the file's directory tree, false if global
	Distance int    // 0 = same directory, 1 = parent, etc.
	Priority int    // source priority
}

// RuleDiscoverer discovers rule files using walk-up stack semantics
// with 4-level precedence: local vs global → distance → source priority → path.
type RuleDiscoverer struct {
	projectRoot  string
	sources      []RuleSource
	globalDirs   []string // user-home rule directories
	managedPaths []string // IT-managed (org policy) rule files; highest, non-excludable precedence
	cache        *InjectionCache
}

// NewRuleDiscoverer creates a rule discoverer for the given project.
func NewRuleDiscoverer(projectRoot string) *RuleDiscoverer {
	home := homepkg.Dir()
	var globalDirs []string
	if home != "" {
		globalDirs = []string{
			filepath.Join(storage.StateDir(), "rules"),
			filepath.Join(home, ".omo", "rules"),
			filepath.Join(home, ".claude", "rules"),
		}
	}
	return &RuleDiscoverer{
		projectRoot:  projectRoot,
		sources:      DefaultRuleSources,
		globalDirs:   globalDirs,
		managedPaths: defaultManagedPaths(),
		cache:        NewInjectionCache(),
	}
}

// Discover finds all applicable rules for a file path, ordered by precedence.
func (rd *RuleDiscoverer) Discover(filePath string) []Rule {
	dir := filepath.Dir(filePath)
	if !filepath.IsAbs(dir) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil
		}
		dir = abs
	}

	var rules []Rule

	// Collect IT-managed (org policy) rules first — highest, non-excludable precedence.
	managedRules := rd.collectManaged()
	rules = append(rules, managedRules...)

	// Collect local rules (walk-up from file directory to project root)
	localRules := rd.collectLocal(dir)
	rules = append(rules, localRules...)

	// Collect global rules (user home)
	globalRules := rd.collectGlobal()
	rules = append(rules, globalRules...)

	// Deduplicate by content hash
	rules = rd.dedup(rules)

	// Sort by precedence: local > global → distance > priority > path
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Local != rules[j].Local {
			return rules[i].Local // local first
		}
		if rules[i].Distance != rules[j].Distance {
			return rules[i].Distance < rules[j].Distance // closer first
		}
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority // lower priority number = higher precedence
		}
		return rules[i].Path < rules[j].Path
	})

	return rules
}

func (rd *RuleDiscoverer) collectLocal(dir string) []Rule {
	var rules []Rule
	currentDir := dir

	for {
		for _, src := range rd.sources {
			if src.IsDir {
				rules = append(rules, rd.scanDir(currentDir, src)...)
			} else {
				rule := rd.checkFile(currentDir, src)
				if rule != nil {
					rules = append(rules, *rule)
				}
			}
		}

		if currentDir == rd.projectRoot || currentDir == filepath.Dir(currentDir) {
			break
		}
		currentDir = filepath.Dir(currentDir)
	}
	return rules
}

// collectManaged loads IT-managed (org policy) rule files. These are given the
// highest precedence (Local=true, Distance/Priority below any real rule) so they
// sort ahead of all user/project/global rules and cannot be overridden.
func (rd *RuleDiscoverer) collectManaged() []Rule {
	var rules []Rule
	for _, path := range rd.managedPaths {
		data, err := os.ReadFile(path) // #nosec G304 -- path comes from configured managed (org policy) rule locations, not external input
		if err != nil {
			continue
		}
		content := stripHTMLComments(string(data))
		rules = append(rules, Rule{
			Path:     path,
			Content:  content,
			Hash:     fmt.Sprintf("%x", sha256.Sum256([]byte(content))),
			Source:   managedSource,
			Local:    true,
			Distance: -1, // sorts before distance 0 (closest local rule)
			Priority: -1, // sorts before priority 1 (highest source priority)
		})
	}
	return rules
}

func (rd *RuleDiscoverer) collectGlobal() []Rule {
	var rules []Rule
	for _, globalDir := range rd.globalDirs {
		entries, err := os.ReadDir(globalDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			path := filepath.Join(globalDir, entry.Name())
			data, err := os.ReadFile(path) // #nosec G304 -- path is built from a trusted global rules directory listing (os.ReadDir entries), not external input
			if err != nil {
				continue
			}
			content := stripHTMLComments(string(data))
			rules = append(rules, Rule{
				Path:     path,
				Content:  content,
				Hash:     fmt.Sprintf("%x", sha256.Sum256([]byte(content))),
				Source:   filepath.Base(globalDir),
				Local:    false,
				Distance: 999,
				Priority: 100,
			})
		}
	}
	return rules
}

func (rd *RuleDiscoverer) scanDir(baseDir string, src RuleSource) []Rule {
	rulesDir := filepath.Join(baseDir, src.Name)
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		return nil
	}
	var rules []Rule
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") && !strings.HasSuffix(entry.Name(), ".mdc") {
			continue
		}
		path := filepath.Join(rulesDir, entry.Name())
		data, err := os.ReadFile(path) // #nosec G304 -- path is built from a trusted rules directory listing (os.ReadDir entries), not external input
		if err != nil {
			continue
		}
		content := stripHTMLComments(string(data))
		rules = append(rules, Rule{
			Path:     path,
			Content:  content,
			Hash:     fmt.Sprintf("%x", sha256.Sum256([]byte(content))),
			Source:   src.Name,
			Local:    true,
			Distance: dirLevel(rd.projectRoot, baseDir),
			Priority: src.Priority,
		})
	}
	return rules
}

func (rd *RuleDiscoverer) checkFile(baseDir string, src RuleSource) *Rule {
	path := filepath.Join(baseDir, src.Name)
	data, err := os.ReadFile(path) // #nosec G304 -- path is built from fixed configured rule source names, not external input
	if err != nil {
		return nil
	}
	content := stripHTMLComments(string(data))
	return &Rule{
		Path:     path,
		Content:  content,
		Hash:     fmt.Sprintf("%x", sha256.Sum256([]byte(content))),
		Source:   src.Name,
		Local:    true,
		Distance: dirLevel(rd.projectRoot, baseDir),
		Priority: src.Priority,
	}
}

func (rd *RuleDiscoverer) dedup(rules []Rule) []Rule {
	seen := make(map[string]bool)
	var result []Rule
	for _, r := range rules {
		if seen[r.Hash] {
			continue
		}
		seen[r.Hash] = true
		result = append(result, r)
	}
	return result
}

// Cache returns the injection cache.
func (rd *RuleDiscoverer) Cache() *InjectionCache {
	return rd.cache
}
