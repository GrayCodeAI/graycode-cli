package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// stdoutIsTerminal reports whether stdout is connected to a terminal (TTY).
// When stdout is a pipe or file — which is exactly the case when an agent or
// shell script captures hawk's output — this is false, and color/Unicode
// chrome must be suppressed so the payload stays clean. It is a var so tests
// can override it.
var stdoutIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// TreeNode represents a node in a tree structure for FormatTree.
type TreeNode struct {
	Name     string
	Children []TreeNode
	Icon     string
}

// OutputTheme holds ANSI color codes for themed output.
type OutputTheme struct {
	Primary   string
	Secondary string
	Success   string
	Error     string
	Warning   string
	Info      string
	Muted     string
	Reset     string
}

// OutputFormatter adapts output format based on terminal capabilities, width, and user preferences.
type OutputFormatter struct {
	Width          int
	ColorEnabled   bool
	UnicodeEnabled bool
	Theme          OutputTheme
	Verbose        bool
	mu             sync.RWMutex
}

// NewOutputFormatter creates an OutputFormatter with auto-detected terminal settings.
func NewOutputFormatter() *OutputFormatter {
	colorEnabled := DetectColorSupport()
	unicodeEnabled := DetectUnicodeSupport()
	width := DetectTerminalWidth()

	theme := OutputTheme{
		Reset: "",
	}
	if colorEnabled {
		theme = OutputTheme{
			Primary:   "\033[36m",
			Secondary: "\033[35m",
			Success:   "\033[32m",
			Error:     "\033[31m",
			Warning:   "\033[33m",
			Info:      "\033[34m",
			Muted:     "\033[90m",
			Reset:     "\033[0m",
		}
	}

	return &OutputFormatter{
		Width:          width,
		ColorEnabled:   colorEnabled,
		UnicodeEnabled: unicodeEnabled,
		Theme:          theme,
		Verbose:        false,
	}
}

// FormatSuccess formats a success message with a green checkmark.
func (f *OutputFormatter) FormatSuccess(msg string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	icon := "[ok]"
	if f.UnicodeEnabled {
		icon = icons.CheckBold() + " "
	}
	if f.ColorEnabled {
		return fmt.Sprintf("%s%s %s%s", f.Theme.Success, icon, msg, f.Theme.Reset)
	}
	return fmt.Sprintf("%s %s", icon, msg)
}

// FormatError formats an error message with a red X.
func (f *OutputFormatter) FormatError(msg string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	icon := "[X]"
	if f.UnicodeEnabled {
		icon = icons.CloseThick() + " "
	}
	if f.ColorEnabled {
		return fmt.Sprintf("%s%s%s%s", f.Theme.Error, icon, msg, f.Theme.Reset)
	}
	return fmt.Sprintf("%s%s", icon, msg)
}

// FormatWarning formats a warning message with a yellow triangle.
func (f *OutputFormatter) FormatWarning(msg string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	icon := "[!]"
	if f.UnicodeEnabled {
		icon = "!" + icons.Alert() + " "
	}
	if f.ColorEnabled {
		return fmt.Sprintf("%s%s %s%s", f.Theme.Warning, icon, msg, f.Theme.Reset)
	}
	return fmt.Sprintf("%s %s", icon, msg)
}

// FormatInfo formats an informational message with a blue circle.
func (f *OutputFormatter) FormatInfo(msg string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	icon := "[i]"
	if f.UnicodeEnabled {
		icon = icons.CircleFilled() + " "
	}
	if f.ColorEnabled {
		return fmt.Sprintf("%s%s %s%s", f.Theme.Info, icon, msg, f.Theme.Reset)
	}
	return fmt.Sprintf("%s %s", icon, msg)
}

// FormatTable formats tabular data with auto-calculated column widths.
// If Unicode is supported, box-drawing characters are used.
// Cells are truncated to fit within the terminal width.
func (f *OutputFormatter) FormatTable(headers []string, rows [][]string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if len(headers) == 0 {
		return ""
	}

	numCols := len(headers)

	// Calculate max width for each column
	colWidths := make([]int, numCols)
	for i, h := range headers {
		if len(h) > colWidths[i] {
			colWidths[i] = len(h)
		}
	}
	for _, row := range rows {
		for i := 0; i < numCols && i < len(row); i++ {
			if len(row[i]) > colWidths[i] {
				colWidths[i] = len(row[i])
			}
		}
	}

	// Truncate columns to fit terminal width
	// Account for separators: | col | col | col | => numCols*3 + 1
	separatorOverhead := numCols*3 + 1
	availableWidth := f.Width - separatorOverhead
	if availableWidth < numCols {
		availableWidth = numCols
	}

	totalColWidth := 0
	for _, w := range colWidths {
		totalColWidth += w
	}

	if totalColWidth > availableWidth {
		// Proportionally shrink columns
		for i := range colWidths {
			colWidths[i] = colWidths[i] * availableWidth / totalColWidth
			if colWidths[i] < 3 {
				colWidths[i] = 3
			}
		}
	}

	var sb strings.Builder

	if f.UnicodeEnabled {
		// Unicode box-drawing table
		sb.WriteString(f.boxTop(colWidths))
		sb.WriteString("\n")
		sb.WriteString(f.boxRow(headers, colWidths))
		sb.WriteString("\n")
		sb.WriteString(f.boxMid(colWidths))
		sb.WriteString("\n")
		for _, row := range rows {
			sb.WriteString(f.boxRow(row, colWidths))
			sb.WriteString("\n")
		}
		sb.WriteString(f.boxBottom(colWidths))
	} else {
		// ASCII table
		sb.WriteString(f.asciiSep(colWidths))
		sb.WriteString("\n")
		sb.WriteString(f.asciiRow(headers, colWidths))
		sb.WriteString("\n")
		sb.WriteString(f.asciiSep(colWidths))
		sb.WriteString("\n")
		for _, row := range rows {
			sb.WriteString(f.asciiRow(row, colWidths))
			sb.WriteString("\n")
		}
		sb.WriteString(f.asciiSep(colWidths))
	}

	return sb.String()
}

func (f *OutputFormatter) boxTop(widths []int) string {
	var parts []string
	for _, w := range widths {
		parts = append(parts, strings.Repeat("─", w+2))
	}
	return "┌" + strings.Join(parts, "┬") + "┐"
}

func (f *OutputFormatter) boxMid(widths []int) string {
	var parts []string
	for _, w := range widths {
		parts = append(parts, strings.Repeat("─", w+2))
	}
	return "├" + strings.Join(parts, "┼") + "┤"
}

func (f *OutputFormatter) boxBottom(widths []int) string {
	var parts []string
	for _, w := range widths {
		parts = append(parts, strings.Repeat("─", w+2))
	}
	return "└" + strings.Join(parts, "┴") + "┘"
}

func (f *OutputFormatter) boxRow(cells []string, widths []int) string {
	var parts []string
	for i, w := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		cell = f.truncateCell(cell, w)
		parts = append(parts, " "+f.padRight(cell, w)+" ")
	}
	return "│" + strings.Join(parts, "│") + "│"
}

func (f *OutputFormatter) asciiSep(widths []int) string {
	var parts []string
	for _, w := range widths {
		parts = append(parts, strings.Repeat("-", w+2))
	}
	return "+" + strings.Join(parts, "+") + "+"
}

func (f *OutputFormatter) asciiRow(cells []string, widths []int) string {
	var parts []string
	for i, w := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		cell = f.truncateCell(cell, w)
		parts = append(parts, " "+f.padRight(cell, w)+" ")
	}
	return "|" + strings.Join(parts, "|") + "|"
}

func (f *OutputFormatter) truncateCell(s string, maxWidth int) string {
	return truncateWithEllipsis(s, maxWidth)
}

func (f *OutputFormatter) padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// FormatList formats a list of items, optionally with numbering.
func (f *OutputFormatter) FormatList(items []string, numbered bool) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var sb strings.Builder
	for i, item := range items {
		if numbered {
			sb.WriteString(fmt.Sprintf("%d. %s", i+1, item))
		} else {
			bullet := "-"
			if f.UnicodeEnabled {
				bullet = "•"
			}
			sb.WriteString(fmt.Sprintf("%s %s", bullet, item))
		}
		if i < len(items)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// FormatProgress formats a progress bar.
// Example: [████████░░░░] 67% (8/12) Installing packages...
func (f *OutputFormatter) FormatProgress(current, total int, label string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if total <= 0 {
		total = 1
	}
	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}

	percent := current * 100 / total

	barWidth := 20
	filled := barWidth * current / total
	empty := barWidth - filled

	var filledChar, emptyChar string
	if f.UnicodeEnabled {
		filledChar = "█"
		emptyChar = "░"
	} else {
		filledChar = "#"
		emptyChar = "-"
	}

	bar := strings.Repeat(filledChar, filled) + strings.Repeat(emptyChar, empty)
	return fmt.Sprintf("[%s] %d%% (%d/%d) %s", bar, percent, current, total, label)
}

// FormatTree formats a tree structure with box-drawing characters.
func (f *OutputFormatter) FormatTree(root string, children []TreeNode) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(root)
	sb.WriteString("\n")
	f.formatTreeNodes(&sb, children, "")
	return sb.String()
}

func (f *OutputFormatter) formatTreeNodes(sb *strings.Builder, nodes []TreeNode, prefix string) {
	for i, node := range nodes {
		isLast := i == len(nodes)-1

		var connector, childPrefix string
		if f.UnicodeEnabled {
			if isLast {
				connector = "└── "
				childPrefix = prefix + "    "
			} else {
				connector = "├── "
				childPrefix = prefix + "│   "
			}
		} else {
			if isLast {
				connector = "`-- "
				childPrefix = prefix + "    "
			} else {
				connector = "|-- "
				childPrefix = prefix + "|   "
			}
		}

		name := node.Name
		if node.Icon != "" {
			name = node.Icon + " " + name
		}

		sb.WriteString(prefix + connector + name + "\n")

		if len(node.Children) > 0 {
			f.formatTreeNodes(sb, node.Children, childPrefix)
		}
	}
}

// FormatDiff formats added/removed line counts with green/red coloring.
func (f *OutputFormatter) FormatDiff(added, removed int) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	addStr := fmt.Sprintf("+%d", added)
	remStr := fmt.Sprintf("-%d", removed)

	if f.ColorEnabled {
		return fmt.Sprintf("%s%s%s %s%s%s", f.Theme.Success, addStr, f.Theme.Reset, f.Theme.Error, remStr, f.Theme.Reset)
	}
	return fmt.Sprintf("%s %s", addStr, remStr)
}

// FormatDuration formats a time.Duration into a human-readable string.
func (f *OutputFormatter) FormatDuration(d time.Duration) string {
	if d < time.Second {
		ms := d.Milliseconds()
		if ms == 0 {
			return "0s"
		}
		return fmt.Sprintf("%dms", ms)
	}

	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}

	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) - mins*60
		if secs == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm %ds", mins, secs)
	}

	hours := int(d.Hours())
	mins := int(d.Minutes()) - hours*60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, mins)
}

// FormatBytes formats a byte count into a human-readable string.
func (f *OutputFormatter) FormatBytes(n int64) string {
	if n < 0 {
		return "0B"
	}
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1fGB", float64(n)/(1024*1024*1024))
}

// FormatNumber formats an integer into a human-readable string with commas or suffixes.
func (f *OutputFormatter) FormatNumber(n int) string {
	if n < 0 {
		return "-" + f.FormatNumber(-n)
	}
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 10000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	if n >= 1000 {
		return addCommas(n)
	}
	return fmt.Sprintf("%d", n)
}

func addCommas(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
		if len(s) > remainder {
			result.WriteString(",")
		}
	}
	for i := remainder; i < len(s); i += 3 {
		result.WriteString(s[i : i+3])
		if i+3 < len(s) {
			result.WriteString(",")
		}
	}
	return result.String()
}

// Truncate truncates a string to maxWidth, appending "..." if it exceeds.
func (f *OutputFormatter) Truncate(s string, maxWidth int) string {
	return truncateWithEllipsis(s, maxWidth)
}

// DetectTerminalWidth returns the terminal width or a default of 80.
func DetectTerminalWidth() int {
	// Try to get terminal width from environment
	colsStr := os.Getenv("COLUMNS")
	if colsStr != "" {
		width := 0
		for _, c := range colsStr {
			if c >= '0' && c <= '9' {
				width = width*10 + int(c-'0')
			} else {
				width = 0
				break
			}
		}
		if width > 0 {
			return width
		}
	}

	// Default width
	return 80
}

// DetectColorSupport checks if the terminal supports color output.
func DetectColorSupport() bool {
	// FORCE_COLOR always wins
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}

	// NO_COLOR disables color (https://no-color.org/)
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// Stdout is not a TTY (piped to an agent, file, or another process):
	// suppress ANSI so the captured output is clean. An explicit FORCE_COLOR
	// above already overrode this for callers that pipe but still want color.
	if !stdoutIsTerminal() {
		return false
	}

	// Check TERM
	t := os.Getenv("TERM")
	if t == "dumb" || t == "" {
		return false
	}

	return true
}

// DetectUnicodeSupport checks if the terminal supports Unicode characters.
func DetectUnicodeSupport() bool {
	// Non-TTY stdout: emit ASCII so box-drawing/glyphs don't corrupt captured
	// output. FORCE_COLOR is a color signal only, so it does not override here.
	if !stdoutIsTerminal() {
		return false
	}

	lang := os.Getenv("LANG")
	lcAll := os.Getenv("LC_ALL")
	lcCtype := os.Getenv("LC_CTYPE")

	for _, v := range []string{lcAll, lcCtype, lang} {
		lower := strings.ToLower(v)
		if strings.Contains(lower, "utf-8") || strings.Contains(lower, "utf8") {
			return true
		}
	}

	// Default: no Unicode support detected
	return false
}
