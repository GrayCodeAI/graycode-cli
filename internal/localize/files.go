package localize

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// defaultIgnoreDirs are directories skipped during the file walk.
var defaultIgnoreDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".nuxt":        true,
	"target":       true,
	"bin":          true,
	"obj":          true,
	".idea":        true,
	".vscode":      true,
}

// supportedExts lists file extensions we can analyze.
var supportedExts = map[string]bool{
	".go":   true,
	".py":   true,
	".ts":   true,
	".tsx":  true,
	".js":   true,
	".jsx":  true,
	".rs":   true,
	".java": true,
}

// findFiles walks rootDir and returns files scored against the query keywords.
// It returns at most maxFiles results, sorted by descending score.
func findFiles(rootDir string, query string, maxFiles int) ([]FileMatch, error) {
	keywords := extractKeywords(query)
	if len(keywords) == 0 {
		return nil, nil
	}

	var results []FileMatch

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			if defaultIgnoreDirs[filepath.Base(path)] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if !supportedExts[ext] {
			return nil
		}

		relPath, relErr := filepath.Rel(rootDir, path)
		if relErr != nil {
			relPath = path
		}

		score, reason := scoreFile(relPath, keywords)
		if score > 0 {
			results = append(results, FileMatch{
				Path:   relPath,
				Score:  score,
				Reason: reason,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort by score descending, then by path for stability
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Path < results[j].Path
	})

	// Trim to maxFiles
	if len(results) > maxFiles {
		results = results[:maxFiles]
	}

	return results, nil
}

// scoreFile scores a file path against a set of keywords.
// Returns the aggregate score and a human-readable reason.
func scoreFile(relPath string, keywords []string) (float64, string) {
	lower := strings.ToLower(relPath)
	base := strings.ToLower(filepath.Base(relPath))
	nameNoExt := strings.TrimSuffix(base, filepath.Ext(base))

	// Split path into components for matching
	parts := strings.FieldsFunc(lower, func(r rune) bool {
		return r == '/' || r == '\\' || r == '_' || r == '-' || r == '.'
	})

	var score float64
	var reasons []string

	for _, kw := range keywords {
		kwLower := strings.ToLower(kw)

		// Exact filename match (highest signal)
		if nameNoExt == kwLower {
			score += 10.0
			reasons = append(reasons, "exact filename match: "+kw)
			continue
		}

		// Filename contains keyword
		if strings.Contains(nameNoExt, kwLower) {
			score += 5.0
			reasons = append(reasons, "filename contains: "+kw)
			continue
		}

		// Path component exact match
		componentMatch := false
		for _, part := range parts {
			if part == kwLower {
				score += 3.0
				reasons = append(reasons, "path component match: "+kw)
				componentMatch = true
				break
			}
		}
		if componentMatch {
			continue
		}

		// Substring of any path component
		substringMatch := false
		for _, part := range parts {
			if strings.Contains(part, kwLower) {
				score += 1.5
				reasons = append(reasons, "path substring: "+kw)
				substringMatch = true
				break
			}
		}
		if substringMatch {
			continue
		}

		// Full path contains keyword
		if strings.Contains(lower, kwLower) {
			score += 1.0
			reasons = append(reasons, "path contains: "+kw)
		}
	}

	reason := strings.Join(reasons, "; ")
	return score, reason
}

// extractKeywords splits a query into meaningful keywords for matching.
// It removes common stop words and short tokens.
func extractKeywords(query string) []string {
	stopWords := map[string]bool{
		"a": true, "an": true, "the": true, "is": true, "in": true,
		"on": true, "at": true, "to": true, "for": true, "of": true,
		"and": true, "or": true, "not": true, "it": true, "this": true,
		"that": true, "with": true, "from": true, "by": true, "as": true,
		"be": true, "are": true, "was": true, "were": true, "been": true,
		"has": true, "have": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "can": true, "but": true, "if": true,
		"when": true, "where": true, "how": true, "what": true, "which": true,
		"who": true, "whom": true, "why": true,
	}

	// Split on whitespace and punctuation
	tokens := strings.FieldsFunc(query, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ',' ||
			r == '.' || r == ';' || r == ':' || r == '!' || r == '?' ||
			r == '(' || r == ')' || r == '[' || r == ']' || r == '{' || r == '}' ||
			r == '"' || r == '\'' || r == '`'
	})

	var keywords []string
	seen := map[string]bool{}
	for _, tok := range tokens {
		lower := strings.ToLower(tok)
		if len(lower) < 2 {
			continue
		}
		if stopWords[lower] {
			continue
		}
		if seen[lower] {
			continue
		}
		seen[lower] = true
		keywords = append(keywords, lower)
	}
	return keywords
}
