package engine

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// DiffSummary holds the full summarized output of a diff analysis.
type DiffSummary struct {
	Files          []FileSummary
	OverallSummary string
	ChangeType     string // "feature", "bugfix", "refactor", "test", "docs", "config"
	Impact         string // "low", "medium", "high"
	AffectedAreas  []string
}

// FileSummary describes summarized changes for a single file in the diff.
type FileSummary struct {
	Path       string
	Action     string // "added", "modified", "deleted"
	Summary    string
	Additions  int
	Deletions  int
	KeyChanges []string
}

// DiffSummarizer parses unified diffs and produces human-readable summaries.
type DiffSummarizer struct {
	mu sync.Mutex
}

// NewDiffSummarizer creates a new DiffSummarizer instance.
func NewDiffSummarizer() *DiffSummarizer {
	return &DiffSummarizer{}
}

// Summarize parses a unified diff and returns a complete DiffSummary.
func (ds *DiffSummarizer) Summarize(diff string) *DiffSummary {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if diff == "" {
		return &DiffSummary{
			Files:          []FileSummary{},
			OverallSummary: "No changes",
			ChangeType:     "refactor",
			Impact:         "low",
			AffectedAreas:  []string{},
		}
	}

	files := ds.parseDiffFiles(diff)
	var summaries []FileSummary
	for _, f := range files {
		fs := ds.SummarizeFile(f.path, f.hunks)
		fs.Action = f.action
		summaries = append(summaries, *fs)
	}

	summary := &DiffSummary{
		Files:         summaries,
		AffectedAreas: ds.extractAffectedAreas(summaries),
	}

	summary.ChangeType = ds.DetectChangeType(summary)
	summary.Impact = ds.AssessImpact(summary)
	summary.OverallSummary = ds.generateOverallSummary(summary)

	return summary
}

// parsedFile holds intermediate parsing state for a single file in the diff.
type parsedFile struct {
	path   string
	action string
	hunks  []string
}

// parseDiffFiles splits a unified diff into per-file sections.
func (ds *DiffSummarizer) parseDiffFiles(diff string) []parsedFile {
	var files []parsedFile
	lines := strings.Split(diff, "\n")

	var current *parsedFile
	var currentHunk strings.Builder
	inHunk := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(line, "diff --git") {
			// Flush previous hunk
			if inHunk && current != nil {
				current.hunks = append(current.hunks, currentHunk.String())
				currentHunk.Reset()
				inHunk = false
			}
			// Flush previous file
			if current != nil {
				files = append(files, *current)
			}
			current = &parsedFile{action: "modified"}
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "--- /dev/null") {
			current.action = "added"
			continue
		}
		if strings.HasPrefix(line, "+++ /dev/null") {
			current.action = "deleted"
			continue
		}
		if strings.HasPrefix(line, "--- a/") {
			// Already set action from /dev/null check if needed
			continue
		}
		if strings.HasPrefix(line, "+++ b/") {
			current.path = strings.TrimPrefix(line, "+++ b/")
			continue
		}
		if strings.HasPrefix(line, "+++ a/") {
			// Deleted file path fallback
			current.path = strings.TrimPrefix(line, "+++ a/")
			continue
		}
		if strings.HasPrefix(line, "new file") {
			current.action = "added"
			continue
		}
		if strings.HasPrefix(line, "deleted file") {
			current.action = "deleted"
			continue
		}

		if strings.HasPrefix(line, "@@") {
			if inHunk {
				current.hunks = append(current.hunks, currentHunk.String())
				currentHunk.Reset()
			}
			inHunk = true
			currentHunk.WriteString(line)
			currentHunk.WriteString("\n")
			continue
		}

		if inHunk {
			currentHunk.WriteString(line)
			currentHunk.WriteString("\n")
		}
	}

	// Flush final hunk and file
	if inHunk && current != nil {
		current.hunks = append(current.hunks, currentHunk.String())
	}
	if current != nil {
		// Handle deleted files where path comes from --- line
		if current.path == "" && current.action == "deleted" {
			// Try to find path from the diff --git line
			for _, line := range lines {
				if strings.HasPrefix(line, "--- a/") {
					current.path = strings.TrimPrefix(line, "--- a/")
					break
				}
			}
		}
		files = append(files, *current)
	}

	return files
}

// SummarizeFile analyzes hunks for a single file and produces a FileSummary.
func (ds *DiffSummarizer) SummarizeFile(path string, hunks []string) *FileSummary {
	fs := &FileSummary{
		Path:       path,
		Action:     "modified",
		KeyChanges: []string{},
	}

	var addedLines []string
	var removedLines []string

	for _, hunk := range hunks {
		for _, line := range strings.Split(hunk, "\n") {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				fs.Additions++
				addedLines = append(addedLines, strings.TrimPrefix(line, "+"))
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				fs.Deletions++
				removedLines = append(removedLines, strings.TrimPrefix(line, "-"))
			}
		}
	}

	// Detect key changes
	fs.KeyChanges = ds.detectKeyChanges(addedLines, removedLines)

	// Generate summary
	fs.Summary = ds.generateFileSummary(path, addedLines, removedLines, fs)

	return fs
}

var funcPattern = regexp.MustCompile(`^\s*func\s+(?:\([^)]+\)\s+)?(\w+)\s*\(`)
var structPattern = regexp.MustCompile(`^\s*type\s+(\w+)\s+struct`)
var interfacePattern = regexp.MustCompile(`^\s*type\s+(\w+)\s+interface`)

// detectKeyChanges identifies significant additions/removals like new functions, types, etc.
func (ds *DiffSummarizer) detectKeyChanges(added, removed []string) []string {
	var changes []string

	newFuncs := ds.extractPatternMatches(added, funcPattern)
	removedFuncs := ds.extractPatternMatches(removed, funcPattern)
	newStructs := ds.extractPatternMatches(added, structPattern)
	removedStructs := ds.extractPatternMatches(removed, structPattern)
	newInterfaces := ds.extractPatternMatches(added, interfacePattern)

	for _, f := range newFuncs {
		changes = append(changes, "new "+f+"()")
	}
	for _, f := range removedFuncs {
		changes = append(changes, "removed "+f+"()")
	}
	for _, s := range newStructs {
		changes = append(changes, "new "+s+" struct")
	}
	for _, s := range removedStructs {
		changes = append(changes, "removed "+s+" struct")
	}
	for _, iface := range newInterfaces {
		changes = append(changes, "new "+iface+" interface")
	}

	return changes
}

// extractPatternMatches finds all first-group matches of pattern in lines.
func (ds *DiffSummarizer) extractPatternMatches(lines []string, pattern *regexp.Regexp) []string {
	seen := make(map[string]bool)
	var results []string
	for _, line := range lines {
		if m := pattern.FindStringSubmatch(line); m != nil {
			name := m[1]
			if !seen[name] {
				seen[name] = true
				results = append(results, name)
			}
		}
	}
	return results
}

// generateFileSummary creates a concise one-line summary of a file change.
func (ds *DiffSummarizer) generateFileSummary(path string, added, removed []string, fs *FileSummary) string {
	if len(added) == 0 && len(removed) == 0 {
		return "No significant changes"
	}

	newFuncs := ds.extractPatternMatches(added, funcPattern)
	newStructs := ds.extractPatternMatches(added, structPattern)
	removedFuncs := ds.extractPatternMatches(removed, funcPattern)

	// Simple heuristics for summary generation
	if len(removed) == 0 && len(added) > 0 {
		if len(newFuncs) > 0 {
			return fmt.Sprintf("Add %s", strings.Join(newFuncs, ", "))
		}
		if len(newStructs) > 0 {
			return fmt.Sprintf("Add %s type", strings.Join(newStructs, ", "))
		}
		return fmt.Sprintf("Add %d lines", len(added))
	}

	if len(added) == 0 && len(removed) > 0 {
		if len(removedFuncs) > 0 {
			return fmt.Sprintf("Remove %s", strings.Join(removedFuncs, ", "))
		}
		return fmt.Sprintf("Remove %d lines", len(removed))
	}

	// Mixed add/remove
	if len(newFuncs) > 0 && len(removedFuncs) > 0 {
		return fmt.Sprintf("Replace %s with %s", strings.Join(removedFuncs, ", "), strings.Join(newFuncs, ", "))
	}
	if len(newFuncs) > 0 {
		return fmt.Sprintf("Add %s", strings.Join(newFuncs, ", "))
	}

	// Check for error handling patterns
	for _, line := range added {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "err !=") || strings.Contains(trimmed, "error") {
			return "Add error handling"
		}
	}

	return fmt.Sprintf("Modify logic (+%d/-%d)", len(added), len(removed))
}

// DetectChangeType classifies the overall change type based on file patterns and content.
func (ds *DiffSummarizer) DetectChangeType(summary *DiffSummary) string {
	if len(summary.Files) == 0 {
		return "refactor"
	}

	testCount := 0
	docCount := 0
	configCount := 0
	newFileCount := 0
	hasErrorHandling := false
	hasFunctionalChange := false

	for _, f := range summary.Files {
		if diffSumIsTestFile(f.Path) {
			testCount++
		}
		if diffSumIsDocFile(f.Path) {
			docCount++
		}
		if diffSumIsConfigFile(f.Path) {
			configCount++
		}
		if f.Action == "added" {
			newFileCount++
		}
		for _, kc := range f.KeyChanges {
			if strings.Contains(kc, "new ") {
				hasFunctionalChange = true
			}
		}
		if strings.Contains(f.Summary, "error") || strings.Contains(f.Summary, "Error") {
			hasErrorHandling = true
		}
	}

	totalFiles := len(summary.Files)

	// All test files
	if testCount == totalFiles {
		return "test"
	}
	// All doc files
	if docCount == totalFiles {
		return "docs"
	}
	// All config files
	if configCount == totalFiles {
		return "config"
	}
	// New files with functional changes
	if newFileCount > 0 && hasFunctionalChange {
		return "feature"
	}
	// Error handling added
	if hasErrorHandling && !hasFunctionalChange {
		return "bugfix"
	}
	// New functions/structs in existing files
	if hasFunctionalChange {
		return "feature"
	}

	return "refactor"
}

// AssessImpact determines the impact level of the changes.
func (ds *DiffSummarizer) AssessImpact(summary *DiffSummary) string {
	if len(summary.Files) == 0 {
		return "low"
	}

	totalFiles := len(summary.Files)
	totalAdditions := 0
	totalDeletions := 0
	hasExportedChanges := false

	for _, f := range summary.Files {
		totalAdditions += f.Additions
		totalDeletions += f.Deletions
		for _, kc := range f.KeyChanges {
			// Exported names start with uppercase
			parts := strings.Fields(kc)
			for _, p := range parts {
				if len(p) > 0 && p[0] >= 'A' && p[0] <= 'Z' {
					hasExportedChanges = true
					break
				}
			}
		}
	}

	totalChanges := totalAdditions + totalDeletions

	// High impact: many files with exported changes or large diffs
	if totalFiles >= 5 && hasExportedChanges {
		return "high"
	}
	if totalChanges > 200 && hasExportedChanges {
		return "high"
	}

	// Medium impact: moderate changes or exported changes in few files
	if hasExportedChanges {
		return "medium"
	}
	if totalFiles >= 3 || totalChanges > 50 {
		return "medium"
	}

	return "low"
}

// GenerateCommitMessage produces a conventional commit message from the summary.
func (ds *DiffSummarizer) GenerateCommitMessage(summary *DiffSummary) string {
	if summary == nil || len(summary.Files) == 0 {
		return "chore: update code"
	}

	prefix := changeTypeToPrefix(summary.ChangeType)
	scope := ""
	if len(summary.AffectedAreas) > 0 {
		scope = fmt.Sprintf("(%s)", summary.AffectedAreas[0])
	}

	description := ds.commitDescription(summary)

	return fmt.Sprintf("%s%s: %s", prefix, scope, description)
}

// changeTypeToPrefix maps a change type to a conventional commit prefix.
func changeTypeToPrefix(changeType string) string {
	switch changeType {
	case "feature":
		return "feat"
	case "bugfix":
		return "fix"
	case "refactor":
		return "refactor"
	case "test":
		return "test"
	case "docs":
		return "docs"
	case "config":
		return "chore"
	default:
		return "chore"
	}
}

// commitDescription generates the description portion of the commit message.
func (ds *DiffSummarizer) commitDescription(summary *DiffSummary) string {
	if summary.OverallSummary != "" {
		// Use the overall summary but lowercase the first letter
		desc := summary.OverallSummary
		if len(desc) > 0 && desc[0] >= 'A' && desc[0] <= 'Z' {
			desc = strings.ToLower(desc[:1]) + desc[1:]
		}
		// Trim to keep it concise
		if len(desc) > 72 {
			desc = desc[:69] + "..."
		}
		return desc
	}

	// Fallback: summarize from file summaries
	if len(summary.Files) == 1 {
		return strings.ToLower(summary.Files[0].Summary)
	}

	return fmt.Sprintf("update %d files", len(summary.Files))
}

// GeneratePRSummary produces a multi-paragraph PR description from the summary.
func (ds *DiffSummarizer) GeneratePRSummary(summary *DiffSummary) string {
	if summary == nil || len(summary.Files) == 0 {
		return "No changes to summarize."
	}

	var sb strings.Builder

	// Title section
	sb.WriteString("## Summary\n\n")
	sb.WriteString(summary.OverallSummary)
	sb.WriteString("\n\n")

	// Change type and impact
	sb.WriteString(fmt.Sprintf("**Type:** %s | **Impact:** %s\n\n", summary.ChangeType, summary.Impact))

	// Files changed
	sb.WriteString("## Changes\n\n")
	for _, f := range summary.Files {
		icon := actionIcon(f.Action)
		sb.WriteString(fmt.Sprintf("- %s `%s` — %s", icon, f.Path, f.Summary))
		if len(f.KeyChanges) > 0 {
			sb.WriteString(fmt.Sprintf(" (%s)", strings.Join(f.KeyChanges, ", ")))
		}
		sb.WriteString("\n")
	}

	// Affected areas
	if len(summary.AffectedAreas) > 0 {
		sb.WriteString("\n## Affected Areas\n\n")
		for _, area := range summary.AffectedAreas {
			sb.WriteString(fmt.Sprintf("- %s\n", area))
		}
	}

	return sb.String()
}

// actionIcon returns a markdown-friendly icon for the file action.
func actionIcon(action string) string {
	switch action {
	case "added":
		return "+"
	case "deleted":
		return "-"
	default:
		return "~"
	}
}

// FormatSummary produces a compact terminal-friendly summary display.
func (ds *DiffSummarizer) FormatSummary(summary *DiffSummary) string {
	if summary == nil || len(summary.Files) == 0 {
		return "Diff Summary:\nNo changes detected.\n"
	}

	var sb strings.Builder

	sb.WriteString("Diff Summary:\n")
	sb.WriteString(fmt.Sprintf("Type: %s | Impact: %s\n", summary.ChangeType, summary.Impact))
	sb.WriteString(strings.Repeat("─", 31))
	sb.WriteString("\n")

	for _, f := range summary.Files {
		prefix := "~"
		switch f.Action {
		case "added":
			prefix = "+"
		case "deleted":
			prefix = "-"
		}

		sb.WriteString(fmt.Sprintf("%s %s", prefix, f.Path))
		if f.Summary != "" {
			sb.WriteString(fmt.Sprintf(" — %s", f.Summary))
		}
		sb.WriteString("\n")

		if len(f.KeyChanges) > 0 {
			sb.WriteString(fmt.Sprintf("  Key: %s\n", strings.Join(f.KeyChanges, ", ")))
		}
	}

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Overall: %s\n", summary.OverallSummary))
	if len(summary.AffectedAreas) > 0 {
		sb.WriteString(fmt.Sprintf("Affected: %s\n", strings.Join(summary.AffectedAreas, ", ")))
	}

	return sb.String()
}

// generateOverallSummary creates a concise overall summary from all file summaries.
func (ds *DiffSummarizer) generateOverallSummary(summary *DiffSummary) string {
	if len(summary.Files) == 0 {
		return "No changes"
	}
	if len(summary.Files) == 1 {
		return summary.Files[0].Summary
	}

	// Collect key themes from file summaries
	var themes []string
	for _, f := range summary.Files {
		if f.Summary != "" && f.Summary != "No significant changes" {
			themes = append(themes, f.Summary)
		}
	}

	if len(themes) == 0 {
		return "Minor changes across multiple files"
	}

	if len(themes) <= 2 {
		return strings.Join(themes, " and ")
	}

	// Summarize: first theme + "with N more changes"
	return fmt.Sprintf("%s with %d additional changes", themes[0], len(themes)-1)
}

// extractAffectedAreas identifies the top-level directories or packages affected.
func (ds *DiffSummarizer) extractAffectedAreas(files []FileSummary) []string {
	seen := make(map[string]bool)
	var areas []string

	for _, f := range files {
		area := extractArea(f.Path)
		if area != "" && !seen[area] {
			seen[area] = true
			areas = append(areas, area)
		}
	}

	return areas
}

// extractArea returns the first meaningful path component as the area.
func extractArea(path string) string {
	// Remove leading src/ if present
	path = strings.TrimPrefix(path, "src/")

	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		// Root-level file, use filename without extension
		base := filepath.Base(path)
		ext := filepath.Ext(base)
		return strings.TrimSuffix(base, ext)
	}

	// Use the first directory component
	parts := strings.Split(filepath.ToSlash(dir), "/")
	if len(parts) > 0 {
		return parts[0]
	}

	return dir
}

// diffSumIsTestFile checks whether a file path looks like a test file.
func diffSumIsTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go") ||
		strings.HasSuffix(path, ".test.ts") ||
		strings.HasSuffix(path, ".test.js") ||
		strings.HasSuffix(path, "_test.py") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/") ||
		strings.HasPrefix(path, "test/") ||
		strings.HasPrefix(path, "tests/") ||
		strings.Contains(path, "__tests__/")
}

// diffSumIsDocFile checks whether a file path looks like a documentation file.
func diffSumIsDocFile(path string) bool {
	return strings.HasSuffix(path, ".md") ||
		strings.HasSuffix(path, ".rst") ||
		strings.HasSuffix(path, ".txt") ||
		strings.Contains(path, "/docs/") ||
		strings.Contains(path, "/doc/")
}

// diffSumIsConfigFile checks whether a file path looks like a configuration file.
func diffSumIsConfigFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".yaml") ||
		strings.HasSuffix(base, ".yml") ||
		strings.HasSuffix(base, ".toml") ||
		strings.HasSuffix(base, ".json") ||
		strings.HasSuffix(base, ".ini") ||
		base == "Makefile" ||
		base == "Dockerfile" ||
		base == ".gitignore" ||
		base == "go.mod" ||
		base == "go.sum"
}
