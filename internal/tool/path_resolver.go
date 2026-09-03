package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// PathResolver helps the agent find the correct file paths even when given
// partial or ambiguous paths.
type PathResolver struct {
	ProjectDir string
	FileCache  map[string]bool
	LastScan   time.Time
	mu         sync.RWMutex
}

// ResolveResult contains the outcome of a path resolution attempt.
type ResolveResult struct {
	Found        bool
	Path         string
	Alternatives []string
	Confidence   float64
}

// NewPathResolver creates a new PathResolver for the given project directory.
func NewPathResolver(projectDir string) *PathResolver {
	return &PathResolver{
		ProjectDir: projectDir,
		FileCache:  make(map[string]bool),
	}
}

// Resolve attempts to find the correct file path given an exact, partial, or
// ambiguous input path. It tries multiple strategies in order of confidence.
func (pr *PathResolver) Resolve(path string) *ResolveResult {
	pr.mu.RLock()
	cacheEmpty := len(pr.FileCache) == 0
	pr.mu.RUnlock()

	if cacheEmpty {
		_ = pr.ScanProject()
	}

	normalized := pr.NormalizePath(path)

	// Strategy 1: Exact path exists on filesystem
	if pr.IsValidPath(normalized) {
		return &ResolveResult{
			Found:      true,
			Path:       normalized,
			Confidence: 1.0,
		}
	}

	// Strategy 2: Relative to project root
	relPath := filepath.Join(pr.ProjectDir, normalized)
	if pr.IsValidPath(relPath) {
		return &ResolveResult{
			Found:      true,
			Path:       relPath,
			Confidence: 0.98,
		}
	}

	// Strategy 3: Basename match (if unique)
	baseName := filepath.Base(normalized)
	matches := pr.FindByName(baseName)
	if len(matches) == 1 {
		return &ResolveResult{
			Found:      true,
			Path:       matches[0],
			Confidence: 0.85,
		}
	}
	if len(matches) > 1 {
		// Try to narrow down by partial directory matching
		best := pr.bestDirMatch(normalized, matches)
		if best != "" {
			alts := make([]string, 0, len(matches)-1)
			for _, m := range matches {
				if m != best {
					alts = append(alts, m)
				}
			}
			return &ResolveResult{
				Found:        true,
				Path:         best,
				Alternatives: alts,
				Confidence:   0.80,
			}
		}
		return &ResolveResult{
			Found:        false,
			Path:         matches[0],
			Alternatives: matches[1:],
			Confidence:   0.50,
		}
	}

	// Strategy 4: Fuzzy match via Levenshtein distance
	similar := pr.FindSimilar(normalized, 5)
	if len(similar) > 0 {
		// Calculate confidence based on distance
		best := similar[0]
		dist := levenshtein(normalized, pr.relativeTo(best))
		maxLen := len(normalized)
		if len(pr.relativeTo(best)) > maxLen {
			maxLen = len(pr.relativeTo(best))
		}
		confidence := 1.0 - float64(dist)/float64(maxLen)
		if confidence < 0 {
			confidence = 0
		}

		var alts []string
		if len(similar) > 1 {
			alts = similar[1:]
		}

		return &ResolveResult{
			Found:        confidence >= 0.7,
			Path:         best,
			Alternatives: alts,
			Confidence:   confidence,
		}
	}

	return &ResolveResult{
		Found:      false,
		Confidence: 0,
	}
}

// FindByName finds all files with this exact name anywhere in the project.
func (pr *PathResolver) FindByName(filename string) []string {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	var results []string
	for path := range pr.FileCache {
		if filepath.Base(path) == filename {
			results = append(results, path)
		}
	}
	sort.Strings(results)
	return results
}

// FindSimilar returns paths similar to the input based on Levenshtein distance.
func (pr *PathResolver) FindSimilar(path string, limit int) []string {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	normalized := pr.NormalizePath(path)
	// Use the relative form for comparison
	relInput := normalized
	if filepath.IsAbs(normalized) {
		rel, err := filepath.Rel(pr.ProjectDir, normalized)
		if err == nil {
			relInput = rel
		}
	}

	type scored struct {
		path string
		dist int
	}

	var candidates []scored
	for cachedPath := range pr.FileCache {
		relCached := pr.relativeTo(cachedPath)
		dist := levenshtein(relInput, relCached)
		// Only include results within a reasonable distance
		maxLen := len(relInput)
		if len(relCached) > maxLen {
			maxLen = len(relCached)
		}
		if maxLen > 0 && float64(dist)/float64(maxLen) < 0.5 {
			candidates = append(candidates, scored{path: cachedPath, dist: dist})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].dist < candidates[j].dist
	})

	if limit > len(candidates) {
		limit = len(candidates)
	}

	results := make([]string, limit)
	for i := 0; i < limit; i++ {
		results[i] = candidates[i].path
	}
	return results
}

// FindByPattern finds files matching a glob pattern across the project.
func (pr *PathResolver) FindByPattern(glob string) []string {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	var results []string
	for path := range pr.FileCache {
		relPath := pr.relativeTo(path)
		matched, err := filepath.Match(glob, relPath)
		if err == nil && matched {
			results = append(results, path)
			continue
		}
		// Also try matching just the basename
		matched, err = filepath.Match(glob, filepath.Base(path))
		if err == nil && matched {
			results = append(results, path)
		}
	}
	sort.Strings(results)
	return results
}

// ScanProject builds the file cache for fast lookups.
func (pr *PathResolver) ScanProject() error {
	cache := make(map[string]bool)

	err := filepath.WalkDir(pr.ProjectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "dist" || name == ".graycode" {
				return filepath.SkipDir
			}
			return nil
		}
		cache[path] = true
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan project: %w", err)
	}

	pr.mu.Lock()
	pr.FileCache = cache
	pr.LastScan = time.Now()
	pr.mu.Unlock()

	return nil
}

// SuggestCorrection returns the best single correction suggestion for a wrong path.
func (pr *PathResolver) SuggestCorrection(wrongPath string) string {
	result := pr.Resolve(wrongPath)
	if result.Found {
		return result.Path
	}
	if result.Path != "" {
		return result.Path
	}
	return ""
}

// FormatResult formats a ResolveResult for display.
func (pr *PathResolver) FormatResult(result *ResolveResult) string {
	if result == nil {
		return "Path resolution: no result"
	}

	var sb strings.Builder

	if result.Path != "" {
		relPath := pr.relativeTo(result.Path)
		if result.Found {
			sb.WriteString(fmt.Sprintf("Path resolution:\n  -> Resolved to: %s (confidence: %.2f)\n", relPath, result.Confidence))
		} else {
			sb.WriteString(fmt.Sprintf("Path resolution:\n  -> Best guess: %s (confidence: %.2f)\n", relPath, result.Confidence))
		}
	} else {
		sb.WriteString("Path resolution: not found\n")
	}

	if len(result.Alternatives) > 0 {
		sb.WriteString("  Alternatives:\n")
		for _, alt := range result.Alternatives {
			relAlt := pr.relativeTo(alt)
			sb.WriteString(fmt.Sprintf("    %s\n", relAlt))
		}
	}

	return sb.String()
}

// IsValidPath checks whether the given path points to an existing file.
func (pr *PathResolver) IsValidPath(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// NormalizePath cleans a path, resolves .., and handles ~ expansion.
func (pr *PathResolver) NormalizePath(path string) string {
	if path == "" {
		return ""
	}

	// Handle ~ expansion
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = home
		}
	}

	// Clean the path (resolve .., remove double slashes, etc.)
	path = filepath.Clean(path)

	return path
}

// bestDirMatch selects the candidate whose directory components best match the input.
func (pr *PathResolver) bestDirMatch(input string, candidates []string) string {
	inputDir := filepath.Dir(input)
	if inputDir == "." {
		return ""
	}

	inputParts := strings.Split(inputDir, string(filepath.Separator))

	bestScore := 0
	bestPath := ""

	for _, candidate := range candidates {
		relCandidate := pr.relativeTo(candidate)
		candDir := filepath.Dir(relCandidate)
		candParts := strings.Split(candDir, string(filepath.Separator))

		score := 0
		for _, ip := range inputParts {
			for _, cp := range candParts {
				if ip == cp {
					score++
					break
				}
			}
		}

		if score > bestScore {
			bestScore = score
			bestPath = candidate
		}
	}

	if bestScore > 0 {
		return bestPath
	}
	return ""
}

// relativeTo returns the path relative to ProjectDir, or the path itself if
// it cannot be made relative.
func (pr *PathResolver) relativeTo(path string) string {
	rel, err := filepath.Rel(pr.ProjectDir, path)
	if err != nil {
		return path
	}
	return rel
}

// levenshtein computes the Levenshtein distance between two strings.
func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Use two rows to save memory
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)

	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(
				curr[j-1]+1,    // insertion
				prev[j]+1,      // deletion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}

	return prev[len(b)]
}

// min3 returns the minimum of three integers.
func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
