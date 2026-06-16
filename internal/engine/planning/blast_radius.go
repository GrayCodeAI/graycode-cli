// Package planning provides execution planning for tool calls.
// This file implements blast radius estimation — predicting the scope
// and risk of a planned change before execution begins.
package planning

import (
	"fmt"
	"path/filepath"
	"strings"
)

// BlastRadius classifies the scope of a planned change.
type BlastRadius int

const (
	// RadiusSmall is 1-3 files: proceed immediately.
	RadiusSmall BlastRadius = iota
	// RadiusMedium is 4-10 files: show summary, confirm.
	RadiusMedium
	// RadiusLarge is 11-25 files: require explicit approval, enable validation.
	RadiusLarge
	// RadiusHuge is 26+ files: suggest breaking into sub-tasks.
	RadiusHuge
)

// String returns a human-readable label for the radius.
func (r BlastRadius) String() string {
	switch r {
	case RadiusSmall:
		return "small"
	case RadiusMedium:
		return "medium"
	case RadiusLarge:
		return "large"
	case RadiusHuge:
		return "huge"
	default:
		return "unknown"
	}
}

// Emoji returns a visual indicator for the radius.
func (r BlastRadius) Emoji() string {
	switch r {
	case RadiusSmall:
		return "LOW"
	case RadiusMedium:
		return "MED"
	case RadiusLarge:
		return "HIGH"
	case RadiusHuge:
		return "CRIT"
	default:
		return "-"
	}
}

// NeedsConfirmation returns true if the change scope requires user approval.
func (r BlastRadius) NeedsConfirmation() bool {
	return r >= RadiusMedium
}

// NeedsValidation returns true if pre-commit validation should be enforced.
func (r BlastRadius) NeedsValidation() bool {
	return r >= RadiusLarge
}

// SuggestsDecomposition returns true if the change should be broken into sub-tasks.
func (r BlastRadius) SuggestsDecomposition() bool {
	return r >= RadiusHuge
}

// BlastRadiusReport contains the full analysis of a planned change's scope.
type BlastRadiusReport struct {
	Radius       BlastRadius
	FileCount    int
	UniqueFiles  []string
	DirsAffected []string
	FileTypes    map[string]int // extension -> count
	HasTests     bool
	HasConfig    bool
	HasDocs      bool
	Message      string // human-readable summary
}

// EstimateBlastRadius analyzes a set of planned tool calls and returns a
// blast radius report. Files are extracted from the Targets field of each
// PlannedCall.
func EstimateBlastRadius(calls []PlannedCall) *BlastRadiusReport {
	seen := make(map[string]bool)
	dirs := make(map[string]bool)
	exts := make(map[string]int)

	for _, call := range calls {
		for _, target := range call.Targets {
			if target == "" {
				continue
			}
			// Normalize path
			clean := filepath.Clean(target)
			if seen[clean] {
				continue
			}
			seen[clean] = true

			// Track directory
			dir := filepath.Dir(clean)
			dirs[dir] = true

			// Track extension
			ext := strings.ToLower(filepath.Ext(clean))
			if ext != "" {
				exts[ext]++
			}
		}
	}

	// Build sorted file list
	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sortStrings(files)

	// Build sorted directory list
	dirList := make([]string, 0, len(dirs))
	for d := range dirs {
		dirList = append(dirList, d)
	}
	sortStrings(dirList)

	count := len(files)
	radius := classifyRadius(count)

	// Detect file categories
	hasTests := false
	hasConfig := false
	hasDocs := false
	for _, f := range files {
		lower := strings.ToLower(f)
		if strings.HasSuffix(lower, "_test.go") || strings.Contains(lower, "test") || strings.Contains(lower, "spec") {
			hasTests = true
		}
		if strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".toml") ||
			strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".env") || strings.Contains(lower, "config") {
			hasConfig = true
		}
		if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".rst") {
			hasDocs = true
		}
	}

	return &BlastRadiusReport{
		Radius:       radius,
		FileCount:    count,
		UniqueFiles:  files,
		DirsAffected: dirList,
		FileTypes:    exts,
		HasTests:     hasTests,
		HasConfig:    hasConfig,
		HasDocs:      hasDocs,
		Message:      formatBlastMessage(radius, count, dirList),
	}
}

// classifyRadius maps a file count to a BlastRadius category.
func classifyRadius(count int) BlastRadius {
	switch {
	case count <= 3:
		return RadiusSmall
	case count <= 10:
		return RadiusMedium
	case count <= 25:
		return RadiusLarge
	default:
		return RadiusHuge
	}
}

// formatBlastMessage creates a human-readable summary of the blast radius.
func formatBlastMessage(radius BlastRadius, count int, dirs []string) string {
	dirSummary := ""
	if len(dirs) > 0 {
		if len(dirs) <= 3 {
			dirSummary = " in " + strings.Join(dirs, ", ")
		} else {
			dirSummary = fmt.Sprintf(" across %d directories", len(dirs))
		}
	}

	switch radius {
	case RadiusSmall:
		return fmt.Sprintf("%s Small change: %d file(s)%s — proceeding", radius.Emoji(), count, dirSummary)
	case RadiusMedium:
		return fmt.Sprintf("%s Medium change: %d file(s)%s — please confirm", radius.Emoji(), count, dirSummary)
	case RadiusLarge:
		return fmt.Sprintf("%s Large change: %d file(s)%s — validation will run", radius.Emoji(), count, dirSummary)
	case RadiusHuge:
		return fmt.Sprintf("%s Huge change: %d file(s)%s — consider breaking into sub-tasks", radius.Emoji(), count, dirSummary)
	default:
		return fmt.Sprintf("Change scope: %d file(s)%s", count, dirSummary)
	}
}

// sortStrings sorts a string slice in place.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
