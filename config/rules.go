package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Rule represents a project rule loaded from .hawk/rules/*.md.
type Rule struct {
	Name    string
	Content string
	Paths   []string // glob patterns; empty = always active
}

// LoadRules reads all .md files from .hawk/rules/ in the current directory.
// Each file can have optional YAML frontmatter with a paths field.
func LoadRules() []Rule {
	dir, _ := os.Getwd()
	return LoadRulesFrom(dir)
}

// LoadRulesFrom reads rules from .hawk/rules/ under the given directory.
func LoadRulesFrom(base string) []Rule {
	rulesDir := filepath.Join(base, ".hawk", "rules")
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		return nil
	}

	var rules []Rule
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(rulesDir, e.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		content := string(data)
		var paths []string

		// Parse YAML frontmatter if present
		if strings.HasPrefix(content, "---\n") {
			parts := strings.SplitN(content[4:], "\n---\n", 2)
			if len(parts) == 2 {
				paths = parseFrontmatterPaths(parts[0])
				content = strings.TrimSpace(parts[1])
			}
		}

		rules = append(rules, Rule{
			Name:    name,
			Content: content,
			Paths:   paths,
		})
	}
	return rules
}

// parseFrontmatterPaths extracts paths from simple YAML frontmatter.
// Supports: paths: ["glob1", "glob2"] or paths:\n- glob1\n- glob2
func parseFrontmatterPaths(frontmatter string) []string {
	var paths []string
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)

		// Inline array: paths: ["src/**", "lib/**"]
		if strings.HasPrefix(line, "paths:") {
			rest := strings.TrimPrefix(line, "paths:")
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
				inner := rest[1 : len(rest)-1]
				for _, item := range strings.Split(inner, ",") {
					item = strings.TrimSpace(item)
					item = strings.Trim(item, `"'`)
					if item != "" {
						paths = append(paths, item)
					}
				}
				return paths
			}
			continue
		}

		// List items: - glob
		if strings.HasPrefix(line, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			item = strings.Trim(item, `"'`)
			if item != "" {
				paths = append(paths, item)
			}
		}
	}
	return paths
}

// ActiveRules filters rules to those whose Paths match any of the touchedPaths.
// Rules with empty Paths are always active.
func ActiveRules(rules []Rule, touchedPaths []string) []Rule {
	var active []Rule
	for _, rule := range rules {
		if len(rule.Paths) == 0 {
			active = append(active, rule)
			continue
		}
		if ruleMatchesAny(rule.Paths, touchedPaths) {
			active = append(active, rule)
		}
	}
	return active
}

// ruleMatchesAny checks if any of the rule's glob patterns match any touched path.
func ruleMatchesAny(patterns, touchedPaths []string) bool {
	for _, pattern := range patterns {
		for _, tp := range touchedPaths {
			if globMatch(pattern, tp) {
				return true
			}
		}
	}
	return false
}

// globMatch performs glob matching with support for ** (match any path segments).
func globMatch(pattern, path string) bool {
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	// Handle ** patterns by trying all possible prefix matches
	if strings.Contains(pattern, "**") {
		parts := strings.SplitN(pattern, "**", 2)
		prefix := parts[0]
		suffix := parts[1]
		if strings.HasPrefix(suffix, "/") {
			suffix = suffix[1:]
		}

		// Check if path starts with prefix
		if prefix != "" && !strings.HasPrefix(path, prefix) {
			return false
		}

		// If no suffix, prefix match is enough
		if suffix == "" {
			return true
		}

		// Try matching suffix against remaining path segments
		remaining := strings.TrimPrefix(path, prefix)
		segments := strings.Split(remaining, "/")
		for i := range segments {
			candidate := strings.Join(segments[i:], "/")
			matched, _ := filepath.Match(suffix, candidate)
			if matched {
				return true
			}
			// Also try matching just the base name
			matched, _ = filepath.Match(suffix, filepath.Base(candidate))
			if matched {
				return true
			}
		}
		return false
	}

	matched, _ := filepath.Match(pattern, path)
	return matched
}

// FormatActiveRules formats active rules for injection into the system prompt.
func FormatActiveRules(rules []Rule) string {
	if len(rules) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Project Rules\n\n")
	for _, rule := range rules {
		b.WriteString("### " + rule.Name + "\n")
		b.WriteString(rule.Content + "\n\n")
	}
	return b.String()
}
