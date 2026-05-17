package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/GrayCodeAI/eyrie/client"
)

// FileTracker maintains a cumulative record of files read and modified
// across the session lifetime, persisting through compactions.
type FileTracker struct {
	ReadFiles     map[string]int // path -> count of reads
	ModifiedFiles map[string]int // path -> count of modifications
}

// NewFileTracker creates a new FileTracker with initialized maps.
func NewFileTracker() *FileTracker {
	return &FileTracker{
		ReadFiles:     make(map[string]int),
		ModifiedFiles: make(map[string]int),
	}
}

// RecordRead notes that a file was read.
func (ft *FileTracker) RecordRead(path string) {
	if path == "" {
		return
	}
	ft.ReadFiles[path]++
}

// RecordModified notes that a file was modified.
func (ft *FileTracker) RecordModified(path string) {
	if path == "" {
		return
	}
	ft.ModifiedFiles[path]++
}

// ExtractFromMessages scans messages for tool calls and extracts file paths.
// Looks at Read tool calls for reads, Write/Edit for modifications.
func (ft *FileTracker) ExtractFromMessages(messages []client.EyrieMessage) {
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		for _, tc := range msg.ToolUse {
			cn := canonicalToolName(tc.Name)
			p, ok := pathArgument(tc.Arguments)
			if !ok {
				continue
			}
			switch cn {
			case "Read":
				ft.ReadFiles[p]++
			case "Write", "Edit":
				ft.ModifiedFiles[p]++
			}
		}
	}
}

// FormatForSummary returns a text block suitable for injection into compaction summaries.
// Format:
// <tracked-files>
// Read: path1.go (3x), path2.go (1x)
// Modified: path3.go (2x), path4.go (1x)
// </tracked-files>
func (ft *FileTracker) FormatForSummary() string {
	if len(ft.ReadFiles) == 0 && len(ft.ModifiedFiles) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<tracked-files>\n")

	if len(ft.ReadFiles) > 0 {
		sb.WriteString("Read: ")
		sb.WriteString(formatPathCounts(ft.ReadFiles))
		sb.WriteString("\n")
	}

	if len(ft.ModifiedFiles) > 0 {
		sb.WriteString("Modified: ")
		sb.WriteString(formatPathCounts(ft.ModifiedFiles))
		sb.WriteString("\n")
	}

	sb.WriteString("</tracked-files>")
	return sb.String()
}

// ParseFromSummary extracts previously tracked files from a compaction summary
// containing <tracked-files> blocks, merging with current state.
func (ft *FileTracker) ParseFromSummary(summary string) {
	start := strings.Index(summary, "<tracked-files>")
	end := strings.Index(summary, "</tracked-files>")
	if start < 0 || end <= start {
		return
	}

	block := summary[start+len("<tracked-files>") : end]
	lines := strings.Split(strings.TrimSpace(block), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Read: ") {
			parsePathLine(line[len("Read: "):], ft.ReadFiles)
		} else if strings.HasPrefix(line, "Modified: ") {
			parsePathLine(line[len("Modified: "):], ft.ModifiedFiles)
		}
	}
}

// Merge combines another FileTracker's data into this one.
func (ft *FileTracker) Merge(other *FileTracker) {
	if other == nil {
		return
	}
	for path, count := range other.ReadFiles {
		ft.ReadFiles[path] += count
	}
	for path, count := range other.ModifiedFiles {
		ft.ModifiedFiles[path] += count
	}
}

// formatPathCounts formats a map of path->count into "path1.go (3x), path2.go (1x)" style.
func formatPathCounts(m map[string]int) string {
	// Sort paths for deterministic output
	paths := make([]string, 0, len(m))
	for p := range m {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		parts = append(parts, fmt.Sprintf("%s (%dx)", p, m[p]))
	}
	return strings.Join(parts, ", ")
}

// parsePathLine parses "path1.go (3x), path2.go (1x)" into the target map.
func parsePathLine(line string, target map[string]int) {
	entries := strings.Split(line, ", ")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Parse "path (Nx)" format
		parenIdx := strings.LastIndex(entry, " (")
		if parenIdx < 0 {
			// No count, treat as single occurrence
			target[entry]++
			continue
		}

		path := entry[:parenIdx]
		countStr := entry[parenIdx+2:]
		countStr = strings.TrimSuffix(countStr, ")")
		countStr = strings.TrimSuffix(countStr, "x")

		count, err := strconv.Atoi(countStr)
		if err != nil {
			count = 1
		}
		target[path] += count
	}
}
