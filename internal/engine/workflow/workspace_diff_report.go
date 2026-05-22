package workflow

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WorkspaceDiffReport contains the full report of all workspace changes made during a session.
type WorkspaceDiffReport struct {
	Files           []FileDiffReport
	TotalAdditions  int
	TotalDeletions  int
	FilesAdded      int
	FilesModified   int
	FilesDeleted    int
	SessionDuration time.Duration
	GeneratedAt     time.Time
}

// FileDiffReport describes the diff status and summary for a single file.
type FileDiffReport struct {
	Path       string
	Status     string // "added", "modified", "deleted"
	Additions  int
	Deletions  int
	Summary    string
	KeyChanges []string
}

// DiffReporter generates comprehensive diff reports for a workspace.
type DiffReporter struct {
	ProjectDir string
	mu         sync.Mutex
}

// NewDiffReporter creates a new DiffReporter for the given project directory.
func NewDiffReporter(projectDir string) *DiffReporter {
	return &DiffReporter{
		ProjectDir: projectDir,
	}
}

// GenerateReport creates a WorkspaceDiffReport from a map of modified file paths to their diff content.
// The modifiedFiles map keys are file paths and values are unified diff content for each file.
func (dr *DiffReporter) GenerateReport(modifiedFiles map[string]string) *WorkspaceDiffReport {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	report := &WorkspaceDiffReport{
		Files:       []FileDiffReport{},
		GeneratedAt: time.Now(),
	}

	// Sort file paths for deterministic output
	paths := make([]string, 0, len(modifiedFiles))
	for p := range modifiedFiles {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, path := range paths {
		diff := modifiedFiles[path]
		fileReport := dr.analyzeFileDiff(path, diff)
		report.Files = append(report.Files, fileReport)

		report.TotalAdditions += fileReport.Additions
		report.TotalDeletions += fileReport.Deletions

		switch fileReport.Status {
		case "added":
			report.FilesAdded++
		case "modified":
			report.FilesModified++
		case "deleted":
			report.FilesDeleted++
		}
	}

	return report
}

// GenerateFromGit builds a WorkspaceDiffReport by running git diff commands in the project directory.
func (dr *DiffReporter) GenerateFromGit() (*WorkspaceDiffReport, error) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	report := &WorkspaceDiffReport{
		Files:       []FileDiffReport{},
		GeneratedAt: time.Now(),
	}

	// Get diff stat for overview
	statOut, err := dr.runGit("diff", "--stat", "HEAD")
	if err != nil {
		// Try without HEAD (for initial commits or unstaged changes)
		statOut, err = dr.runGit("diff", "--stat")
		if err != nil {
			return nil, fmt.Errorf("git diff --stat failed: %w", err)
		}
	}

	// Get full diff for detailed analysis
	diffOut, err := dr.runGit("diff", "HEAD")
	if err != nil {
		diffOut, err = dr.runGit("diff")
		if err != nil {
			return nil, fmt.Errorf("git diff failed: %w", err)
		}
	}

	// Also check for staged changes
	stagedDiff, _ := dr.runGit("diff", "--cached")
	if stagedDiff != "" {
		if diffOut != "" {
			diffOut = diffOut + "\n" + stagedDiff
		} else {
			diffOut = stagedDiff
		}
	}

	// Also check for untracked files
	untrackedOut, _ := dr.runGit("ls-files", "--others", "--exclude-standard")

	// Parse the stat output for file-level stats
	if statOut != "" {
		dr.parseStatOutput(report, statOut)
	}

	// Parse the full diff for detailed file reports
	if diffOut != "" {
		dr.parseDiffOutput(report, diffOut)
	}

	// Add untracked files as new
	if untrackedOut != "" {
		dr.addUntrackedFiles(report, untrackedOut)
	}

	// Recalculate totals from individual file reports
	report.TotalAdditions = 0
	report.TotalDeletions = 0
	report.FilesAdded = 0
	report.FilesModified = 0
	report.FilesDeleted = 0

	for _, f := range report.Files {
		report.TotalAdditions += f.Additions
		report.TotalDeletions += f.Deletions
		switch f.Status {
		case "added":
			report.FilesAdded++
		case "modified":
			report.FilesModified++
		case "deleted":
			report.FilesDeleted++
		}
	}

	return report, nil
}

// FormatAsMarkdown renders the report as a markdown document suitable for PR descriptions.
func FormatAsMarkdown(report *WorkspaceDiffReport) string {
	if report == nil || len(report.Files) == 0 {
		return "## Session Changes\n\nNo changes detected.\n"
	}

	var sb strings.Builder

	sb.WriteString("## Session Changes\n\n")

	// Summary line
	var parts []string
	if report.FilesModified > 0 {
		parts = append(parts, fmt.Sprintf("%d files modified", report.FilesModified))
	}
	if report.FilesAdded > 0 {
		parts = append(parts, fmt.Sprintf("%d added", report.FilesAdded))
	}
	if report.FilesDeleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", report.FilesDeleted))
	}

	summaryLine := strings.Join(parts, ", ")
	sb.WriteString(fmt.Sprintf("**Summary:** %s (+%d, -%d)\n", summaryLine, report.TotalAdditions, report.TotalDeletions))

	// Duration if available
	if report.SessionDuration > 0 {
		sb.WriteString(fmt.Sprintf("**Duration:** %s\n", wdrFormatDuration(report.SessionDuration)))
	}

	sb.WriteString("\n### Files Changed\n\n")
	sb.WriteString("| File | Status | Changes | Summary |\n")
	sb.WriteString("|------|--------|---------|--------|\n")

	for _, f := range report.Files {
		changes := formatChangeCounts(f.Additions, f.Deletions)
		summary := f.Summary
		if summary == "" {
			summary = "-"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", f.Path, f.Status, changes, summary))
	}

	// Key changes section
	var allKeyChanges []string
	for _, f := range report.Files {
		allKeyChanges = append(allKeyChanges, f.KeyChanges...)
	}

	if len(allKeyChanges) > 0 {
		sb.WriteString("\n### Key Changes\n")
		for _, kc := range allKeyChanges {
			sb.WriteString(fmt.Sprintf("- %s\n", kc))
		}
	}

	return sb.String()
}

// FormatAsTerminal renders the report with ANSI color codes for terminal display.
func FormatAsTerminal(report *WorkspaceDiffReport) string {
	if report == nil || len(report.Files) == 0 {
		return "\033[1mSession Changes\033[0m\nNo changes detected.\n"
	}

	const (
		bold    = "\033[1m"
		red     = "\033[31m"
		green   = "\033[32m"
		yellow  = "\033[33m"
		cyan    = "\033[36m"
		reset   = "\033[0m"
		dimGray = "\033[2m"
	)

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%sSession Changes%s\n", bold, reset))
	sb.WriteString(fmt.Sprintf("%s%s%s\n\n", dimGray, strings.Repeat("─", 50), reset))

	// Summary
	sb.WriteString(fmt.Sprintf("%s+%d%s / %s-%d%s across %d file(s)\n",
		green, report.TotalAdditions, reset,
		red, report.TotalDeletions, reset,
		len(report.Files)))

	if report.SessionDuration > 0 {
		sb.WriteString(fmt.Sprintf("%sDuration:%s %s\n", dimGray, reset, wdrFormatDuration(report.SessionDuration)))
	}

	sb.WriteString("\n")

	// File list with change bars
	maxPathLen := 0
	for _, f := range report.Files {
		if len(f.Path) > maxPathLen {
			maxPathLen = len(f.Path)
		}
	}
	if maxPathLen > 50 {
		maxPathLen = 50
	}

	for _, f := range report.Files {
		var statusColor string
		var statusIcon string
		switch f.Status {
		case "added":
			statusColor = green
			statusIcon = "+"
		case "deleted":
			statusColor = red
			statusIcon = "-"
		default:
			statusColor = yellow
			statusIcon = "~"
		}

		displayPath := f.Path
		if len(displayPath) > 50 {
			displayPath = "..." + displayPath[len(displayPath)-47:]
		}

		// Change bar visualization
		bar := renderChangeBar(f.Additions, f.Deletions, 20)

		sb.WriteString(fmt.Sprintf("  %s%s%s %-*s %s %s%s%s\n",
			statusColor, statusIcon, reset,
			maxPathLen, displayPath,
			bar,
			dimGray, f.Summary, reset))
	}

	// Key changes
	var allKeyChanges []string
	for _, f := range report.Files {
		allKeyChanges = append(allKeyChanges, f.KeyChanges...)
	}
	if len(allKeyChanges) > 0 {
		sb.WriteString(fmt.Sprintf("\n%sKey Changes:%s\n", cyan, reset))
		for _, kc := range allKeyChanges {
			sb.WriteString(fmt.Sprintf("  %s•%s %s\n", cyan, reset, kc))
		}
	}

	return sb.String()
}

// FormatForCommit renders the report in a compact format suitable for a commit message body.
func FormatForCommit(report *WorkspaceDiffReport) string {
	if report == nil || len(report.Files) == 0 {
		return ""
	}

	var sb strings.Builder

	// First line: compact summary
	totalFiles := len(report.Files)
	sb.WriteString(fmt.Sprintf("Changes: %d file(s) (+%d/-%d)\n\n", totalFiles, report.TotalAdditions, report.TotalDeletions))

	// File list with status indicators
	for _, f := range report.Files {
		prefix := "M"
		switch f.Status {
		case "added":
			prefix = "A"
		case "deleted":
			prefix = "D"
		}
		line := fmt.Sprintf("  %s %s", prefix, f.Path)
		if f.Summary != "" {
			line += " - " + f.Summary
		}
		sb.WriteString(line + "\n")
	}

	// Key changes as bullet points
	var allKeyChanges []string
	for _, f := range report.Files {
		allKeyChanges = append(allKeyChanges, f.KeyChanges...)
	}
	if len(allKeyChanges) > 0 {
		sb.WriteString("\nHighlights:\n")
		for _, kc := range allKeyChanges {
			sb.WriteString(fmt.Sprintf("- %s\n", kc))
		}
	}

	return sb.String()
}

// CompareReports shows the differences between two report snapshots.
func CompareReports(before, after *WorkspaceDiffReport) string {
	if before == nil && after == nil {
		return "No reports to compare."
	}
	if before == nil {
		return fmt.Sprintf("New session: %d file(s) changed (+%d/-%d)",
			len(after.Files), after.TotalAdditions, after.TotalDeletions)
	}
	if after == nil {
		return "Session ended with no final report."
	}

	var sb strings.Builder

	sb.WriteString("## Report Comparison\n\n")

	// Stats comparison
	addDelta := after.TotalAdditions - before.TotalAdditions
	delDelta := after.TotalDeletions - before.TotalDeletions
	fileDelta := len(after.Files) - len(before.Files)

	sb.WriteString(fmt.Sprintf("Files: %d -> %d (%s)\n", len(before.Files), len(after.Files), formatDelta(fileDelta)))
	sb.WriteString(fmt.Sprintf("Additions: %d -> %d (%s)\n", before.TotalAdditions, after.TotalAdditions, formatDelta(addDelta)))
	sb.WriteString(fmt.Sprintf("Deletions: %d -> %d (%s)\n", before.TotalDeletions, after.TotalDeletions, formatDelta(delDelta)))

	// Find new files in after that are not in before
	beforePaths := make(map[string]FileDiffReport)
	for _, f := range before.Files {
		beforePaths[f.Path] = f
	}

	afterPaths := make(map[string]FileDiffReport)
	for _, f := range after.Files {
		afterPaths[f.Path] = f
	}

	var newFiles []string
	var removedFiles []string
	var changedFiles []string

	for _, f := range after.Files {
		if _, exists := beforePaths[f.Path]; !exists {
			newFiles = append(newFiles, f.Path)
		} else {
			bf := beforePaths[f.Path]
			if bf.Additions != f.Additions || bf.Deletions != f.Deletions || bf.Status != f.Status {
				changedFiles = append(changedFiles, f.Path)
			}
		}
	}

	for _, f := range before.Files {
		if _, exists := afterPaths[f.Path]; !exists {
			removedFiles = append(removedFiles, f.Path)
		}
	}

	if len(newFiles) > 0 {
		sb.WriteString("\nNew files since last report:\n")
		for _, p := range newFiles {
			sb.WriteString(fmt.Sprintf("  + %s\n", p))
		}
	}

	if len(removedFiles) > 0 {
		sb.WriteString("\nFiles no longer changed:\n")
		for _, p := range removedFiles {
			sb.WriteString(fmt.Sprintf("  - %s\n", p))
		}
	}

	if len(changedFiles) > 0 {
		sb.WriteString("\nFiles with updated diffs:\n")
		for _, p := range changedFiles {
			bf := beforePaths[p]
			af := afterPaths[p]
			sb.WriteString(fmt.Sprintf("  ~ %s (+%d/-%d -> +%d/-%d)\n", p, bf.Additions, bf.Deletions, af.Additions, af.Deletions))
		}
	}

	return sb.String()
}

// analyzeFileDiff parses a single file's unified diff content and produces a FileDiffReport.
func (dr *DiffReporter) analyzeFileDiff(path, diff string) FileDiffReport {
	report := FileDiffReport{
		Path:       path,
		Status:     "modified",
		KeyChanges: []string{},
	}

	if diff == "" {
		report.Summary = "No changes"
		return report
	}

	lines := strings.Split(diff, "\n")

	var addedLines []string
	var removedLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "--- /dev/null") {
			report.Status = "added"
			continue
		}
		if strings.HasPrefix(line, "+++ /dev/null") {
			report.Status = "deleted"
			continue
		}
		if strings.HasPrefix(line, "new file") {
			report.Status = "added"
			continue
		}
		if strings.HasPrefix(line, "deleted file") {
			report.Status = "deleted"
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			report.Additions++
			addedLines = append(addedLines, strings.TrimPrefix(line, "+"))
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			report.Deletions++
			removedLines = append(removedLines, strings.TrimPrefix(line, "-"))
		}
	}

	// Detect key changes
	report.KeyChanges = detectReportKeyChanges(addedLines, removedLines)

	// Generate summary
	report.Summary = generateReportSummary(path, addedLines, removedLines)

	return report
}

// parseDiffOutput parses a full git diff output and populates the report.
func (dr *DiffReporter) parseDiffOutput(report *WorkspaceDiffReport, diffOutput string) {
	// Split into per-file diffs
	fileDiffs := splitIntoDiffs(diffOutput)

	// Build a lookup of existing files in the report (from stat parsing)
	existingFiles := make(map[string]int)
	for i, f := range report.Files {
		existingFiles[f.Path] = i
	}

	for path, diff := range fileDiffs {
		fileReport := dr.analyzeFileDiff(path, diff)

		if idx, exists := existingFiles[path]; exists {
			// Update existing entry with detailed info
			report.Files[idx] = fileReport
		} else {
			report.Files = append(report.Files, fileReport)
		}
	}
}

// parseStatOutput parses git diff --stat output to populate initial file entries.
func (dr *DiffReporter) parseStatOutput(report *WorkspaceDiffReport, statOutput string) {
	lines := strings.Split(strings.TrimSpace(statOutput), "\n")

	// The last line of stat output is the summary, skip it
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip summary line (e.g., "3 files changed, 10 insertions(+), 5 deletions(-)")
		if strings.Contains(line, "files changed") || strings.Contains(line, "file changed") {
			continue
		}

		// Format: "path/to/file | 5 ++-"
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}

		filePath := strings.TrimSpace(parts[0])
		if filePath == "" {
			continue
		}

		// Parse the stat section for additions/deletions count
		statPart := strings.TrimSpace(parts[1])
		adds, dels := parseStatCounts(statPart)

		fileReport := FileDiffReport{
			Path:       filePath,
			Status:     "modified",
			Additions:  adds,
			Deletions:  dels,
			KeyChanges: []string{},
		}

		report.Files = append(report.Files, fileReport)
	}
}

// addUntrackedFiles adds untracked files to the report as new additions.
func (dr *DiffReporter) addUntrackedFiles(report *WorkspaceDiffReport, untrackedOutput string) {
	lines := strings.Split(strings.TrimSpace(untrackedOutput), "\n")

	// Check existing paths in the report
	existingPaths := make(map[string]bool)
	for _, f := range report.Files {
		existingPaths[f.Path] = true
	}

	for _, line := range lines {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		if existingPaths[path] {
			continue
		}

		report.Files = append(report.Files, FileDiffReport{
			Path:       path,
			Status:     "added",
			Summary:    "New untracked file",
			KeyChanges: []string{},
		})
	}
}

// runGit executes a git command in the project directory and returns output.
func (dr *DiffReporter) runGit(args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dr.ProjectDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// splitIntoDiffs splits a full unified diff into per-file sections.
func splitIntoDiffs(fullDiff string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(fullDiff, "\n")

	var currentPath string
	var currentDiff strings.Builder
	inFile := false

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			// Save previous file diff
			if inFile && currentPath != "" {
				result[currentPath] = currentDiff.String()
			}
			currentDiff.Reset()
			currentPath = ""
			inFile = true
			currentDiff.WriteString(line)
			currentDiff.WriteString("\n")
			continue
		}

		if inFile {
			currentDiff.WriteString(line)
			currentDiff.WriteString("\n")

			// Extract path from +++ line
			if strings.HasPrefix(line, "+++ b/") {
				currentPath = strings.TrimPrefix(line, "+++ b/")
			} else if strings.HasPrefix(line, "--- a/") && currentPath == "" {
				// For deleted files, path comes from --- line
				currentPath = strings.TrimPrefix(line, "--- a/")
			}
		}
	}

	// Flush last file
	if inFile && currentPath != "" {
		result[currentPath] = currentDiff.String()
	}

	return result
}

// parseStatCounts extracts addition and deletion counts from a stat line segment like "5 ++--".
func parseStatCounts(stat string) (int, int) {
	parts := strings.Fields(stat)
	if len(parts) == 0 {
		return 0, 0
	}

	// First field is the total number
	total, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0
	}

	// If there's a second field with +/- characters
	if len(parts) >= 2 {
		bar := parts[1]
		adds := strings.Count(bar, "+")
		dels := strings.Count(bar, "-")
		totalChars := adds + dels
		if totalChars > 0 {
			return total * adds / totalChars, total * dels / totalChars
		}
	}

	return total, 0
}

// detectReportKeyChanges identifies significant changes from added and removed lines.
func detectReportKeyChanges(added, removed []string) []string {
	var changes []string

	reportFuncPattern := regexp.MustCompile(`^\s*func\s+(?:\([^)]+\)\s+)?(\w+)\s*\(`)
	reportStructPattern := regexp.MustCompile(`^\s*type\s+(\w+)\s+struct`)
	reportInterfacePattern := regexp.MustCompile(`^\s*type\s+(\w+)\s+interface`)

	newFuncs := extractReportMatches(added, reportFuncPattern)
	removedFuncs := extractReportMatches(removed, reportFuncPattern)
	newStructs := extractReportMatches(added, reportStructPattern)
	removedStructs := extractReportMatches(removed, reportStructPattern)
	newInterfaces := extractReportMatches(added, reportInterfacePattern)

	for _, f := range newFuncs {
		changes = append(changes, fmt.Sprintf("Added %s()", f))
	}
	for _, f := range removedFuncs {
		changes = append(changes, fmt.Sprintf("Removed %s()", f))
	}
	for _, s := range newStructs {
		changes = append(changes, fmt.Sprintf("Added %s struct", s))
	}
	for _, s := range removedStructs {
		changes = append(changes, fmt.Sprintf("Removed %s struct", s))
	}
	for _, iface := range newInterfaces {
		changes = append(changes, fmt.Sprintf("Added %s interface", iface))
	}

	return changes
}

// extractReportMatches finds first-group regex matches in a set of lines.
func extractReportMatches(lines []string, pattern *regexp.Regexp) []string {
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

// generateReportSummary creates a concise summary for a file's changes.
func generateReportSummary(path string, added, removed []string) string {
	if len(added) == 0 && len(removed) == 0 {
		return "No significant changes"
	}

	funcPattern := regexp.MustCompile(`^\s*func\s+(?:\([^)]+\)\s+)?(\w+)\s*\(`)
	structPattern := regexp.MustCompile(`^\s*type\s+(\w+)\s+struct`)

	newFuncs := extractReportMatches(added, funcPattern)
	newStructs := extractReportMatches(added, structPattern)
	removedFuncs := extractReportMatches(removed, funcPattern)

	ext := filepath.Ext(path)

	// Pure addition
	if len(removed) == 0 {
		if len(newFuncs) > 0 {
			return fmt.Sprintf("Added %s", strings.Join(truncateSlice(newFuncs, 3), ", "))
		}
		if len(newStructs) > 0 {
			return fmt.Sprintf("Added %s type", strings.Join(truncateSlice(newStructs, 3), ", "))
		}
		if isTestFileExt(ext) {
			return fmt.Sprintf("Added %d lines of tests", len(added))
		}
		return fmt.Sprintf("Added %d lines", len(added))
	}

	// Pure deletion
	if len(added) == 0 {
		if len(removedFuncs) > 0 {
			return fmt.Sprintf("Removed %s", strings.Join(truncateSlice(removedFuncs, 3), ", "))
		}
		return fmt.Sprintf("Removed %d lines", len(removed))
	}

	// Mixed changes
	if len(newFuncs) > 0 && len(removedFuncs) > 0 {
		return fmt.Sprintf("Replaced %s with %s",
			strings.Join(truncateSlice(removedFuncs, 2), ", "),
			strings.Join(truncateSlice(newFuncs, 2), ", "))
	}
	if len(newFuncs) > 0 {
		return fmt.Sprintf("Added %s", strings.Join(truncateSlice(newFuncs, 3), ", "))
	}
	if len(newStructs) > 0 {
		return fmt.Sprintf("Added %s type", strings.Join(truncateSlice(newStructs, 3), ", "))
	}

	return fmt.Sprintf("Modified (+%d/-%d)", len(added), len(removed))
}

// renderChangeBar creates a terminal-friendly bar showing additions and deletions.
func renderChangeBar(additions, deletions, width int) string {
	total := additions + deletions
	if total == 0 {
		return strings.Repeat(" ", width)
	}

	addWidth := 0
	delWidth := 0
	if total > 0 {
		addWidth = (additions * width) / total
		delWidth = (deletions * width) / total
		// Ensure at least 1 char for non-zero values
		if additions > 0 && addWidth == 0 {
			addWidth = 1
		}
		if deletions > 0 && delWidth == 0 {
			delWidth = 1
		}
		// Clamp total to width
		if addWidth+delWidth > width {
			if addWidth > delWidth {
				addWidth = width - delWidth
			} else {
				delWidth = width - addWidth
			}
		}
	}

	bar := "\033[32m" + strings.Repeat("+", addWidth) + "\033[31m" + strings.Repeat("-", delWidth) + "\033[0m"
	padding := width - addWidth - delWidth
	if padding > 0 {
		bar += strings.Repeat(" ", padding)
	}

	return bar
}

// wdrFormatDuration formats a time.Duration in a human-friendly way.
func wdrFormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if d < time.Hour {
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	hours := int(d.Hours())
	minutes = minutes % 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// formatChangeCounts produces a compact "+N, -M" string for additions and deletions.
func formatChangeCounts(additions, deletions int) string {
	if additions > 0 && deletions > 0 {
		return fmt.Sprintf("+%d, -%d", additions, deletions)
	}
	if additions > 0 {
		return fmt.Sprintf("+%d", additions)
	}
	if deletions > 0 {
		return fmt.Sprintf("-%d", deletions)
	}
	return "0"
}

// formatDelta formats an integer delta with +/- prefix.
func formatDelta(delta int) string {
	if delta > 0 {
		return fmt.Sprintf("+%d", delta)
	}
	if delta < 0 {
		return fmt.Sprintf("%d", delta)
	}
	return "0"
}

// truncateSlice returns at most n elements from a string slice.
func truncateSlice(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// isTestFileExt checks if a file extension indicates a test file.
func isTestFileExt(ext string) bool {
	return ext == ".go" || ext == ".ts" || ext == ".js" || ext == ".py"
}
