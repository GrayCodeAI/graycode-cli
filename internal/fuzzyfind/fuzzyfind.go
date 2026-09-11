// Package fuzzyfind implements a standalone fuzzy file finder with graceful
// degradation, adopted from grok-build's xai-fuzzy-file-search: results are
// scored by substring match quality (exact basename > basename prefix >
// basename contains > all-segments > path contains > camel-case abbreviation)
// with a path-length tiebreak (shorter = more specific match).
package fuzzyfind

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Match is one scored file path.
type Match struct {
	Path  string `json:"path"`
	Score int    `json:"score"`
}

// Finder searches a directory tree for files matching a query.
type Finder struct {
	root     string
	maxFiles int
}

// New creates a Finder rooted at dir. Unlike the grok-build original (which
// probes goroutine spawnability for three-tier degradation), Go goroutines
// are always available so the mode is always full; kept simple by design.
func New(dir string) (*Finder, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("fuzzyfind: stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("fuzzyfind: not a directory: %s", dir)
	}
	return &Finder{root: dir, maxFiles: 50_000}, nil
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	".hawk": true, "__pycache__": true, "dist": true,
	"target": true, ".next": true, "build": true,
}

// Search walks the tree and returns top-k paths matching query, sorted by
// descending score then ascending path length.
func (f *Finder) Search(query string, k int) []Match {
	if k <= 0 {
		k = 20
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil // empty query returns nothing (browse mode is caller-side)
	}
	var paths []string
	_ = filepath.WalkDir(f.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || skipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(f.root, path)
		if relErr != nil {
			return nil
		}
		if len(paths) >= f.maxFiles {
			return filepath.SkipAll
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})

	var out []Match
	for _, p := range paths {
		sc := score(p, query)
		if sc > 0 {
			out = append(out, Match{Path: p, Score: sc})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return len(out[i].Path) < len(out[j].Path)
	})
	if len(out) > k {
		out = out[:k]
	}
	return out
}

// score computes relevance for a lowered path against a lowered query.
func score(path, query string) int {
	low := strings.ToLower(path)
	base := low[strings.LastIndexByte(low, '/')+1:]
	switch {
	case base == query:
		return 1000
	case strings.HasPrefix(base, query):
		return 800
	case strings.Contains(base, query):
		return 600
	case containsAllSegments(low, query):
		return 400
	case strings.Contains(low, query):
		return 200
	case matchAbbrev(path, query):
		return 100
	default:
		return 0
	}
}

// containsAllSegments requires every space-separated query term somewhere in
// the path; only meaningful for multi-word queries.
func containsAllSegments(low, query string) bool {
	fields := strings.Fields(query)
	if len(fields) < 2 {
		return false
	}
	for _, t := range fields {
		if !strings.Contains(low, t) {
			return false
		}
	}
	return true
}

// matchAbbrev scores camelCase / snake_case abbreviations where each rune of
// the query maps to the start of a successive word in the basename.
func matchAbbrev(path, query string) bool {
	base := path[strings.LastIndexByte(path, '/')+1:]
	var words []string
	start := 0
	for i := 0; i < len(base); i++ {
		c := base[i]
		if i > 0 && (c == '_' || c == '-' || c == '.' ||
			(c >= 'A' && c <= 'Z' && base[i-1] >= 'a' && base[i-1] <= 'z')) {
			words = append(words, strings.ToLower(base[start:i]))
			start = i
		}
	}
	words = append(words, strings.ToLower(base[start:]))

	q := strings.ToLower(query)
	qi := 0
	for _, w := range words {
		for _, r := range w {
			if qi < len(q) && rune(q[qi]) == r {
				qi++
			}
		}
	}
	return qi == len(q)
}
