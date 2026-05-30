package context

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RuleSource defines a source of rule files with precedence.
type RuleSource struct {
	Name     string // directory or file name (e.g., ".hawk/rules", "AGENTS.md")
	Priority int    // lower = higher priority
	IsDir    bool   // true = scan directory recursively for *.md files
}

// DefaultRuleSources lists rule sources in priority order.
var DefaultRuleSources = []RuleSource{
	{".hawk/rules", 1, true},
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
	Path      string
	Content   string
	Hash      string
	Source    string // e.g., ".hawk/rules", "AGENTS.md"
	Local     bool   // true if found in the file's directory tree, false if global
	Distance  int    // 0 = same directory, 1 = parent, etc.
	Priority  int    // source priority
}

// RuleDiscoverer discovers rule files using walk-up stack semantics
// with 4-level precedence: local vs global → distance → source priority → path.
type RuleDiscoverer struct {
	projectRoot string
	sources     []RuleSource
	globalDirs  []string // user-home rule directories
	cache       *InjectionCache
}

// NewRuleDiscoverer creates a rule discoverer for the given project.
func NewRuleDiscoverer(projectRoot string) *RuleDiscoverer {
	home, _ := os.UserHomeDir()
	var globalDirs []string
	if home != "" {
		globalDirs = []string{
			filepath.Join(home, ".hawk", "rules"),
			filepath.Join(home, ".omo", "rules"),
			filepath.Join(home, ".claude", "rules"),
		}
	}
	return &RuleDiscoverer{
		projectRoot: projectRoot,
		sources:     DefaultRuleSources,
		globalDirs:  globalDirs,
		cache:       NewInjectionCache(),
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
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			rules = append(rules, Rule{
				Path:     path,
				Content:  string(data),
				Hash:     fmt.Sprintf("%x", sha256.Sum256(data)),
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
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		rules = append(rules, Rule{
			Path:     path,
			Content:  string(data),
			Hash:     fmt.Sprintf("%x", sha256.Sum256(data)),
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return &Rule{
		Path:     path,
		Content:  string(data),
		Hash:     fmt.Sprintf("%x", sha256.Sum256(data)),
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
