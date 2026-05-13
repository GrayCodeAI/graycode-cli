package engine

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrorInstance represents a single occurrence of an error.
type ErrorInstance struct {
	Message   string    `json:"message"`
	File      string    `json:"file"`
	Line      int       `json:"line"`
	Timestamp time.Time `json:"timestamp"`
	Context   string    `json:"context"`
}

// ErrorGroup clusters similar errors together to track patterns and resolutions.
type ErrorGroup struct {
	ID         string          `json:"id"`
	Pattern    string          `json:"pattern"`
	Instances  []ErrorInstance `json:"instances"`
	Count      int             `json:"count"`
	FirstSeen  time.Time       `json:"first_seen"`
	LastSeen   time.Time       `json:"last_seen"`
	Resolution string          `json:"resolution"`
	Status     string          `json:"status"` // "active", "resolved", "recurring"
}

// ErrorGrouper clusters similar errors to avoid repeating fix attempts.
type ErrorGrouper struct {
	Groups map[string]*ErrorGroup
	mu     sync.RWMutex
}

// NewErrorGrouper creates a new ErrorGrouper with an initialized groups map.
func NewErrorGrouper() *ErrorGrouper {
	return &ErrorGrouper{
		Groups: make(map[string]*ErrorGroup),
	}
}

// normalization regexes compiled once
var (
	pathPattern     = regexp.MustCompile(`(?:/[a-zA-Z0-9._\-]+)+(?:\.[a-zA-Z]+)?`)
	lineNumPattern  = regexp.MustCompile(`(?:line\s*|:)\d+`)
	numberPattern   = regexp.MustCompile(`\b\d{2,}\b`)
	quotedPattern   = regexp.MustCompile(`"[^"]*"`)
	hexPattern      = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	multiSpaceRegex = regexp.MustCompile(`\s+`)
)

// NormalizeError strips file paths, line numbers, and specific values from an
// error message, keeping only the structural pattern.
func NormalizeError(msg string) string {
	normalized := msg

	// Strip hex addresses
	normalized = hexPattern.ReplaceAllString(normalized, "<addr>")

	// Strip file paths
	normalized = pathPattern.ReplaceAllString(normalized, "<path>")

	// Strip line numbers (e.g., "line 42", ":42")
	normalized = lineNumPattern.ReplaceAllString(normalized, "<line>")

	// Strip quoted strings
	normalized = quotedPattern.ReplaceAllString(normalized, "<str>")

	// Strip remaining multi-digit numbers
	normalized = numberPattern.ReplaceAllString(normalized, "<num>")

	// Collapse whitespace
	normalized = multiSpaceRegex.ReplaceAllString(normalized, " ")

	return strings.TrimSpace(normalized)
}

// groupID generates a deterministic ID from the normalized error message.
func groupID(normalized string) string {
	// Simple hash-like ID from the normalized string
	var h uint32
	for _, c := range normalized {
		h = h*31 + uint32(c)
	}
	return fmt.Sprintf("eg_%08x", h)
}

// Add records an error occurrence, finding or creating the appropriate group.
// It returns the group the error was added to.
func (eg *ErrorGrouper) Add(errorMsg, file string, line int, context string) *ErrorGroup {
	normalized := NormalizeError(errorMsg)
	id := groupID(normalized)
	now := time.Now()

	instance := ErrorInstance{
		Message:   errorMsg,
		File:      file,
		Line:      line,
		Timestamp: now,
		Context:   context,
	}

	eg.mu.Lock()
	defer eg.mu.Unlock()

	group, exists := eg.Groups[id]
	if !exists {
		group = &ErrorGroup{
			ID:        id,
			Pattern:   normalized,
			Instances: []ErrorInstance{instance},
			Count:     1,
			FirstSeen: now,
			LastSeen:  now,
			Status:    "active",
		}
		eg.Groups[id] = group
		return group
	}

	group.Instances = append(group.Instances, instance)
	group.Count++
	group.LastSeen = now

	// If it was resolved but is appearing again, mark as recurring
	if group.Status == "resolved" {
		group.Status = "recurring"
	}

	return group
}

// FindGroup returns the group matching the given error message, or nil if none exists.
func (eg *ErrorGrouper) FindGroup(errorMsg string) *ErrorGroup {
	normalized := NormalizeError(errorMsg)
	id := groupID(normalized)

	eg.mu.RLock()
	defer eg.mu.RUnlock()

	return eg.Groups[id]
}

// MarkResolved marks the group with the given ID as resolved with the provided resolution.
func (eg *ErrorGrouper) MarkResolved(groupID, resolution string) {
	eg.mu.Lock()
	defer eg.mu.Unlock()

	group, exists := eg.Groups[groupID]
	if !exists {
		return
	}
	group.Status = "resolved"
	group.Resolution = resolution
}

// IsKnown reports whether this error pattern has been seen before.
func (eg *ErrorGrouper) IsKnown(errorMsg string) bool {
	normalized := NormalizeError(errorMsg)
	id := groupID(normalized)

	eg.mu.RLock()
	defer eg.mu.RUnlock()

	_, exists := eg.Groups[id]
	return exists
}

// GetResolution returns the resolution for a previously resolved error pattern.
// Returns an empty string if the error is unknown or unresolved.
func (eg *ErrorGrouper) GetResolution(errorMsg string) string {
	normalized := NormalizeError(errorMsg)
	id := groupID(normalized)

	eg.mu.RLock()
	defer eg.mu.RUnlock()

	group, exists := eg.Groups[id]
	if !exists {
		return ""
	}
	return group.Resolution
}

// GetActive returns all groups with "active" or "recurring" status.
func (eg *ErrorGrouper) GetActive() []*ErrorGroup {
	eg.mu.RLock()
	defer eg.mu.RUnlock()

	var active []*ErrorGroup
	for _, group := range eg.Groups {
		if group.Status == "active" || group.Status == "recurring" {
			active = append(active, group)
		}
	}

	// Sort by last seen, most recent first
	sort.Slice(active, func(i, j int) bool {
		return active[i].LastSeen.After(active[j].LastSeen)
	})

	return active
}

// FormatGroups returns a human-readable summary of all error groups.
func (eg *ErrorGrouper) FormatGroups() string {
	eg.mu.RLock()
	defer eg.mu.RUnlock()

	if len(eg.Groups) == 0 {
		return "Error Groups (0 active):\n─────────────────────────\nNo error groups recorded."
	}

	// Separate active/recurring from resolved
	var activeCount int
	for _, g := range eg.Groups {
		if g.Status == "active" || g.Status == "recurring" {
			activeCount++
		}
	}

	// Collect all groups sorted by last seen
	groups := make([]*ErrorGroup, 0, len(eg.Groups))
	for _, g := range eg.Groups {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].LastSeen.After(groups[j].LastSeen)
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Error Groups (%d active):\n", activeCount))
	sb.WriteString("─────────────────────────\n")

	for i, g := range groups {
		ago := time.Since(g.LastSeen).Truncate(time.Second)
		agoStr := fmtGroupDuration(ago)

		sb.WriteString(fmt.Sprintf("%d. %q (%d instances, last: %s)\n",
			i+1, g.Pattern, g.Count, agoStr))
		sb.WriteString(fmt.Sprintf("   Status: %s\n", g.Status))

		if g.Status == "resolved" || g.Status == "recurring" {
			if g.Resolution != "" {
				sb.WriteString(fmt.Sprintf("   Resolution: %s\n", g.Resolution))
			}
		}

		// List unique files
		files := uniqueFiles(g.Instances)
		if len(files) > 0 {
			sb.WriteString(fmt.Sprintf("   Files: %s\n", strings.Join(files, ", ")))
		}

		if i < len(groups)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// Prune removes resolved groups older than maxAge.
func (eg *ErrorGrouper) Prune(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)

	eg.mu.Lock()
	defer eg.mu.Unlock()

	for id, group := range eg.Groups {
		if group.Status == "resolved" && group.LastSeen.Before(cutoff) {
			delete(eg.Groups, id)
		}
	}
}

// fmtGroupDuration produces a human-friendly duration string like "2m ago" or "1h ago".
func fmtGroupDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// uniqueFiles extracts distinct file names from error instances.
func uniqueFiles(instances []ErrorInstance) []string {
	seen := make(map[string]struct{})
	var files []string
	for _, inst := range instances {
		if inst.File == "" {
			continue
		}
		if _, ok := seen[inst.File]; !ok {
			seen[inst.File] = struct{}{}
			files = append(files, inst.File)
		}
	}
	return files
}
