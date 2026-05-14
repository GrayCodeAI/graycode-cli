// Package mention implements @-prefixed file mentions in prompt input,
// enabling users to reference project files that get auto-included as context.
package mention

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// mentionRe matches @-prefixed file paths in input text.
// Supports @path/to/file.go and @"path with spaces/file.go" forms.
var mentionRe = regexp.MustCompile(`@"([^"]+)"|@([^\s,;:!?]+)`)

// ParseResult holds the output of parsing mentions from input.
type ParseResult struct {
	CleanInput    string   // Input with mentions removed
	MentionedFiles []string // Resolved file paths
	RawMentions   []string // Original mention strings as typed
}

// ParseMentions extracts @-prefixed file mentions from the input string.
// Returns the cleaned input (without mention tokens) and the list of mentioned file paths.
func ParseMentions(input string, projectRoot string) ParseResult {
	matches := mentionRe.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return ParseResult{CleanInput: input}
	}

	var result ParseResult
	var cleanParts []string
	lastEnd := 0

	for _, match := range matches {
		// Add text before this match.
		cleanParts = append(cleanParts, input[lastEnd:match[0]])

		// Extract the file path (group 1 for quoted, group 2 for unquoted).
		var path string
		if match[2] >= 0 && match[3] >= 0 {
			path = input[match[2]:match[3]]
		} else if match[4] >= 0 && match[5] >= 0 {
			path = input[match[4]:match[5]]
		}

		if path != "" {
			result.RawMentions = append(result.RawMentions, path)
			resolved := resolvePath(path, projectRoot)
			if resolved != "" {
				result.MentionedFiles = append(result.MentionedFiles, resolved)
			}
		}

		lastEnd = match[1]
	}
	cleanParts = append(cleanParts, input[lastEnd:])
	result.CleanInput = strings.TrimSpace(strings.Join(cleanParts, ""))

	return result
}

// FuzzyMatch performs fuzzy matching of a partial path against the project file tree.
// Returns up to maxResults matching paths, sorted by relevance.
func FuzzyMatch(partial string, projectRoot string, maxResults int) []string {
	if partial == "" || projectRoot == "" {
		return nil
	}

	partial = strings.TrimPrefix(partial, "@")
	partial = strings.ToLower(partial)

	var matches []scoredMatch
	_ = filepath.WalkDir(projectRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Skip hidden directories and common noise.
		name := d.Name()
		if strings.HasPrefix(name, ".") && d.IsDir() {
			return filepath.SkipDir
		}
		if d.IsDir() && isIgnoredDir(name) {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return nil
		}

		score := fuzzyScore(partial, strings.ToLower(rel))
		if score > 0 {
			matches = append(matches, scoredMatch{path: rel, score: score})
		}
		return nil
	})

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}

	results := make([]string, len(matches))
	for i, m := range matches {
		results[i] = m.path
	}
	return results
}

// IsMention returns true if the token starts with @.
func IsMention(token string) bool {
	return strings.HasPrefix(token, "@") && len(token) > 1
}

// ExtractPartial extracts the partial path being typed after @ for completion.
func ExtractPartial(input string, cursorPos int) string {
	if cursorPos > len(input) {
		cursorPos = len(input)
	}

	// Walk backwards from cursor to find the @.
	start := cursorPos - 1
	for start >= 0 {
		if input[start] == '@' {
			return input[start+1 : cursorPos]
		}
		if input[start] == ' ' || input[start] == '\n' {
			break
		}
		start--
	}
	return ""
}

type scoredMatch struct {
	path  string
	score int
}

// fuzzyScore computes a relevance score for matching partial against candidate.
func fuzzyScore(partial, candidate string) int {
	if partial == "" {
		return 0
	}

	// Exact prefix match gets highest score.
	if strings.HasPrefix(candidate, partial) {
		return 100 + len(partial)
	}

	// Check if the filename starts with partial.
	base := filepath.Base(candidate)
	if strings.HasPrefix(base, partial) {
		return 80 + len(partial)
	}

	// Path contains partial as substring.
	if strings.Contains(candidate, partial) {
		return 60
	}

	// Fuzzy character matching.
	pi := 0
	matchCount := 0
	for ci := 0; ci < len(candidate) && pi < len(partial); ci++ {
		if candidate[ci] == partial[pi] {
			pi++
			matchCount++
		}
	}
	if pi == len(partial) {
		return 30 + matchCount
	}

	return 0
}

// resolvePath resolves a mentioned path relative to the project root.
func resolvePath(path string, projectRoot string) string {
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		return ""
	}

	// Try relative to project root.
	full := filepath.Join(projectRoot, path)
	if _, err := os.Stat(full); err == nil {
		return full
	}

	// Try glob match.
	matches, err := filepath.Glob(filepath.Join(projectRoot, "**", path))
	if err == nil && len(matches) > 0 {
		return matches[0]
	}

	// Return the path as-is if it looks reasonable (user might add it later).
	return filepath.Join(projectRoot, path)
}

// isIgnoredDir returns true for directories that should be skipped during file matching.
func isIgnoredDir(name string) bool {
	ignored := map[string]bool{
		"node_modules": true,
		"vendor":       true,
		".git":         true,
		"__pycache__":  true,
		".cache":       true,
		"dist":         true,
		"build":        true,
		"target":       true,
	}
	return ignored[name]
}
