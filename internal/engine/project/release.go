package project

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ReleaseManager handles release automation including changelog generation,
// version bumping, and release preparation.
type ReleaseManager struct {
	ProjectDir     string
	CurrentVersion string
	mu             sync.Mutex
}

// Release represents a prepared release with all associated metadata.
type Release struct {
	Version         string
	Date            time.Time
	Changes         []ChangeEntry
	BreakingChanges []ChangeEntry
	Contributors    []string
	Stats           ReleaseStats
}

// ChangeEntry represents a single change parsed from a commit message.
type ChangeEntry struct {
	Type        string // "feat", "fix", "refactor", "perf", "docs", "test", "chore"
	Scope       string
	Description string
	CommitHash  string
	Author      string
	Breaking    bool
}

// ReleaseStats holds numerical statistics for a release.
type ReleaseStats struct {
	Commits      int
	FilesChanged int
	Additions    int
	Deletions    int
	Contributors int
}

// conventionalCommitRe matches conventional commit format: type(scope): description
var conventionalCommitRe = regexp.MustCompile(`^(\w+)(\(([^)]*)\))?(!)?:\s*(.+)`)

// NewReleaseManager creates a new ReleaseManager for the given project directory.
func NewReleaseManager(projectDir string) *ReleaseManager {
	return &ReleaseManager{
		ProjectDir: projectDir,
	}
}

// DetectCurrentVersion reads the current version from git tags, go.mod, package.json, or Cargo.toml.
func (rm *ReleaseManager) DetectCurrentVersion() (string, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Try git tags first
	cmd := exec.CommandContext(context.Background(), "git", "describe", "--tags", "--abbrev=0")
	cmd.Dir = rm.ProjectDir
	if output, err := cmd.Output(); err == nil {
		version := strings.TrimSpace(string(output))
		version = strings.TrimPrefix(version, "v")
		if isValidSemver(version) {
			rm.CurrentVersion = version
			return version, nil
		}
	}

	// Try package.json
	pkgJSON := filepath.Join(rm.ProjectDir, "package.json")
	if data, err := os.ReadFile(pkgJSON); err == nil { // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
		var pkg map[string]interface{}
		if err := json.Unmarshal(data, &pkg); err == nil {
			if v, ok := pkg["version"].(string); ok && isValidSemver(v) {
				rm.CurrentVersion = v
				return v, nil
			}
		}
	}

	// Try Cargo.toml
	cargoToml := filepath.Join(rm.ProjectDir, "Cargo.toml")
	if f, err := os.Open(cargoToml); err == nil { // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
		defer func() { _ = f.Close() }()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(strings.TrimSpace(line), "version") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					v := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
					if isValidSemver(v) {
						rm.CurrentVersion = v
						return v, nil
					}
				}
			}
		}
	}

	// Try go.mod (look for a version comment or module version)
	goMod := filepath.Join(rm.ProjectDir, "go.mod")
	if f, err := os.Open(goMod); err == nil { // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
		defer func() { _ = f.Close() }()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			// Look for // version: X.Y.Z comment
			if strings.Contains(line, "// version:") {
				parts := strings.SplitN(line, "// version:", 2)
				if len(parts) == 2 {
					v := strings.TrimSpace(parts[1])
					if isValidSemver(v) {
						rm.CurrentVersion = v
						return v, nil
					}
				}
			}
		}
	}

	// Default to 0.0.0 if nothing found
	rm.CurrentVersion = "0.0.0"
	return "0.0.0", nil
}

// ParseConventionalCommit parses a commit message in conventional commit format.
// Returns nil if the message cannot be parsed.
func ParseConventionalCommit(msg string) *ChangeEntry {
	lines := strings.SplitN(msg, "\n", 2)
	subject := strings.TrimSpace(lines[0])

	matches := conventionalCommitRe.FindStringSubmatch(subject)
	if matches == nil {
		return nil
	}

	entry := &ChangeEntry{
		Type:        matches[1],
		Scope:       matches[3],
		Description: strings.TrimSpace(matches[5]),
		Breaking:    matches[4] == "!",
	}

	// Check for BREAKING CHANGE in body
	if len(lines) > 1 {
		body := lines[1]
		if strings.Contains(body, "BREAKING CHANGE:") || strings.Contains(body, "BREAKING-CHANGE:") {
			entry.Breaking = true
		}
	}

	return entry
}

// classifyNonConventional classifies a non-conventional commit message into a ChangeEntry.
func classifyNonConventional(msg string) *ChangeEntry {
	subject := strings.SplitN(msg, "\n", 2)[0]
	subject = strings.TrimSpace(subject)
	lower := strings.ToLower(subject)

	entry := &ChangeEntry{
		Description: subject,
		Type:        "chore",
	}

	switch {
	case strings.HasPrefix(lower, "fix") || strings.Contains(lower, "bug") || strings.Contains(lower, "patch"):
		entry.Type = "fix"
	case strings.Contains(lower, "test"):
		entry.Type = "test"
	case strings.HasPrefix(lower, "add") || strings.HasPrefix(lower, "feat") || strings.Contains(lower, "feature") || strings.Contains(lower, "implement"):
		entry.Type = "feat"
	case strings.Contains(lower, "refactor") || strings.Contains(lower, "restructure") || strings.Contains(lower, "reorganize"):
		entry.Type = "refactor"
	case strings.Contains(lower, "perf") || strings.Contains(lower, "speed") || strings.Contains(lower, "optim"):
		entry.Type = "perf"
	case strings.Contains(lower, "doc") || strings.Contains(lower, "readme"):
		entry.Type = "docs"
	}

	return entry
}

// GatherChanges collects and parses all commits since the given tag.
func (rm *ReleaseManager) GatherChanges(sinceTag string) ([]ChangeEntry, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	var logRange string
	if sinceTag != "" {
		logRange = sinceTag + "..HEAD"
	} else {
		logRange = "HEAD"
	}

	// Get commits with hash, author, and message
	cmd := exec.CommandContext(context.Background(), "git", "log", logRange, "--pretty=format:%H|%an|%s%n%b%n---END---") // #nosec G204 -- git subcommand invocation with fixed subcommand and internally-derived args
	cmd.Dir = rm.ProjectDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run git log: %w", err)
	}

	raw := string(output)
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	var entries []ChangeEntry
	commits := strings.Split(raw, "---END---")

	for _, commitBlock := range commits {
		commitBlock = strings.TrimSpace(commitBlock)
		if commitBlock == "" {
			continue
		}

		lines := strings.SplitN(commitBlock, "\n", 2)
		if len(lines) == 0 {
			continue
		}

		headerLine := lines[0]
		parts := strings.SplitN(headerLine, "|", 3)
		if len(parts) < 3 {
			continue
		}

		hash := parts[0]
		author := parts[1]
		subject := parts[2]

		// Reconstruct full message with body
		fullMsg := subject
		if len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
			fullMsg = subject + "\n" + lines[1]
		}

		entry := ParseConventionalCommit(fullMsg)
		if entry == nil {
			entry = classifyNonConventional(fullMsg)
		}

		entry.CommitHash = hash
		entry.Author = author
		entries = append(entries, *entry)
	}

	return entries, nil
}

// BumpVersion determines the next version based on the changes.
// Breaking changes cause a major bump, features cause a minor bump,
// and fixes/other changes cause a patch bump.
func BumpVersion(current string, changes []ChangeEntry) string {
	parts := strings.Split(current, ".")
	if len(parts) != 3 {
		return "0.1.0"
	}

	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])

	hasBreaking := false
	hasFeature := false

	for _, c := range changes {
		if c.Breaking {
			hasBreaking = true
			break
		}
		if c.Type == "feat" {
			hasFeature = true
		}
	}

	switch {
	case hasBreaking:
		if major == 0 {
			// Pre-1.0: breaking change bumps minor
			return fmt.Sprintf("0.%d.0", minor+1)
		}
		return fmt.Sprintf("%d.0.0", major+1)
	case hasFeature:
		return fmt.Sprintf("%d.%d.0", major, minor+1)
	default:
		return fmt.Sprintf("%d.%d.%d", major, minor, patch+1)
	}
}

// GenerateChangelog produces a markdown-formatted changelog for a release.
func GenerateChangelog(release *Release) string {
	var b strings.Builder

	dateStr := release.Date.Format("2006-01-02")
	b.WriteString(fmt.Sprintf("## v%s (%s)\n", release.Version, dateStr))

	// Group changes by type
	groups := map[string][]ChangeEntry{
		"feat":     {},
		"fix":      {},
		"perf":     {},
		"refactor": {},
		"docs":     {},
		"test":     {},
		"chore":    {},
	}

	for _, c := range release.Changes {
		if _, ok := groups[c.Type]; ok {
			groups[c.Type] = append(groups[c.Type], c)
		} else {
			groups["chore"] = append(groups["chore"], c)
		}
	}

	typeHeaders := []struct {
		key   string
		title string
	}{
		{"feat", "Features"},
		{"fix", "Bug Fixes"},
		{"perf", "Performance"},
		{"refactor", "Refactoring"},
		{"docs", "Documentation"},
		{"test", "Tests"},
		{"chore", "Chores"},
	}

	for _, th := range typeHeaders {
		entries := groups[th.key]
		if len(entries) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("\n### %s\n", th.title))
		for _, e := range entries {
			if e.Scope != "" {
				b.WriteString(fmt.Sprintf("- **%s**: %s", e.Scope, e.Description))
			} else {
				b.WriteString(fmt.Sprintf("- %s", e.Description))
			}
			if e.CommitHash != "" && len(e.CommitHash) >= 7 {
				b.WriteString(fmt.Sprintf(" (%s)", e.CommitHash[:7]))
			}
			b.WriteString("\n")
		}
	}

	// Breaking changes section
	if len(release.BreakingChanges) > 0 {
		b.WriteString("\n### Breaking Changes\n")
		for _, e := range release.BreakingChanges {
			if e.Scope != "" {
				b.WriteString(fmt.Sprintf("- **%s**: %s", e.Scope, e.Description))
			} else {
				b.WriteString(fmt.Sprintf("- %s", e.Description))
			}
			b.WriteString("\n")
		}
	}

	// Stats
	b.WriteString(fmt.Sprintf(
		"\n### Stats\n%d commits, %d files changed, +%s -%s, %d contributors\n",
		release.Stats.Commits,
		release.Stats.FilesChanged,
		formatNumber(release.Stats.Additions),
		formatNumber(release.Stats.Deletions),
		release.Stats.Contributors,
	))

	return b.String()
}

// PrepareRelease gathers changes since the last tag, bumps the version,
// generates changelog, and returns the release ready for review.
func (rm *ReleaseManager) PrepareRelease() (*Release, error) {
	currentVersion, err := rm.DetectCurrentVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to detect current version: %w", err)
	}

	sinceTag := ""
	if currentVersion != "0.0.0" {
		// Try with v prefix first, then without
		sinceTag = "v" + currentVersion
		cmd := exec.CommandContext(context.Background(), "git", "rev-parse", sinceTag)
		cmd.Dir = rm.ProjectDir
		if runErr := cmd.Run(); runErr != nil {
			sinceTag = currentVersion
			cmd2 := exec.CommandContext(context.Background(), "git", "rev-parse", sinceTag)
			cmd2.Dir = rm.ProjectDir
			if runErr := cmd2.Run(); runErr != nil {
				sinceTag = ""
			}
		}
	}

	changes, err := rm.GatherChanges(sinceTag)
	if err != nil {
		return nil, fmt.Errorf("failed to gather changes: %w", err)
	}

	newVersion := BumpVersion(currentVersion, changes)

	// Separate breaking changes
	var breakingChanges []ChangeEntry
	for _, c := range changes {
		if c.Breaking {
			breakingChanges = append(breakingChanges, c)
		}
	}

	// Collect unique contributors
	contributorSet := make(map[string]struct{})
	for _, c := range changes {
		if c.Author != "" {
			contributorSet[c.Author] = struct{}{}
		}
	}
	var contributors []string
	for name := range contributorSet {
		contributors = append(contributors, name)
	}
	sort.Strings(contributors)

	// Get diff stats
	stats := rm.gatherStats(sinceTag, len(changes), len(contributors))

	release := &Release{
		Version:         newVersion,
		Date:            time.Now(),
		Changes:         changes,
		BreakingChanges: breakingChanges,
		Contributors:    contributors,
		Stats:           stats,
	}

	return release, nil
}

// gatherStats collects numerical stats for the release.
func (rm *ReleaseManager) gatherStats(sinceTag string, commitCount, contributorCount int) ReleaseStats {
	stats := ReleaseStats{
		Commits:      commitCount,
		Contributors: contributorCount,
	}

	var diffRange string
	if sinceTag != "" {
		diffRange = sinceTag + "..HEAD"
	} else {
		diffRange = "HEAD"
	}

	cmd := exec.CommandContext(context.Background(), "git", "diff", "--stat", diffRange) // #nosec G204 -- git subcommand invocation with fixed subcommand and internally-derived args
	cmd.Dir = rm.ProjectDir
	output, err := cmd.Output()
	if err != nil {
		return stats
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return stats
	}

	// Parse the summary line (last line): "X files changed, Y insertions(+), Z deletions(-)"
	summary := lines[len(lines)-1]
	statsRe := regexp.MustCompile(`(\d+) files? changed`)
	addRe := regexp.MustCompile(`(\d+) insertions?\(\+\)`)
	delRe := regexp.MustCompile(`(\d+) deletions?\(-\)`)

	if m := statsRe.FindStringSubmatch(summary); m != nil {
		stats.FilesChanged, _ = strconv.Atoi(m[1])
	}
	if m := addRe.FindStringSubmatch(summary); m != nil {
		stats.Additions, _ = strconv.Atoi(m[1])
	}
	if m := delRe.FindStringSubmatch(summary); m != nil {
		stats.Deletions, _ = strconv.Atoi(m[1])
	}

	return stats
}

// FormatReleaseNotes produces GitHub-style release notes with sections.
func FormatReleaseNotes(release *Release) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Release v%s\n\n", release.Version))
	b.WriteString(fmt.Sprintf("**Release Date:** %s\n\n", release.Date.Format("January 2, 2006")))

	// Highlights section
	if len(release.Changes) > 0 {
		b.WriteString("## Highlights\n\n")
		// Show features first as highlights
		for _, c := range release.Changes {
			if c.Type == "feat" {
				if c.Scope != "" {
					b.WriteString(fmt.Sprintf("- **%s**: %s\n", c.Scope, c.Description))
				} else {
					b.WriteString(fmt.Sprintf("- %s\n", c.Description))
				}
			}
		}
		b.WriteString("\n")
	}

	// Breaking changes warning
	if len(release.BreakingChanges) > 0 {
		b.WriteString("## Breaking Changes\n\n")
		b.WriteString("> **Warning:** This release contains breaking changes.\n\n")
		for _, c := range release.BreakingChanges {
			if c.Scope != "" {
				b.WriteString(fmt.Sprintf("- **%s**: %s\n", c.Scope, c.Description))
			} else {
				b.WriteString(fmt.Sprintf("- %s\n", c.Description))
			}
		}
		b.WriteString("\n")
	}

	// All changes grouped
	groups := map[string][]ChangeEntry{}
	for _, c := range release.Changes {
		groups[c.Type] = append(groups[c.Type], c)
	}

	typeHeaders := []struct {
		key   string
		title string
	}{
		{"feat", "New Features"},
		{"fix", "Bug Fixes"},
		{"perf", "Performance Improvements"},
		{"refactor", "Code Refactoring"},
		{"docs", "Documentation"},
		{"test", "Tests"},
		{"chore", "Maintenance"},
	}

	for _, th := range typeHeaders {
		entries := groups[th.key]
		if len(entries) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s\n\n", th.title))
		for _, e := range entries {
			if e.Scope != "" {
				b.WriteString(fmt.Sprintf("- **%s**: %s", e.Scope, e.Description))
			} else {
				b.WriteString(fmt.Sprintf("- %s", e.Description))
			}
			if e.CommitHash != "" && len(e.CommitHash) >= 7 {
				b.WriteString(fmt.Sprintf(" (`%s`)", e.CommitHash[:7]))
			}
			if e.Author != "" {
				b.WriteString(fmt.Sprintf(" - @%s", e.Author))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Contributors
	if len(release.Contributors) > 0 {
		b.WriteString("## Contributors\n\n")
		for _, c := range release.Contributors {
			b.WriteString(fmt.Sprintf("- @%s\n", c))
		}
		b.WriteString("\n")
	}

	// Stats footer
	b.WriteString("---\n\n")
	b.WriteString(fmt.Sprintf(
		"**Full Changelog:** %d commits, %d files changed, +%s -%s\n",
		release.Stats.Commits,
		release.Stats.FilesChanged,
		formatNumber(release.Stats.Additions),
		formatNumber(release.Stats.Deletions),
	))

	return b.String()
}

// UpdateVersionFile updates the version string in the given file.
// Supports Go constant files, package.json, and Cargo.toml formats.
func UpdateVersionFile(version, filePath string) error {
	data, err := os.ReadFile(filePath) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	content := string(data)
	ext := filepath.Ext(filePath)
	base := filepath.Base(filePath)

	var updated string

	switch {
	case base == "package.json":
		// Update "version": "x.y.z"
		re := regexp.MustCompile(`("version"\s*:\s*")([^"]+)(")`)
		updated = re.ReplaceAllString(content, "${1}"+version+"${3}")

	case base == "Cargo.toml":
		// Update version = "x.y.z"
		re := regexp.MustCompile(`(version\s*=\s*")([^"]+)(")`)
		updated = re.ReplaceAllString(content, "${1}"+version+"${3}")

	case ext == ".go":
		// Update Version = "x.y.z" or Version string = "x.y.z"
		re := regexp.MustCompile(`((?:Version|VERSION)\s*(?:string\s*)?=\s*")([^"]+)(")`)
		updated = re.ReplaceAllString(content, "${1}"+version+"${3}")

	default:
		// Generic: try to find version-like patterns
		re := regexp.MustCompile(`(version\s*[:=]\s*["']?)(\d+\.\d+\.\d+)(["']?)`)
		updated = re.ReplaceAllString(content, "${1}"+version+"${3}")
	}

	if updated == content {
		return fmt.Errorf("no version pattern found in %s", filePath)
	}

	if err := os.WriteFile(filePath, []byte(updated), 0o600); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return nil
}

// ValidateRelease checks that a release is valid and ready to be published.
// Returns a list of validation issues (empty if valid).
func (rm *ReleaseManager) ValidateRelease(release *Release) []string {
	var issues []string

	if release == nil {
		return []string{"release is nil"}
	}

	// Check has changes
	if len(release.Changes) == 0 {
		issues = append(issues, "release has no changes")
	}

	// Check version is valid semver
	if !isValidSemver(release.Version) {
		issues = append(issues, fmt.Sprintf("invalid version: %s", release.Version))
	}

	// Check version is not empty
	if release.Version == "" {
		issues = append(issues, "version is empty")
	}

	// Check for uncommitted files
	cmd := exec.CommandContext(context.Background(), "git", "status", "--porcelain")
	cmd.Dir = rm.ProjectDir
	output, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(output)) != "" {
		issues = append(issues, "there are uncommitted changes in the working directory")
	}

	// Check date is set
	if release.Date.IsZero() {
		issues = append(issues, "release date is not set")
	}

	return issues
}

// isValidSemver checks if a string is a valid semantic version (X.Y.Z).
func isValidSemver(v string) bool {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
	}
	return true
}

// formatNumber formats an integer with comma separators for thousands.
func formatNumber(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}

	s := strconv.Itoa(n)
	var result []byte
	for i, digit := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(digit))
	}
	return string(result)
}
