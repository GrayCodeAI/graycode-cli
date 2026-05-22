package errs

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type ErrorInstance struct {
	Message   string    `json:"message"`
	File      string    `json:"file"`
	Line      int       `json:"line"`
	Timestamp time.Time `json:"timestamp"`
	Context   string    `json:"context"`
}

type ErrorGroup struct {
	ID         string          `json:"id"`
	Pattern    string          `json:"pattern"`
	Instances  []ErrorInstance `json:"instances"`
	Count      int             `json:"count"`
	FirstSeen  time.Time       `json:"first_seen"`
	LastSeen   time.Time       `json:"last_seen"`
	Resolution string          `json:"resolution"`
	Status     string          `json:"status"`
}

type ErrorGrouper struct {
	Groups map[string]*ErrorGroup
	mu     sync.RWMutex
}

func NewErrorGrouper() *ErrorGrouper {
	return &ErrorGrouper{
		Groups: make(map[string]*ErrorGroup),
	}
}

var (
	pathPattern        = regexp.MustCompile(`(?:/[a-zA-Z0-9._\-]+)+(?:\.[a-zA-Z]+)?`)
	errFileLinePattern = regexp.MustCompile(`\b[a-zA-Z0-9_\-]+\.[a-zA-Z]{1,4}:\d+:?`)
	lineNumPattern     = regexp.MustCompile(`(?:line\s*|:)\d+:?`)
	numberPattern      = regexp.MustCompile(`\b\d{2,}`)
	quotedPattern      = regexp.MustCompile(`"[^"]*"`)
	hexPattern         = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	multiSpaceRegex    = regexp.MustCompile(`\s+`)
)

func NormalizeError(msg string) string {
	normalized := msg

	normalized = hexPattern.ReplaceAllString(normalized, "<addr>")

	normalized = pathPattern.ReplaceAllString(normalized, "<path>")

	normalized = errFileLinePattern.ReplaceAllString(normalized, "<path><line>")

	normalized = lineNumPattern.ReplaceAllString(normalized, "<line>")

	normalized = quotedPattern.ReplaceAllString(normalized, "<str>")

	normalized = numberPattern.ReplaceAllString(normalized, "<num>")

	normalized = multiSpaceRegex.ReplaceAllString(normalized, " ")

	return strings.TrimSpace(normalized)
}

func groupID(normalized string) string {
	var h uint32
	for _, c := range normalized {
		h = h*31 + uint32(c)
	}
	return fmt.Sprintf("eg_%08x", h)
}

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

	if group.Status == "resolved" {
		group.Status = "recurring"
	}

	return group
}

func (eg *ErrorGrouper) FindGroup(errorMsg string) *ErrorGroup {
	normalized := NormalizeError(errorMsg)
	id := groupID(normalized)

	eg.mu.RLock()
	defer eg.mu.RUnlock()

	return eg.Groups[id]
}

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

func (eg *ErrorGrouper) IsKnown(errorMsg string) bool {
	normalized := NormalizeError(errorMsg)
	id := groupID(normalized)

	eg.mu.RLock()
	defer eg.mu.RUnlock()

	_, exists := eg.Groups[id]
	return exists
}

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

func (eg *ErrorGrouper) GetActive() []*ErrorGroup {
	eg.mu.RLock()
	defer eg.mu.RUnlock()

	var active []*ErrorGroup
	for _, group := range eg.Groups {
		if group.Status == "active" || group.Status == "recurring" {
			active = append(active, group)
		}
	}

	sort.Slice(active, func(i, j int) bool {
		return active[i].LastSeen.After(active[j].LastSeen)
	})

	return active
}

func (eg *ErrorGrouper) FormatGroups() string {
	eg.mu.RLock()
	defer eg.mu.RUnlock()

	if len(eg.Groups) == 0 {
		return "Error Groups (0 active):\n─────────────────────────\nNo error groups recorded."
	}

	var activeCount int
	for _, g := range eg.Groups {
		if g.Status == "active" || g.Status == "recurring" {
			activeCount++
		}
	}

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
