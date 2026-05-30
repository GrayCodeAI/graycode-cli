package context

import (
	"path/filepath"
	"strings"
)

// RuleFrontmatter holds the YAML frontmatter parsed from a rule file.
type RuleFrontmatter struct {
	Description string   `yaml:"description"`
	Globs       []string `yaml:"globs"`
	AlwaysApply *bool    `yaml:"alwaysApply"`
}

// ParseFrontmatter extracts YAML frontmatter from a markdown file.
// Returns the parsed frontmatter, the markdown body, and any error.
// If no frontmatter is present, returns nil frontmatter and the full content as body.
func ParseFrontmatter(content string) (*RuleFrontmatter, string) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return nil, content
	}

	// Find the closing ---
	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return nil, content
	}

	yamlBlock := strings.TrimSpace(rest[:endIdx])
	body := strings.TrimSpace(rest[endIdx+4:])

	fm := parseSimpleYAML(yamlBlock)
	return fm, body
}

// parseSimpleYAML does a minimal YAML parse of the frontmatter block.
// It handles the specific fields we care about (description, globs, alwaysApply)
// without requiring a full YAML library.
func parseSimpleYAML(block string) *RuleFrontmatter {
	fm := &RuleFrontmatter{}
	lines := strings.Split(block, "\n")

	inGlobs := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "description:") {
			fm.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
			fm.Description = strings.Trim(fm.Description, "\"'")
			inGlobs = false
			continue
		}

		if strings.HasPrefix(trimmed, "alwaysApply:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "alwaysApply:"))
			b := val == "true"
			fm.AlwaysApply = &b
			inGlobs = false
			continue
		}

		if strings.HasPrefix(trimmed, "globs:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "globs:"))
			if strings.HasPrefix(val, "[") {
				// Inline array: globs: ["*.ts", "*.tsx"]
				val = strings.Trim(val, "[]")
				val = strings.ReplaceAll(val, "\"", "")
				val = strings.ReplaceAll(val, "'", "")
				for _, g := range strings.Split(val, ",") {
					g = strings.TrimSpace(g)
					if g != "" {
						fm.Globs = append(fm.Globs, g)
					}
				}
				inGlobs = false
			} else {
				// Multi-line list follows
				inGlobs = true
			}
			continue
		}

		if inGlobs && strings.HasPrefix(trimmed, "- ") {
			g := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			g = strings.Trim(g, "\"'")
			if g != "" {
				fm.Globs = append(fm.Globs, g)
			}
			continue
		}

		// Any other key resets globs context
		if !strings.HasPrefix(trimmed, "-") && trimmed != "" {
			inGlobs = false
		}
	}

	return fm
}

// ShouldInject determines whether a rule file should be injected for a given
// file path, based on its frontmatter configuration.
// Rules are injected when:
// - alwaysApply is true (or nil)
// - alwaysApply is false AND the file matches one of the globs
// If no frontmatter is present, the rule is always applied.
func ShouldInject(fm *RuleFrontmatter, filePath string) bool {
	if fm == nil {
		return true
	}
	if fm.AlwaysApply == nil || *fm.AlwaysApply {
		return true
	}
	if len(fm.Globs) == 0 {
		return true
	}
	return MatchGlobs(fm.Globs, filePath)
}

// MatchGlobs checks if a file path matches any of the given glob patterns.
// Supports standard filepath.Match patterns plus ** for directory matching.
func MatchGlobs(globs []string, filePath string) bool {
	for _, glob := range globs {
		if matchGlob(glob, filePath) {
			return true
		}
	}
	return false
}

func matchGlob(glob, filePath string) bool {
	// Normalize separators
	glob = filepath.ToSlash(glob)
	filePath = filepath.ToSlash(filePath)

	// Simple ** support: **/ matches any path prefix
	if strings.Contains(glob, "**") {
		return matchDoubleStar(glob, filePath)
	}

	// Use filepath.Match for standard patterns
	matched, _ := filepath.Match(glob, filePath)
	if matched {
		return true
	}
	// Also try matching just the basename
	matched, _ = filepath.Match(glob, filepath.Base(filePath))
	return matched
}

func matchDoubleStar(glob, filePath string) bool {
	parts := strings.Split(glob, "**")
	if len(parts) == 1 {
		matched, _ := filepath.Match(glob, filePath)
		return matched
	}

	// For patterns like **/*.ts, match suffix
	if len(parts) == 2 && parts[0] == "" {
		suffix := strings.TrimPrefix(parts[1], "/")
		// Match the suffix against the file path
		return matchSuffix(suffix, filePath)
	}

	// For patterns like src/**/*.ts
	prefix := parts[0]
	suffix := strings.TrimPrefix(parts[1], "/")

	if !strings.HasPrefix(filePath, strings.TrimSuffix(prefix, "/")) {
		return false
	}
	remaining := strings.TrimPrefix(filePath, strings.TrimSuffix(prefix, "/"))
	remaining = strings.TrimPrefix(remaining, "/")
	return matchSuffix(suffix, remaining)
}

func matchSuffix(pattern, path string) bool {
	if !strings.Contains(pattern, "/") {
		// Simple suffix like *.ts - match against any path component
		return matchGlob(pattern, filepath.Base(path))
	}
	// Path pattern - try matching against each suffix of the path
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i := range parts {
		candidate := strings.Join(parts[i:], "/")
		matched, _ := filepath.Match(pattern, candidate)
		if matched {
			return true
		}
	}
	return false
}
