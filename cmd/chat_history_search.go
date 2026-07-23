package cmd

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
	"github.com/mattn/go-runewidth"
)

// historySearchStyles holds the lipgloss styles for the history search overlay.
var (
	histSearchBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1)
	histSearchTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	histSearchDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	histSearchItemStyle  = lipgloss.NewStyle().Padding(0, 1)
	histSearchSelStyle   = lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("240")).Foreground(lipgloss.Color("230"))
	histSearchMatchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

// applyHistorySearchFilter filters the input history based on the current search query.
// Results are ordered by relevance: exact prefix matches first, then substring matches,
// then subsequence matches. Within each tier, most recent entries come first.
func (m *chatModel) applyHistorySearchFilter() {
	query := strings.ToLower(m.historySearchInput)
	if query == "" {
		// Show all history in reverse order (most recent first).
		m.historySearchFiltered = make([]string, 0, len(m.history))
		for i := len(m.history) - 1; i >= 0; i-- {
			m.historySearchFiltered = append(m.historySearchFiltered, m.history[i])
		}
		m.historySearchSel = 0
		return
	}

	// Score each history entry.
	type scoredEntry struct {
		entry string
		score int
		idx   int // original index for stable sort
	}
	var results []scoredEntry

	for i, entry := range m.history {
		lower := strings.ToLower(entry)
		score := 0

		if lower == query {
			score = 1000 // exact match
		} else if strings.HasPrefix(lower, query) {
			score = 800 // prefix match
		} else if strings.Contains(lower, query) {
			score = 600 // substring match
		} else if subsequenceMatch(lower, query) {
			score = 400 // subsequence match
		}

		if score > 0 {
			// Boost by recency (more recent = higher boost).
			recencyBoost := i * 10 / max(len(m.history), 1)
			results = append(results, scoredEntry{
				entry: entry,
				score: score + recencyBoost,
				idx:   i,
			})
		}
	}

	// Sort by score descending, then by index descending (most recent first).
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score ||
				(results[j].score == results[i].score && results[j].idx > results[i].idx) {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	m.historySearchFiltered = make([]string, 0, len(results))
	for _, r := range results {
		m.historySearchFiltered = append(m.historySearchFiltered, r.entry)
	}
	m.historySearchSel = 0
}

// subsequenceMatch checks if all characters of query appear in order within target.
func subsequenceMatch(target, query string) bool {
	if len(query) == 0 {
		return true
	}
	if len(target) == 0 {
		return false
	}
	q := 0
	for t := 0; t < len(target) && q < len(query); t++ {
		if target[t] == query[q] {
			q++
		}
	}
	return q == len(query)
}

// renderHistorySearchOverlay renders the Ctrl+R history search overlay.
func (m *chatModel) renderHistorySearchOverlay(viewWidth int) string {
	if !m.historySearchOpen {
		return ""
	}

	maxVisible := 8
	if viewWidth < 60 {
		viewWidth = 60
	}
	boxWidth := viewWidth - 4
	if boxWidth > 70 {
		boxWidth = 70
	}

	var b strings.Builder

	// Title
	b.WriteString(histSearchTitleStyle.Render("  History Search"))
	b.WriteString(histSearchDimStyle.Render("  (Esc to cancel, Enter to select)"))
	b.WriteString("\n\n")

	// Search input display
	queryDisplay := m.historySearchInput
	if queryDisplay == "" {
		queryDisplay = "type to search..."
	}
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render(icons.Magnify() + " "))
	if m.historySearchInput == "" {
		b.WriteString(histSearchDimStyle.Italic(true).Render(queryDisplay))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Render(queryDisplay))
	}
	b.WriteString("\n\n")

	// Results
	if len(m.historySearchFiltered) == 0 {
		b.WriteString(histSearchDimStyle.Render("  No matching history"))
	} else {
		start := 0
		if m.historySearchSel >= maxVisible {
			start = m.historySearchSel - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(m.historySearchFiltered) {
			end = len(m.historySearchFiltered)
		}

		for i := start; i < end; i++ {
			entry := m.historySearchFiltered[i]
			// Highlight the matching portion.
			displayEntry := highlightMatch(entry, m.historySearchInput, boxWidth-6)
			if i == m.historySearchSel {
				// Selected item: add a marker and use distinct style.
				marker := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Render("> ")
				b.WriteString(histSearchSelStyle.Width(boxWidth).Render(marker + displayEntry))
			} else {
				b.WriteString(histSearchItemStyle.Width(boxWidth).Render("  " + displayEntry))
			}
			b.WriteString("\n")
		}

		// Scroll indicator
		if len(m.historySearchFiltered) > maxVisible {
			b.WriteString(histSearchDimStyle.Render("  " + strconv.Itoa(m.historySearchSel+1) + "/" + strconv.Itoa(len(m.historySearchFiltered)) + " results"))
		}
	}

	return histSearchBoxStyle.Width(boxWidth).Render(b.String())
}

// highlightMatch highlights the matching substring in the entry.
func highlightMatch(entry, query string, maxWidth int) string {
	if query == "" {
		if runewidth.StringWidth(entry) > maxWidth-4 {
			return truncateString(entry, maxWidth-4)
		}
		return "  " + entry
	}

	display := entry
	if idx, matchLen := indexFold(entry, query); idx >= 0 {
		// Insert highlight around the match. indexFold returns a byte span in
		// the original string, so this never splits a multi-byte rune even when
		// case-folding changes a rune's encoded length.
		matched := entry[idx : idx+matchLen]
		display = entry[:idx] + histSearchMatchStyle.Render(matched) + entry[idx+matchLen:]
	}

	if runewidth.StringWidth(entry) > maxWidth-4 {
		display = truncateString(entry, maxWidth-4)
	}

	return "  " + display
}

// indexFold returns the byte offset and byte length of the first
// case-insensitive match of substr in s, comparing rune-by-rune via
// unicode.ToLower. It returns (-1, 0) when substr is empty or not found.
//
// Unlike strings.Index on a lowercased copy, the returned span refers to the
// original string and always falls on rune boundaries, so callers can slice s
// at [idx, idx+matchLen) without ever splitting a multi-byte rune — even for
// runes whose case-folded form has a different encoded byte length (e.g. İ, ẞ).
func indexFold(s, substr string) (idx, matchLen int) {
	sub := []rune(substr)
	if len(sub) == 0 {
		return -1, 0
	}
	for start := 0; start < len(s); {
		if end, ok := matchFoldPrefix(s, start, sub); ok {
			return start, end - start
		}
		_, size := utf8.DecodeRuneInString(s[start:])
		start += size
	}
	return -1, 0
}

// matchFoldPrefix reports whether s[start:] begins with sub under rune-wise
// case-insensitive comparison, and if so returns the byte offset just past the
// matched region in s.
func matchFoldPrefix(s string, start int, sub []rune) (end int, ok bool) {
	i := start
	for _, want := range sub {
		if i >= len(s) {
			return 0, false
		}
		got, size := utf8.DecodeRuneInString(s[i:])
		if unicode.ToLower(got) != unicode.ToLower(want) {
			return 0, false
		}
		i += size
	}
	return i, true
}

// truncateString truncates a string to the given width with ellipsis.
func truncateString(s string, maxWidth int) string {
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	w := 0
	for i, r := range runes {
		w += runewidth.RuneWidth(r)
		if w > maxWidth-1 {
			return string(runes[:i]) + "…"
		}
	}
	return s
}
