package git

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GitContext provides git-aware context enrichment for files and sessions.
type GitContext struct {
	RepoDir string
	mu      sync.RWMutex
	cache   map[string]*GitFileInfo
}

// GitFileInfo holds git metadata about a specific file.
type GitFileInfo struct {
	Path          string
	LastAuthor    string
	LastCommitMsg string
	LastModified  time.Time
	CommitCount   int
	Contributors  []string
	RecentCommits []CommitInfo
	Blame         []BlameLine
}

// CommitInfo represents a single git commit.
type CommitInfo struct {
	Hash         string
	Author       string
	Date         time.Time
	Message      string
	FilesChanged int
}

// BlameLine represents blame info for a single line.
type BlameLine struct {
	LineNo int
	Author string
	Date   time.Time
	Commit string
}

// NewGitContext creates a new GitContext for the given repo directory.
func NewGitContext(repoDir string) *GitContext {
	return &GitContext{
		RepoDir: repoDir,
		cache:   make(map[string]*GitFileInfo),
	}
}

// runGit executes a git command in the repo directory and returns its output.
func (gc *GitContext) runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = gc.RepoDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// GetFileInfo returns git metadata for a file, using cache if available.
func (gc *GitContext) GetFileInfo(path string) (*GitFileInfo, error) {
	gc.mu.RLock()
	if info, ok := gc.cache[path]; ok {
		gc.mu.RUnlock()
		return info, nil
	}
	gc.mu.RUnlock()

	info := &GitFileInfo{Path: path}

	// Get last commit info for this file
	logOut, err := gc.runGit("log", "-1", "--format=%H%n%an%n%aI%n%s", "--", path)
	if err != nil {
		return nil, fmt.Errorf("git log for %s: %w", path, err)
	}
	lines := strings.Split(logOut, "\n")
	if len(lines) >= 4 {
		info.LastAuthor = lines[1]
		if t, err := time.Parse(time.RFC3339, lines[2]); err == nil {
			info.LastModified = t
		}
		info.LastCommitMsg = lines[3]
	}

	// Count commits touching this file
	countOut, err := gc.runGit("rev-list", "--count", "HEAD", "--", path)
	if err == nil {
		if n, err := strconv.Atoi(countOut); err == nil {
			info.CommitCount = n
		}
	}

	// Get unique contributors
	contribOut, err := gc.runGit("log", "--format=%an", "--", path)
	if err == nil && contribOut != "" {
		authorLines := strings.Split(contribOut, "\n")
		authorCount := make(map[string]int)
		for _, a := range authorLines {
			a = strings.TrimSpace(a)
			if a != "" {
				authorCount[a]++
			}
		}
		type ac struct {
			name  string
			count int
		}
		var sorted []ac
		for name, count := range authorCount {
			sorted = append(sorted, ac{name, count})
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].count > sorted[j].count
		})
		for _, s := range sorted {
			info.Contributors = append(info.Contributors, s.name)
		}
	}

	// Get recent commits (last 10)
	recentOut, err := gc.runGit("log", "-10", "--format=%H|%an|%aI|%s", "--", path)
	if err == nil && recentOut != "" {
		for _, line := range strings.Split(recentOut, "\n") {
			parts := strings.SplitN(line, "|", 4)
			if len(parts) == 4 {
				ci := CommitInfo{
					Hash:    parts[0],
					Author:  parts[1],
					Message: parts[3],
				}
				if t, err := time.Parse(time.RFC3339, parts[2]); err == nil {
					ci.Date = t
				}
				info.RecentCommits = append(info.RecentCommits, ci)
			}
		}
	}

	gc.mu.Lock()
	gc.cache[path] = info
	gc.mu.Unlock()

	return info, nil
}

// GetRecentChanges returns commits from the last N days.
func (gc *GitContext) GetRecentChanges(days int) ([]CommitInfo, error) {
	since := fmt.Sprintf("--since=%d.days.ago", days)
	out, err := gc.runGit("log", since, "--format=%H|%an|%aI|%s", "--shortstat")
	if err != nil {
		return nil, fmt.Errorf("git log recent changes: %w", err)
	}
	if out == "" {
		return nil, nil
	}

	var commits []CommitInfo
	lines := strings.Split(out, "\n")
	filesChangedRe := regexp.MustCompile(`(\d+) files? changed`)

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) == 4 {
			ci := CommitInfo{
				Hash:    parts[0],
				Author:  parts[1],
				Message: parts[3],
			}
			if t, err := time.Parse(time.RFC3339, parts[2]); err == nil {
				ci.Date = t
			}
			// Look ahead for shortstat line
			if i+1 < len(lines) {
				statLine := strings.TrimSpace(lines[i+1])
				if matches := filesChangedRe.FindStringSubmatch(statLine); len(matches) > 1 {
					if n, err := strconv.Atoi(matches[1]); err == nil {
						ci.FilesChanged = n
					}
					i++ // skip the stat line
				}
			}
			commits = append(commits, ci)
		}
	}
	return commits, nil
}

// GetBranch returns the current branch name.
func (gc *GitContext) GetBranch() (string, error) {
	branch, err := gc.runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("get branch: %w", err)
	}
	return branch, nil
}

// GetUncommitted returns a list of modified or staged files.
func (gc *GitContext) GetUncommitted() ([]string, error) {
	out, err := gc.runGit("status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	if out == "" {
		return nil, nil
	}

	var files []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 3 {
			// Porcelain format: XY filename
			file := strings.TrimSpace(line[2:])
			// Handle renames "old -> new"
			if idx := strings.Index(file, " -> "); idx >= 0 {
				file = file[idx+4:]
			}
			files = append(files, file)
		}
	}
	return files, nil
}

// GetBlame returns blame info for lines startLine to endLine of the given file.
func (gc *GitContext) GetBlame(path string, startLine, endLine int) ([]BlameLine, error) {
	lineRange := fmt.Sprintf("%d,%d", startLine, endLine)
	out, err := gc.runGit("blame", "-L", lineRange, "--porcelain", path)
	if err != nil {
		return nil, fmt.Errorf("git blame %s: %w", path, err)
	}
	if out == "" {
		return nil, nil
	}

	var blameLines []BlameLine
	lines := strings.Split(out, "\n")
	lineNo := startLine

	commitRe := regexp.MustCompile(`^([0-9a-f]{40})\s+\d+\s+(\d+)`)
	authorRe := regexp.MustCompile(`^author (.+)`)
	dateRe := regexp.MustCompile(`^author-time (\d+)`)

	var currentCommit string
	var currentAuthor string
	var currentDate time.Time

	for _, line := range lines {
		if matches := commitRe.FindStringSubmatch(line); len(matches) > 2 {
			currentCommit = matches[1]
			if n, err := strconv.Atoi(matches[2]); err == nil {
				lineNo = n
			}
		} else if matches := authorRe.FindStringSubmatch(line); len(matches) > 1 {
			currentAuthor = matches[1]
		} else if matches := dateRe.FindStringSubmatch(line); len(matches) > 1 {
			if ts, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
				currentDate = time.Unix(ts, 0)
			}
		} else if strings.HasPrefix(line, "\t") {
			// This is the content line, marks end of a blame entry
			blameLines = append(blameLines, BlameLine{
				LineNo: lineNo,
				Author: currentAuthor,
				Date:   currentDate,
				Commit: currentCommit,
			})
		}
	}
	return blameLines, nil
}

// GetRelatedFiles finds files frequently modified together with the given file.
func (gc *GitContext) GetRelatedFiles(path string) ([]string, error) {
	// Get commits that touched this file
	out, err := gc.runGit("log", "--format=%H", "-20", "--", path)
	if err != nil {
		return nil, fmt.Errorf("git log for related files: %w", err)
	}
	if out == "" {
		return nil, nil
	}

	hashes := strings.Split(out, "\n")
	fileCount := make(map[string]int)

	for _, hash := range hashes {
		hash = strings.TrimSpace(hash)
		if hash == "" {
			continue
		}
		filesOut, err := gc.runGit("diff-tree", "--no-commit-id", "-r", "--name-only", hash)
		if err != nil {
			continue
		}
		for _, f := range strings.Split(filesOut, "\n") {
			f = strings.TrimSpace(f)
			if f != "" && f != path {
				fileCount[f]++
			}
		}
	}

	type fc struct {
		file  string
		count int
	}
	var sorted []fc
	for file, count := range fileCount {
		if count >= 2 { // Only include files that co-changed at least twice
			sorted = append(sorted, fc{file, count})
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	var related []string
	limit := 10
	if len(sorted) < limit {
		limit = len(sorted)
	}
	for i := 0; i < limit; i++ {
		related = append(related, sorted[i].file)
	}
	return related, nil
}

// BuildContextForFile gathers all git context for a file and formats it as a readable string.
func (gc *GitContext) BuildContextForFile(path string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Git Context for %s\n", path))

	info, err := gc.GetFileInfo(path)
	if err != nil {
		sb.WriteString(fmt.Sprintf("Error: %v\n", err))
		return sb.String()
	}

	// Last modified
	ago := formatTimeAgo(info.LastModified)
	sb.WriteString(fmt.Sprintf("Last modified: %s by @%s (\"%s\")\n", ago, info.LastAuthor, info.LastCommitMsg))

	// Contributors with percentages
	if len(info.Contributors) > 0 && info.CommitCount > 0 {
		// Recalculate percentages from contributor commit counts
		contribOut, _ := gc.runGit("log", "--format=%an", "--", path)
		if contribOut != "" {
			authorCount := make(map[string]int)
			total := 0
			for _, a := range strings.Split(contribOut, "\n") {
				a = strings.TrimSpace(a)
				if a != "" {
					authorCount[a]++
					total++
				}
			}
			type ac struct {
				name  string
				count int
			}
			var sorted []ac
			for name, count := range authorCount {
				sorted = append(sorted, ac{name, count})
			}
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].count > sorted[j].count
			})
			var parts []string
			for _, s := range sorted {
				pct := (s.count * 100) / total
				parts = append(parts, fmt.Sprintf("%s (%d%%)", s.name, pct))
			}
			sb.WriteString(fmt.Sprintf("Contributors: %s\n", strings.Join(parts, ", ")))
		}
	}

	// Recent changes
	recentCommits, _ := gc.GetRecentChanges(7)
	commitCount := 0
	for _, c := range recentCommits {
		// Check if this commit touched the file
		filesOut, err := gc.runGit("diff-tree", "--no-commit-id", "-r", "--name-only", c.Hash)
		if err == nil {
			for _, f := range strings.Split(filesOut, "\n") {
				if strings.TrimSpace(f) == path {
					commitCount++
					break
				}
			}
		}
	}
	sb.WriteString(fmt.Sprintf("Recent changes (last 7 days): %d commits\n", commitCount))

	// Related files
	related, _ := gc.GetRelatedFiles(path)
	if len(related) > 0 {
		limit := 3
		if len(related) < limit {
			limit = len(related)
		}
		sb.WriteString(fmt.Sprintf("Related files: %s\n", strings.Join(related[:limit], ", ")))
	}

	// Branch
	branch, err := gc.GetBranch()
	if err == nil {
		sb.WriteString(fmt.Sprintf("Branch: %s\n", branch))
	}

	return sb.String()
}

// BuildContextForSession returns overall repository context.
func (gc *GitContext) BuildContextForSession() string {
	var sb strings.Builder
	sb.WriteString("## Repository Context\n")

	// Branch info
	branch, err := gc.GetBranch()
	if err == nil {
		// Check how far ahead of main
		aheadBehind := ""
		for _, base := range []string{"main", "master"} {
			out, err := gc.runGit("rev-list", "--count", base+"..HEAD")
			if err == nil {
				if n, _ := strconv.Atoi(out); n > 0 {
					aheadBehind = fmt.Sprintf(" (ahead of %s by %d commits)", base, n)
				}
				break
			}
		}
		sb.WriteString(fmt.Sprintf("Branch: %s%s\n", branch, aheadBehind))
	}

	// Uncommitted files
	uncommitted, err := gc.GetUncommitted()
	if err == nil {
		if len(uncommitted) > 0 {
			sb.WriteString(fmt.Sprintf("Uncommitted: %d files modified\n", len(uncommitted)))
		} else {
			sb.WriteString("Uncommitted: clean working tree\n")
		}
	}

	// Recent activity
	recentCommits, err := gc.GetRecentChanges(7)
	if err == nil {
		sb.WriteString(fmt.Sprintf("Recent activity: %d commits in last 7 days\n", len(recentCommits)))

		// Active contributors
		authors := make(map[string]bool)
		for _, c := range recentCommits {
			authors[c.Author] = true
		}
		if len(authors) > 0 {
			var authorList []string
			for a := range authors {
				authorList = append(authorList, a)
			}
			sort.Strings(authorList)
			sb.WriteString(fmt.Sprintf("Active contributors: %s\n", strings.Join(authorList, ", ")))
		}
	}

	return sb.String()
}

// GetDiffSummary summarizes current uncommitted changes.
func (gc *GitContext) GetDiffSummary() (string, error) {
	// Staged changes
	staged, err := gc.runGit("diff", "--cached", "--stat")
	if err != nil {
		return "", fmt.Errorf("git diff cached: %w", err)
	}

	// Unstaged changes
	unstaged, err := gc.runGit("diff", "--stat")
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}

	var sb strings.Builder
	if staged != "" {
		sb.WriteString("Staged changes:\n")
		sb.WriteString(staged)
		sb.WriteString("\n")
	}
	if unstaged != "" {
		sb.WriteString("Unstaged changes:\n")
		sb.WriteString(unstaged)
		sb.WriteString("\n")
	}
	if staged == "" && unstaged == "" {
		sb.WriteString("No uncommitted changes\n")
	}
	return sb.String(), nil
}

// IsRecentlyModified checks if the file was modified within the given duration.
func (gc *GitContext) IsRecentlyModified(path string, within time.Duration) bool {
	out, err := gc.runGit("log", "-1", "--format=%aI", "--", path)
	if err != nil || out == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, out)
	if err != nil {
		return false
	}
	return time.Since(t) <= within
}

// formatTimeAgo formats a time as a human-readable relative string.
func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
