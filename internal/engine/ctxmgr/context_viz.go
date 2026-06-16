package ctxmgr

import (
	"fmt"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// ContextVisualizer provides a real-time view of context window usage,
// showing users exactly what occupies their context and how space is allocated.
type ContextVisualizer struct {
	MaxTokens int
	Sections  []ContextSection
	mu        sync.RWMutex
}

// ContextSection represents a named region of the context window.
type ContextSection struct {
	Name         string // "system_prompt", "memory", "conversation", "tool_results", "reserved"
	Tokens       int
	Percentage   float64
	Color        string // ANSI color code for the bar
	Items        []VizContextItem
	Compressible bool
}

// VizContextItem represents a single piece of content within a section.
type VizContextItem struct {
	Label     string
	Tokens    int
	Role      string
	Truncated bool
}

// ContextSnapshot captures context state at a particular turn for history tracking.
type ContextSnapshot struct {
	Turn        int
	TotalTokens int
	Percentage  float64
	Compacted   bool
}

// NewContextVisualizer creates a visualizer for the given maximum token budget.
func NewContextVisualizer(maxTokens int) *ContextVisualizer {
	return &ContextVisualizer{
		MaxTokens: maxTokens,
		Sections:  nil,
	}
}

// Update replaces the current sections and recalculates percentages.
func (cv *ContextVisualizer) Update(sections []ContextSection) {
	cv.mu.Lock()
	defer cv.mu.Unlock()

	for i := range sections {
		if cv.MaxTokens > 0 {
			sections[i].Percentage = float64(sections[i].Tokens) / float64(cv.MaxTokens) * 100.0
		} else {
			sections[i].Percentage = 0
		}
	}
	cv.Sections = sections
}

// totalUsed returns the sum of tokens across all sections.
func (cv *ContextVisualizer) totalUsed() int {
	total := 0
	for _, s := range cv.Sections {
		total += s.Tokens
	}
	return total
}

// usedPercentage returns the fraction of context that is used.
func (cv *ContextVisualizer) usedPercentage() float64 {
	if cv.MaxTokens == 0 {
		return 0
	}
	return float64(cv.totalUsed()) / float64(cv.MaxTokens) * 100.0
}

// RenderBar renders a horizontal stacked bar showing each section's proportion.
//
// Example output:
//
//	Context: [████████░░░░░░░░░░░░] 42% used (84,000/200,000)
//	         [sys|mem|████conv████|tool|░░░reserved░░░]
func (cv *ContextVisualizer) RenderBar(width int) string {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	if width < 10 {
		width = 10
	}

	used := cv.totalUsed()
	pct := cv.usedPercentage()

	// Top bar: filled vs empty
	filledCount := int(float64(width) * pct / 100.0)
	if filledCount > width {
		filledCount = width
	}
	emptyCount := width - filledCount

	topBar := strings.Repeat("█", filledCount) + strings.Repeat("░", emptyCount)
	line1 := fmt.Sprintf("Context: [%s] %d%% used (%s/%s)",
		topBar, int(pct), formatTokens(used), formatTokens(cv.MaxTokens))

	// Bottom bar: section labels
	remaining := width
	var parts []string
	for _, sec := range cv.Sections {
		chars := int(float64(width) * sec.Percentage / 100.0)
		if chars < 1 && sec.Tokens > 0 {
			chars = 1
		}
		if chars > remaining {
			chars = remaining
		}
		remaining -= chars

		label := sectionShortName(sec.Name)
		part := fitLabel(label, chars)
		parts = append(parts, part)
	}

	// Fill remaining space with available
	if remaining > 0 {
		parts = append(parts, fitLabel("free", remaining))
	}

	bottomBar := strings.Join(parts, "|")
	// Truncate if needed to fit within width
	if len([]rune(bottomBar)) > width {
		runes := []rune(bottomBar)
		bottomBar = string(runes[:width])
	}

	padding := "         " // align with "Context: "
	line2 := fmt.Sprintf("%s[%s]", padding, bottomBar)

	return line1 + "\n" + line2
}

// RenderDetailed renders a full breakdown table of context usage by section.
func (cv *ContextVisualizer) RenderDetailed() string {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	used := cv.totalUsed()
	pct := cv.usedPercentage()
	available := cv.MaxTokens - used
	if available < 0 {
		available = 0
	}

	var b strings.Builder

	b.WriteString(fmt.Sprintf("Context Window Usage (%s / %s tokens — %d%%)\n",
		formatTokens(used), formatTokens(cv.MaxTokens), int(pct)))
	b.WriteString("═══════════════════════════════════════════════════\n")
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%-22s %8s %6s  %s\n", "Section", "Tokens", "%", "Bar"))
	b.WriteString("─────────────────────────────────────────────────\n")

	for _, sec := range cv.Sections {
		barLen := int(sec.Percentage / 2.0) // scale: 2% per block
		if barLen < 1 && sec.Tokens > 0 {
			barLen = 1
		}
		bar := strings.Repeat("█", barLen)

		b.WriteString(fmt.Sprintf("%-22s %8s %5.1f%%  %s\n",
			sectionDisplayName(sec.Name),
			formatTokens(sec.Tokens),
			sec.Percentage,
			bar))

		// Render sub-items indented
		for i, item := range sec.Items {
			prefix := "├─"
			if i == len(sec.Items)-1 {
				prefix = "└─"
			}
			truncMark := ""
			if item.Truncated {
				truncMark = " [truncated]"
			}
			b.WriteString(fmt.Sprintf("  %s %-16s %8s%s\n",
				prefix, item.Label, formatTokens(item.Tokens), truncMark))
		}
	}

	b.WriteString("─────────────────────────────────────────────────\n")

	availPct := 0.0
	if cv.MaxTokens > 0 {
		availPct = float64(available) / float64(cv.MaxTokens) * 100.0
	}
	availBar := strings.Repeat("█", int(availPct/2.0))
	b.WriteString(fmt.Sprintf("%-22s %8s %5.1f%%  %s\n",
		"Available", formatTokens(available), availPct, availBar))

	return b.String()
}

// RenderCompact renders a single-line summary of context usage.
//
// Example: [42% | sys:2% mem:1% conv:26% repo:4% ctx:6% | 116K free]
func (cv *ContextVisualizer) RenderCompact() string {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	used := cv.totalUsed()
	pct := int(cv.usedPercentage())
	available := cv.MaxTokens - used
	if available < 0 {
		available = 0
	}

	var parts []string
	for _, sec := range cv.Sections {
		parts = append(parts, fmt.Sprintf("%s:%d%%", sectionShortName(sec.Name), int(sec.Percentage)))
	}

	return fmt.Sprintf("[%d%% | %s | %s free]",
		pct, strings.Join(parts, " "), formatTokensShort(available))
}

// Recommend returns actionable suggestions based on current context state.
func (cv *ContextVisualizer) Recommend() []string {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	var recs []string

	for _, sec := range cv.Sections {
		switch {
		case sec.Name == "conversation" && sec.Percentage > 20:
			recs = append(recs, fmt.Sprintf(
				"Conversation is %.0f%% of context — consider compacting", sec.Percentage,
			))
		case sec.Name == "tool_results" && sec.Tokens > 5000:
			recs = append(recs, fmt.Sprintf(
				"Tool results are large — enabling tok compression would save ~60%%",
			))
		case sec.Name == "readonly_context" && sec.Tokens > 8000:
			recs = append(recs, fmt.Sprintf(
				"%s tokens in read-only context — review if all files are needed",
				formatTokens(sec.Tokens),
			))
		case sec.Compressible && sec.Percentage > 15:
			recs = append(recs, fmt.Sprintf(
				"%s is compressible and using %.0f%% — consider trimming",
				sectionDisplayName(sec.Name), sec.Percentage,
			))
		}
	}

	return recs
}

// HistoryChart renders context usage across turns as a sparkline-style chart.
//
// Example:
//
//	Turn  1: [██░░░░░░░░░░░░░░░░░░]  8%
//	Turn  5: [████████░░░░░░░░░░░░] 35%
//	Turn 10: [████████████████░░░░] 78%  ← compacted
//	Turn 11: [██████░░░░░░░░░░░░░░] 28%
func (cv *ContextVisualizer) HistoryChart(snapshots []ContextSnapshot, width int) string {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	if width < 10 {
		width = 10
	}

	var b strings.Builder
	for _, snap := range snapshots {
		filled := int(float64(width) * snap.Percentage / 100.0)
		if filled > width {
			filled = width
		}
		empty := width - filled

		bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
		suffix := ""
		if snap.Compacted {
			suffix = "  ← compacted"
		}

		b.WriteString(fmt.Sprintf("Turn %2d: [%s] %2d%%%s\n",
			snap.Turn, bar, int(snap.Percentage), suffix))
	}

	return strings.TrimRight(b.String(), "\n")
}

// TakeSnapshot captures the current context state for a given turn number.
func (cv *ContextVisualizer) TakeSnapshot(turn int) ContextSnapshot {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	used := cv.totalUsed()
	pct := cv.usedPercentage()

	return ContextSnapshot{
		Turn:        turn,
		TotalTokens: used,
		Percentage:  pct,
		Compacted:   false,
	}
}

// WarnIfCritical returns a warning string if context usage is dangerously high.
// Returns empty string if usage is within safe limits.
func (cv *ContextVisualizer) WarnIfCritical() string {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	pct := cv.usedPercentage()

	if pct > 95 {
		return fmt.Sprintf("CRIT"+" Context %d%% full — compacting now", int(pct))
	}
	if pct > 85 {
		return fmt.Sprintf(icons.Alert()+" Context %d%% full — auto-compact will trigger soon", int(pct))
	}
	return ""
}

// --- helper functions ---

// formatTokens formats a token count with comma separators.
func formatTokens(n int) string {
	if n < 0 {
		return "-" + formatTokens(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if result.Len() > 0 {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}

// formatTokensShort formats tokens in abbreviated form (e.g., "116K").
func formatTokensShort(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000.0)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%dK", n/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// sectionShortName returns an abbreviated name for bar display.
func sectionShortName(name string) string {
	switch name {
	case "system_prompt":
		return "sys"
	case "memory":
		return "mem"
	case "conversation":
		return "conv"
	case "tool_results":
		return "tool"
	case "reserved":
		return "rsv"
	case "repo_map":
		return "repo"
	case "readonly_context":
		return "ctx"
	default:
		if len(name) > 4 {
			return name[:4]
		}
		return name
	}
}

// sectionDisplayName returns a human-friendly display name.
func sectionDisplayName(name string) string {
	switch name {
	case "system_prompt":
		return "System Prompt"
	case "memory":
		return "Memory (yaad)"
	case "conversation":
		return "Conversation"
	case "tool_results":
		return "Tool Results"
	case "reserved":
		return "Reserved (output)"
	case "repo_map":
		return "Repo Map"
	case "readonly_context":
		return "Read-Only Context"
	default:
		return name
	}
}

// fitLabel centers or pads a label to fit within a given character width.
// Width is measured in runes (visual characters), not bytes.
func fitLabel(label string, width int) string {
	if width <= 0 {
		return ""
	}
	labelRunes := []rune(label)
	if len(labelRunes) >= width {
		return string(labelRunes[:width])
	}
	// Center the label within the space using fill characters
	padTotal := width - len(labelRunes)
	padLeft := padTotal / 2
	padRight := padTotal - padLeft
	return strings.Repeat("░", padLeft) + label + strings.Repeat("░", padRight)
}
