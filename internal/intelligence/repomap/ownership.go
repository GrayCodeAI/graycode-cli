package repomap

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// OwnershipMap tracks code ownership across a repository by combining
// git history analysis with CODEOWNERS-style rules. It identifies primary
// owners, contributors, and bus-factor risks for each file.
type OwnershipMap struct {
	Owners map[string]*FileOwnership
	Rules  []OwnerRule
	mu     sync.RWMutex
}

// FileOwnership represents the ownership metadata for a single file.
type FileOwnership struct {
	Path         string
	PrimaryOwner string
	Contributors []Contributor
	LastModified time.Time
	TotalCommits int
}

// Contributor represents a person who has contributed to a file.
type Contributor struct {
	Name       string
	Commits    int
	Percentage float64
	LastActive time.Time
}

// OwnerRule represents a code ownership rule from a CODEOWNERS file or config.
type OwnerRule struct {
	Pattern string
	Owners  []string
	Source  string // "codeowners", "git_history", "config"
}

// NewOwnershipMap creates an empty OwnershipMap ready for population.
func NewOwnershipMap() *OwnershipMap {
	return &OwnershipMap{
		Owners: make(map[string]*FileOwnership),
	}
}

// BuildFromGitHistory populates the ownership map by analyzing git log output.
// It runs `git log --format=%an --name-only` to map files to contributors,
// calculates ownership percentages per file, and determines the primary owner.
func (om *OwnershipMap) BuildFromGitHistory(projectDir string) error {
	cmd := exec.Command("git", "log", "--format=COMMIT_AUTHOR:%an", "--name-only")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ownership: git log: %w", err)
	}

	// Parse git log output. We use a custom format marker so parsing is unambiguous.
	// Output looks like:
	//   COMMIT_AUTHOR:alice
	//
	//   file1.go
	//   file2.go
	//
	//   COMMIT_AUTHOR:bob
	//
	//   file3.go
	type commitEntry struct {
		author string
		files  []string
	}

	const marker = "COMMIT_AUTHOR:"
	var entries []commitEntry
	var current *commitEntry

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, marker) {
			// Save previous entry
			if current != nil && current.author != "" && len(current.files) > 0 {
				entries = append(entries, *current)
			}
			current = &commitEntry{author: strings.TrimPrefix(line, marker)}
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if current != nil {
			current.files = append(current.files, line)
		}
	}
	// Don't forget the last entry
	if current != nil && current.author != "" && len(current.files) > 0 {
		entries = append(entries, *current)
	}

	// Build per-file contributor data
	// fileContribs maps filepath -> (author -> commit count)
	fileContribs := make(map[string]map[string]int)
	for _, entry := range entries {
		for _, f := range entry.files {
			f = filepath.Clean(f)
			if fileContribs[f] == nil {
				fileContribs[f] = make(map[string]int)
			}
			fileContribs[f][entry.author]++
		}
	}

	om.mu.Lock()
	defer om.mu.Unlock()

	for path, contribs := range fileContribs {
		totalCommits := 0
		for _, count := range contribs {
			totalCommits += count
		}

		contributors := make([]Contributor, 0, len(contribs))
		for name, commits := range contribs {
			pct := 0.0
			if totalCommits > 0 {
				pct = float64(commits) / float64(totalCommits) * 100.0
			}
			contributors = append(contributors, Contributor{
				Name:       name,
				Commits:    commits,
				Percentage: pct,
			})
		}

		// Sort contributors by commit count descending
		sort.Slice(contributors, func(i, j int) bool {
			return contributors[i].Commits > contributors[j].Commits
		})

		primaryOwner := ""
		if len(contributors) > 0 {
			primaryOwner = contributors[0].Name
		}

		om.Owners[path] = &FileOwnership{
			Path:         path,
			PrimaryOwner: primaryOwner,
			Contributors: contributors,
			TotalCommits: totalCommits,
		}
	}

	return nil
}

// LoadCodeowners parses a CODEOWNERS file and adds the rules to the ownership map.
// The file format follows GitHub's CODEOWNERS convention:
//
//	pattern @owner1 @owner2
//
// Lines starting with # are comments. Blank lines are ignored.
func (om *OwnershipMap) LoadCodeowners(path string) error {
	out, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ownership: read codeowners %q: %w", path, err)
	}

	om.mu.Lock()
	defer om.mu.Unlock()

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		pattern := parts[0]
		owners := make([]string, 0, len(parts)-1)
		for _, owner := range parts[1:] {
			// Strip leading @ if present
			owners = append(owners, strings.TrimPrefix(owner, "@"))
		}

		om.Rules = append(om.Rules, OwnerRule{
			Pattern: pattern,
			Owners:  owners,
			Source:  "codeowners",
		})
	}

	return scanner.Err()
}

// GetOwner returns the FileOwnership for a given path, or nil if not tracked.
func (om *OwnershipMap) GetOwner(path string) *FileOwnership {
	om.mu.RLock()
	defer om.mu.RUnlock()

	path = filepath.Clean(path)

	// First check direct ownership from git history
	if fo, ok := om.Owners[path]; ok {
		return fo
	}

	// Check CODEOWNERS rules (last matching rule wins, per GitHub convention)
	var matchedRule *OwnerRule
	for i := range om.Rules {
		if matchGlobPattern(om.Rules[i].Pattern, path) {
			matchedRule = &om.Rules[i]
		}
	}

	if matchedRule != nil && len(matchedRule.Owners) > 0 {
		return &FileOwnership{
			Path:         path,
			PrimaryOwner: matchedRule.Owners[0],
		}
	}

	return nil
}

// GetOwnersByDirectory returns a mapping of directory path to the primary owner
// of that directory (the person with most commits in that directory).
func (om *OwnershipMap) GetOwnersByDirectory(dir string) map[string]string {
	om.mu.RLock()
	defer om.mu.RUnlock()

	dir = filepath.Clean(dir)

	// Aggregate commits per author per directory
	dirContribs := make(map[string]map[string]int) // dir -> (author -> commits)

	for path, fo := range om.Owners {
		fileDir := filepath.Dir(path)
		if dir != "." && dir != "" {
			if !strings.HasPrefix(fileDir, dir) {
				continue
			}
		}
		if dirContribs[fileDir] == nil {
			dirContribs[fileDir] = make(map[string]int)
		}
		for _, c := range fo.Contributors {
			dirContribs[fileDir][c.Name] += c.Commits
		}
	}

	result := make(map[string]string, len(dirContribs))
	for d, contribs := range dirContribs {
		bestAuthor := ""
		bestCount := 0
		for author, count := range contribs {
			if count > bestCount {
				bestCount = count
				bestAuthor = author
			}
		}
		if bestAuthor != "" {
			result[d] = bestAuthor
		}
	}

	return result
}

// FindExpertFor returns the best person to review changes to a given file path.
// It considers both git history and CODEOWNERS rules, preferring CODEOWNERS
// when available since those represent explicit ownership assignments.
func (om *OwnershipMap) FindExpertFor(path string) string {
	om.mu.RLock()
	defer om.mu.RUnlock()

	path = filepath.Clean(path)

	// Check CODEOWNERS rules first (last match wins)
	var matchedRule *OwnerRule
	for i := range om.Rules {
		if matchGlobPattern(om.Rules[i].Pattern, path) {
			matchedRule = &om.Rules[i]
		}
	}
	if matchedRule != nil && len(matchedRule.Owners) > 0 {
		return matchedRule.Owners[0]
	}

	// Fall back to git history
	if fo, ok := om.Owners[path]; ok && fo.PrimaryOwner != "" {
		return fo.PrimaryOwner
	}

	// Try to find an owner from the parent directory
	dir := filepath.Dir(path)
	bestAuthor := ""
	bestCommits := 0
	for p, fo := range om.Owners {
		if filepath.Dir(p) == dir {
			for _, c := range fo.Contributors {
				if c.Commits > bestCommits {
					bestCommits = c.Commits
					bestAuthor = c.Name
				}
			}
		}
	}

	return bestAuthor
}

// FormatOwnership produces a human-readable summary of code ownership.
// The limit parameter controls how many top owners to display.
func (om *OwnershipMap) FormatOwnership(limit int) string {
	om.mu.RLock()
	defer om.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	// Aggregate total commits per author and their directories
	authorCommits := make(map[string]int)
	authorDirs := make(map[string]map[string]bool)
	totalCommits := 0

	for path, fo := range om.Owners {
		for _, c := range fo.Contributors {
			authorCommits[c.Name] += c.Commits
			totalCommits += c.Commits
			if authorDirs[c.Name] == nil {
				authorDirs[c.Name] = make(map[string]bool)
			}
			dir := filepath.Dir(path)
			authorDirs[c.Name][dir] = true
		}
	}

	// Sort authors by total commits
	type authorStat struct {
		name    string
		commits int
		pct     float64
		dirs    []string
	}

	stats := make([]authorStat, 0, len(authorCommits))
	for name, commits := range authorCommits {
		pct := 0.0
		if totalCommits > 0 {
			pct = float64(commits) / float64(totalCommits) * 100.0
		}
		dirs := make([]string, 0, len(authorDirs[name]))
		for d := range authorDirs[name] {
			dirs = append(dirs, d)
		}
		sort.Strings(dirs)
		// Limit shown directories to top 3
		if len(dirs) > 3 {
			dirs = dirs[:3]
		}
		stats = append(stats, authorStat{
			name:    name,
			commits: commits,
			pct:     pct,
			dirs:    dirs,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].commits > stats[j].commits
	})

	if len(stats) > limit {
		stats = stats[:limit]
	}

	// Find unowned files (files with no contributors)
	var unowned []string
	for path, fo := range om.Owners {
		if fo.PrimaryOwner == "" || fo.TotalCommits == 0 {
			unowned = append(unowned, path)
		}
	}
	sort.Strings(unowned)

	// Build output
	var b strings.Builder
	b.WriteString("Code Ownership:\n")
	b.WriteString("────────────────────────────────\n")

	for _, s := range stats {
		dirList := strings.Join(s.dirs, ", ")
		b.WriteString(fmt.Sprintf("@%-8s %3.0f%% (%s)\n", s.name, s.pct, dirList))
	}

	if len(unowned) > 0 {
		b.WriteString(fmt.Sprintf("\nUnowned files: %d\n", len(unowned)))
		for _, f := range unowned {
			b.WriteString(fmt.Sprintf("  %s\n", f))
		}
	}

	return b.String()
}

// SuggestReviewers returns a deduplicated list of suggested reviewers for a set
// of changed files, based on ownership data. Reviewers are ordered by relevance
// (number of files they own among the changed set).
func (om *OwnershipMap) SuggestReviewers(changedFiles []string) []string {
	om.mu.RLock()
	defer om.mu.RUnlock()

	reviewerScore := make(map[string]int)

	for _, f := range changedFiles {
		f = filepath.Clean(f)

		// Check CODEOWNERS rules (last match wins)
		var matchedRule *OwnerRule
		for i := range om.Rules {
			if matchGlobPattern(om.Rules[i].Pattern, f) {
				matchedRule = &om.Rules[i]
			}
		}
		if matchedRule != nil {
			for _, owner := range matchedRule.Owners {
				reviewerScore[owner] += 2 // CODEOWNERS gets extra weight
			}
		}

		// Check git history
		if fo, ok := om.Owners[f]; ok {
			for _, c := range fo.Contributors {
				reviewerScore[c.Name] += c.Commits
			}
		}
	}

	// Sort by score descending
	type scored struct {
		name  string
		score int
	}
	var scoredList []scored
	for name, score := range reviewerScore {
		scoredList = append(scoredList, scored{name, score})
	}
	sort.Slice(scoredList, func(i, j int) bool {
		if scoredList[i].score == scoredList[j].score {
			return scoredList[i].name < scoredList[j].name
		}
		return scoredList[i].score > scoredList[j].score
	})

	result := make([]string, 0, len(scoredList))
	for _, s := range scoredList {
		result = append(result, s.name)
	}
	return result
}

// DetectBusFactorRisk identifies files where only one person has ever committed.
// These files represent a "bus factor" risk: if that person becomes unavailable,
// no one else has context on the code.
func (om *OwnershipMap) DetectBusFactorRisk() []string {
	om.mu.RLock()
	defer om.mu.RUnlock()

	var risks []string
	for _, fo := range om.Owners {
		if len(fo.Contributors) == 1 && fo.TotalCommits > 0 {
			risk := fmt.Sprintf("Bus factor risk: %s (only @%s)", fo.Path, fo.Contributors[0].Name)
			risks = append(risks, risk)
		}
	}
	sort.Strings(risks)
	return risks
}

// matchGlobPattern matches a path against a CODEOWNERS-style glob pattern.
// Supports:
//   - * matches anything except /
//   - ** matches everything including /
//   - ? matches any single character except /
//   - Leading / anchors to the repo root
//   - Trailing / matches directories
func matchGlobPattern(pattern, path string) bool {
	// Normalize
	pattern = strings.TrimSpace(pattern)
	path = filepath.ToSlash(filepath.Clean(path))

	// A trailing / in pattern means match directory prefix
	if strings.HasSuffix(pattern, "/") {
		dirPattern := strings.TrimSuffix(pattern, "/")
		dirPattern = strings.TrimPrefix(dirPattern, "/")
		return strings.HasPrefix(path, dirPattern+"/") || path == dirPattern
	}

	// Strip leading / for matching (it just means "anchored to root")
	anchored := strings.HasPrefix(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")

	// If pattern has no /, it can match any file name in any directory
	if !anchored && !strings.Contains(pattern, "/") {
		// Match against the basename
		base := filepath.Base(path)
		return globMatch(pattern, base)
	}

	// Full path match
	return globMatch(pattern, path)
}

// globMatch performs simple glob matching supporting *, **, and ?.
func globMatch(pattern, name string) bool {
	return deepMatch(pattern, name)
}

// deepMatch recursively matches pattern against name with support for ** globs.
func deepMatch(pattern, name string) bool {
	for len(pattern) > 0 {
		if len(name) == 0 {
			// Check if remaining pattern is only wildcards
			for i := 0; i < len(pattern); i++ {
				if pattern[i] != '*' {
					return false
				}
			}
			return true
		}

		switch pattern[0] {
		case '*':
			if len(pattern) > 1 && pattern[1] == '*' {
				// ** matches everything including /
				// Try matching rest of pattern at every position
				rest := pattern[2:]
				if len(rest) > 0 && rest[0] == '/' {
					rest = rest[1:]
				}
				if rest == "" {
					return true
				}
				for i := 0; i <= len(name); i++ {
					if deepMatch(rest, name[i:]) {
						return true
					}
				}
				return false
			}
			// * matches anything except /
			// Try matching rest of pattern after consuming 0..n non-slash chars
			for i := 0; i <= len(name); i++ {
				if i > 0 && name[i-1] == '/' {
					break
				}
				if deepMatch(pattern[1:], name[i:]) {
					return true
				}
			}
			return false

		case '?':
			if name[0] == '/' {
				return false
			}
			pattern = pattern[1:]
			name = name[1:]

		default:
			if pattern[0] != name[0] {
				return false
			}
			pattern = pattern[1:]
			name = name[1:]
		}
	}

	return len(name) == 0
}
