package engine

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// PRCompressor intelligently compresses large PR diffs to fit within a model's
// context window. Inspired by pr-agent, it prioritizes important source files,
// truncates oversized diffs, and excludes generated/lock files to maximize
// signal density per token spent.
type PRCompressor struct {
	MaxTokens        int
	LanguagePriority map[string]int
	mu               sync.RWMutex
}

// CompressedPR holds the result of compressing a full PR diff into a
// token-budget-aware representation.
type CompressedPR struct {
	Files         []CompressedFile
	TotalTokens   int
	OverflowFiles []string
	Summary       string
}

// CompressedFile represents a single file's diff after compression.
type CompressedFile struct {
	Path      string
	Diff      string
	Tokens    int
	Priority  float64
	Language  string
	Truncated bool
}

// NewPRCompressor creates a PRCompressor with the given token budget.
func NewPRCompressor(maxTokens int) *PRCompressor {
	return &PRCompressor{
		MaxTokens: maxTokens,
		LanguagePriority: map[string]int{
			".go":   10,
			".py":   10,
			".ts":   10,
			".js":   9,
			".rs":   10,
			".java": 9,
			".rb":   9,
			".c":    9,
			".cpp":  9,
			".h":    8,
		},
	}
}

// CompressDiff parses a unified diff, scores files by priority, and packs
// them into the given token budget. Files that don't fit are reported as
// overflow. Large files are truncated to keep first and last hunks.
func (pc *PRCompressor) CompressDiff(fullDiff string, budget int) *CompressedPR {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if budget <= 0 {
		budget = pc.MaxTokens
	}

	// Parse diff into per-file sections
	fileDiffs := parseDiffIntoFiles(fullDiff)

	// Score and build compressed file entries
	scored := make([]CompressedFile, 0, len(fileDiffs))
	for path, diff := range fileDiffs {
		priority := pc.ScoreFile(path)
		// Skip files with zero priority entirely
		if priority == 0.0 {
			continue
		}
		lang := prDetectLanguage(path)
		tokens := EstimateDiffTokens(diff)
		scored = append(scored, CompressedFile{
			Path:     path,
			Diff:     diff,
			Tokens:   tokens,
			Priority: priority,
			Language: lang,
		})
	}

	// Sort by priority descending, then by path for stability
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Priority != scored[j].Priority {
			return scored[i].Priority > scored[j].Priority
		}
		return scored[i].Path < scored[j].Path
	})

	// Pack files into budget
	result := &CompressedPR{
		Files:         make([]CompressedFile, 0),
		OverflowFiles: make([]string, 0),
	}
	remaining := budget

	for i := range scored {
		f := scored[i]
		if f.Tokens <= remaining {
			// File fits entirely
			result.Files = append(result.Files, f)
			remaining -= f.Tokens
			result.TotalTokens += f.Tokens
		} else if remaining > 100 {
			// Try to truncate the file to fit
			truncated := TruncateHunks(f.Diff, remaining)
			truncTokens := EstimateDiffTokens(truncated)
			if truncTokens > 0 && truncTokens <= remaining {
				f.Diff = truncated
				f.Tokens = truncTokens
				f.Truncated = true
				result.Files = append(result.Files, f)
				remaining -= truncTokens
				result.TotalTokens += truncTokens
			} else {
				result.OverflowFiles = append(result.OverflowFiles, f.Path)
			}
		} else {
			result.OverflowFiles = append(result.OverflowFiles, f.Path)
		}
	}

	// Add zero-priority files to overflow for reporting
	for path := range fileDiffs {
		if pc.ScoreFile(path) == 0.0 {
			result.OverflowFiles = append(result.OverflowFiles, path)
		}
	}
	sort.Strings(result.OverflowFiles)

	result.Summary = buildSummary(result, budget)
	return result
}

// ScoreFile assigns a priority score to a file path based on its role.
// Source code gets highest priority, generated/lock files get lowest.
func (pc *PRCompressor) ScoreFile(path string) float64 {
	// Vendor and node_modules are always excluded
	if strings.Contains(path, "vendor/") || strings.Contains(path, "node_modules/") {
		return 0.0
	}

	// Generated/lock files
	if DetectGenerated(path) {
		return 0.1
	}

	// Documentation
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".md" || ext == ".rst" || ext == ".txt" || strings.Contains(path, "docs/") {
		return 0.4
	}

	// Config files
	if ext == ".yaml" || ext == ".yml" || ext == ".toml" || ext == ".json" || ext == ".xml" ||
		ext == ".ini" || ext == ".cfg" {
		// Don't score lock/generated json files (already handled above)
		return 0.5
	}

	// Test files
	if prIsTestFile(path) {
		return 0.7
	}

	// Source code (default high priority)
	sourceExts := map[string]bool{
		".go": true, ".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
		".rs": true, ".java": true, ".rb": true, ".c": true, ".cpp": true, ".h": true,
		".cs": true, ".swift": true, ".kt": true, ".scala": true, ".ex": true, ".exs": true,
		".php": true, ".vue": true, ".svelte": true, ".dart": true, ".zig": true,
	}
	if sourceExts[ext] {
		return 1.0
	}

	// Shell scripts and makefiles
	if ext == ".sh" || ext == ".bash" || strings.Contains(strings.ToLower(filepath.Base(path)), "makefile") {
		return 0.6
	}

	// Default: moderate priority
	return 0.5
}

// TruncateHunks keeps the first and last hunks of a diff and drops the middle,
// inserting a note about omitted hunks.
func TruncateHunks(diff string, maxTokens int) string {
	hunks := splitHunks(diff)
	if len(hunks) <= 2 {
		// Already small enough or just one/two hunks
		result := strings.Join(hunks, "\n")
		if EstimateDiffTokens(result) <= maxTokens {
			return result
		}
		// Single hunk too large: truncate lines
		return truncateLines(result, maxTokens)
	}

	// Keep first and last hunk
	first := hunks[0]
	last := hunks[len(hunks)-1]
	omitted := len(hunks) - 2
	marker := fmt.Sprintf("\n... %d hunks omitted ...\n", omitted)

	result := first + marker + last
	if EstimateDiffTokens(result) <= maxTokens {
		return result
	}

	// Still too large: try just first hunk with marker
	result = first + fmt.Sprintf("\n... %d hunks omitted ...\n", len(hunks)-1)
	if EstimateDiffTokens(result) <= maxTokens {
		return result
	}

	// Truncate first hunk itself
	return truncateLines(first, maxTokens)
}

// DetectGenerated returns true if the path matches known generated/lock file patterns.
func DetectGenerated(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)

	// Exact filename matches
	generatedFiles := []string{
		"go.sum",
		"package-lock.json",
		"yarn.lock",
		"pnpm-lock.yaml",
		"composer.lock",
		"cargo.lock",
		"gemfile.lock",
		"poetry.lock",
		"pipfile.lock",
	}
	for _, gf := range generatedFiles {
		if lower == gf {
			return true
		}
	}

	// Pattern matches
	if strings.HasSuffix(lower, ".pb.go") {
		return true
	}
	if strings.Contains(lower, ".generated.") {
		return true
	}
	if strings.HasSuffix(lower, ".gen.go") {
		return true
	}
	if strings.HasSuffix(lower, ".min.js") {
		return true
	}
	if strings.HasSuffix(lower, ".min.css") {
		return true
	}

	// Directory patterns
	dir := filepath.Dir(path)
	dirParts := strings.Split(dir, "/")
	for _, part := range dirParts {
		switch part {
		case "dist", "build", "generated", "gen", "__generated__":
			return true
		}
	}

	return false
}

// FormatCompressed produces a human-readable summary of the compressed PR diff.
func FormatCompressedPR(pr *CompressedPR) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("PR Diff (compressed to %s tokens):\n", prFormatNumber(pr.TotalTokens)))

	// Included files
	included := make([]string, 0, len(pr.Files))
	truncated := make([]string, 0)
	for _, f := range pr.Files {
		included = append(included, f.Path)
		if f.Truncated {
			truncated = append(truncated, f.Path)
		}
	}

	if len(included) > 0 {
		sb.WriteString(fmt.Sprintf("Included: %d files (%s)\n", len(included), summarizePaths(included)))
	}
	if len(truncated) > 0 {
		sb.WriteString(fmt.Sprintf("Truncated: %d files (large diffs)\n", len(truncated)))
	}
	if len(pr.OverflowFiles) > 0 {
		sb.WriteString(fmt.Sprintf("Excluded: %d files (lock files, generated)\n", len(pr.OverflowFiles)))
	}

	sb.WriteString("\n")

	// Append compressed diff content
	for _, f := range pr.Files {
		sb.WriteString(f.Diff)
		if !strings.HasSuffix(f.Diff, "\n") {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// EstimateDiffTokens provides a rough token count for a diff string.
// Uses the heuristic of ~4 characters per token (common for code).
func EstimateDiffTokens(diff string) int {
	if len(diff) == 0 {
		return 0
	}
	// Approximate: 1 token per 4 characters for code
	// This aligns with typical BPE tokenizer behavior for source code
	tokens := len(diff) / 4
	if tokens == 0 && len(diff) > 0 {
		tokens = 1
	}
	return tokens
}

// --- internal helpers ---

// parseDiffIntoFiles splits a unified diff into per-file sections.
func parseDiffIntoFiles(fullDiff string) map[string]string {
	result := make(map[string]string)
	if fullDiff == "" {
		return result
	}

	lines := strings.Split(fullDiff, "\n")
	var currentFile string
	var currentDiff strings.Builder

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "diff --git") {
			// Save previous file
			if currentFile != "" {
				result[currentFile] = currentDiff.String()
			}
			// Extract file path from "diff --git a/path b/path"
			currentFile = extractFilePath(line)
			currentDiff.Reset()
			currentDiff.WriteString(line)
			currentDiff.WriteString("\n")
		} else {
			if currentFile != "" {
				currentDiff.WriteString(line)
				currentDiff.WriteString("\n")
			}
		}
	}

	// Save last file
	if currentFile != "" {
		result[currentFile] = currentDiff.String()
	}

	return result
}

// extractFilePath extracts the file path from a "diff --git a/path b/path" line.
func extractFilePath(diffLine string) string {
	// Format: "diff --git a/path/to/file b/path/to/file"
	parts := strings.SplitN(diffLine, " b/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	// Fallback: try to extract from a/ prefix
	parts = strings.SplitN(diffLine, " a/", 2)
	if len(parts) == 2 {
		subParts := strings.SplitN(parts[1], " ", 2)
		if len(subParts) > 0 {
			return subParts[0]
		}
	}
	return diffLine
}

// splitHunks splits a file diff into individual hunks (each starting with @@).
func splitHunks(diff string) []string {
	lines := strings.Split(diff, "\n")
	var hunks []string
	var current strings.Builder
	inHeader := true

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			if !inHeader && current.Len() > 0 {
				hunks = append(hunks, strings.TrimRight(current.String(), "\n"))
				current.Reset()
			}
			inHeader = false
		}
		if !inHeader {
			current.WriteString(line)
			current.WriteString("\n")
		} else {
			// Include header in first hunk
			current.WriteString(line)
			current.WriteString("\n")
		}
	}
	if current.Len() > 0 {
		hunks = append(hunks, strings.TrimRight(current.String(), "\n"))
	}

	return hunks
}

// truncateLines truncates a diff to fit within a token budget by keeping
// initial lines and adding a truncation marker.
func truncateLines(diff string, maxTokens int) string {
	lines := strings.Split(diff, "\n")
	maxChars := maxTokens * 4 // reverse the token estimation
	marker := "\n... truncated ...\n"
	markerLen := len(marker)

	var sb strings.Builder
	for _, line := range lines {
		if sb.Len()+len(line)+1+markerLen > maxChars {
			break
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString("... truncated ...")
	return sb.String()
}

// detectLanguage returns the language identifier for a file based on extension.
func prDetectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	languages := map[string]string{
		".go":      "go",
		".py":      "python",
		".ts":      "typescript",
		".tsx":     "typescript",
		".js":      "javascript",
		".jsx":     "javascript",
		".rs":      "rust",
		".java":    "java",
		".rb":      "ruby",
		".c":       "c",
		".cpp":     "cpp",
		".h":       "c",
		".cs":      "csharp",
		".swift":   "swift",
		".kt":      "kotlin",
		".scala":   "scala",
		".ex":      "elixir",
		".exs":     "elixir",
		".php":     "php",
		".vue":     "vue",
		".svelte":  "svelte",
		".dart":    "dart",
		".yaml":    "yaml",
		".yml":     "yaml",
		".toml":    "toml",
		".json":    "json",
		".md":      "markdown",
		".sh":      "shell",
		".bash":    "shell",
		".sql":     "sql",
		".graphql": "graphql",
	}
	if lang, ok := languages[ext]; ok {
		return lang
	}
	return ""
}

// isTestFile determines if a path is a test file.
func prIsTestFile(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)

	// Go test files
	if strings.HasSuffix(lower, "_test.go") {
		return true
	}
	// Python test files
	if strings.HasPrefix(lower, "test_") || strings.HasSuffix(lower, "_test.py") {
		return true
	}
	// JS/TS test files
	if strings.Contains(lower, ".test.") || strings.Contains(lower, ".spec.") {
		return true
	}
	// Test directories
	dir := filepath.Dir(path) + "/"
	if strings.Contains(dir, "/test/") || strings.Contains(dir, "/tests/") ||
		strings.Contains(dir, "/__tests__/") ||
		strings.HasPrefix(dir, "test/") || strings.HasPrefix(dir, "tests/") ||
		strings.HasPrefix(dir, "__tests__/") {
		return true
	}
	return false
}

// buildSummary creates the summary string for a CompressedPR.
func buildSummary(pr *CompressedPR, budget int) string {
	included := len(pr.Files)
	excluded := len(pr.OverflowFiles)
	truncated := 0
	for _, f := range pr.Files {
		if f.Truncated {
			truncated++
		}
	}
	return fmt.Sprintf("%d files included (%d truncated), %d excluded, %d/%d tokens used",
		included, truncated, excluded, pr.TotalTokens, budget)
}

// formatNumber formats an integer with comma separators for readability.
func prFormatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	// Insert commas from the right
	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
		if len(s) > remainder {
			result.WriteString(",")
		}
	}
	for i := remainder; i < len(s); i += 3 {
		if i > remainder {
			result.WriteString(",")
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}

// summarizePaths returns a comma-separated list of paths, truncating if too many.
func summarizePaths(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	maxShow := 5
	if len(paths) <= maxShow {
		return strings.Join(paths, ", ")
	}
	shown := paths[:maxShow]
	return strings.Join(shown, ", ") + fmt.Sprintf(", ... +%d more", len(paths)-maxShow)
}
