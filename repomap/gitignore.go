package repomap

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// GitignoreRules holds composed gitignore patterns from multiple levels.
type GitignoreRules struct {
	rules []ignoreRule
}

type ignoreRule struct {
	pattern string
	negate  bool   // starts with !
	dirOnly bool   // ends with /
	baseDir string // directory where this rule was defined
}

// LoadGitignoreRules walks from dir up to the filesystem root, loading all
// .gitignore files. Rules from deeper directories take precedence (are appended
// last). Also loads the global gitignore (~/.config/git/ignore) if it exists.
func LoadGitignoreRules(dir string) *GitignoreRules {
	gr := &GitignoreRules{}

	// Load global gitignore first (lowest priority)
	home, err := os.UserHomeDir()
	if err == nil {
		globalPath := filepath.Join(home, ".config", "git", "ignore")
		gr.rules = append(gr.rules, parseGitignore(globalPath, "")...)
	}

	// Collect ancestor directories from root to dir (so deeper overrides later)
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	var ancestors []string
	current := absDir
	for {
		ancestors = append([]string{current}, ancestors...)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// Load .gitignore from root down to the target directory
	for _, ancestor := range ancestors {
		gitignorePath := filepath.Join(ancestor, ".gitignore")
		gr.rules = append(gr.rules, parseGitignore(gitignorePath, ancestor)...)
	}

	return gr
}

// ShouldIgnore checks if a path should be ignored according to gitignore rules.
// The path should be relative to the repository root.
func (gr *GitignoreRules) ShouldIgnore(path string) bool {
	if gr == nil || len(gr.rules) == 0 {
		return false
	}

	ignored := false
	for _, rule := range gr.rules {
		if matchRule(rule, path) {
			ignored = !rule.negate
		}
	}
	return ignored
}

// parseGitignore reads a .gitignore file and returns rules.
func parseGitignore(path, baseDir string) []ignoreRule {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var rules []ignoreRule
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Strip trailing whitespace (but not leading, as that's significant in some cases)
		line = strings.TrimRight(line, " \t")

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		rule := ignoreRule{baseDir: baseDir}

		// Handle negation
		if strings.HasPrefix(line, "!") {
			rule.negate = true
			line = line[1:]
		}

		// Handle directory-only patterns (trailing /)
		if strings.HasSuffix(line, "/") {
			rule.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}

		rule.pattern = line
		rules = append(rules, rule)
	}

	return rules
}

// matchRule checks if a path matches a single gitignore rule.
func matchRule(rule ignoreRule, path string) bool {
	pattern := rule.pattern

	// If the pattern contains a slash (not just trailing), it's anchored to baseDir
	if strings.Contains(pattern, "/") {
		// Make path relative to the rule's baseDir
		if rule.baseDir != "" {
			relPath, err := filepath.Rel(rule.baseDir, path)
			if err != nil {
				return false
			}
			path = relPath
		}
		matched, _ := filepath.Match(pattern, path)
		return matched
	}

	// For non-anchored patterns, match against the basename
	base := filepath.Base(path)
	if matched, _ := filepath.Match(pattern, base); matched {
		return true
	}

	// Also try matching against the full relative path
	if matched, _ := filepath.Match(pattern, path); matched {
		return true
	}

	return false
}
