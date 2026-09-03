package cmd

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
	"github.com/mattn/go-runewidth"

	"github.com/GrayCodeAI/graycode-cli/internal/session"
)

// sessionPickerStyles holds the lipgloss styles for the session picker overlay.
var (
	sessPickBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1)
	sessPickTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	sessPickDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	sessPickItemStyle  = lipgloss.NewStyle().Padding(0, 1)
	sessPickSelStyle   = lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("240")).Foreground(lipgloss.Color("230"))
	sessPickMatchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	sessPickEmptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
)

// applySessionPickerFilter filters the session entries based on the current search query.
// Matches against session ID, preview text, and CWD. Results are scored by relevance
// and recency.
func (m *chatModel) applySessionPickerFilter() {
	query := strings.ToLower(m.sessionPickerInput)
	if query == "" {
		// Show all sessions in the original order (newest first from session.List).
		m.sessionPickerFiltered = make([]session.Entry, len(m.sessionPickerEntries))
		copy(m.sessionPickerFiltered, m.sessionPickerEntries)
		m.sessionPickerSel = 0
		return
	}

	type scoredEntry struct {
		entry session.Entry
		score int
		idx   int
	}
	var results []scoredEntry

	for i, e := range m.sessionPickerEntries {
		lowerID := strings.ToLower(e.ID)
		lowerPreview := strings.ToLower(e.Preview)
		lowerCWD := strings.ToLower(e.CWD)
		score := 0

		// ID match (highest priority — exact prefix of session ID).
		if strings.HasPrefix(lowerID, query) {
			score = 1000
		} else if strings.Contains(lowerID, query) {
			score = 800
		} else if strings.HasPrefix(lowerPreview, query) {
			score = 600
		} else if strings.Contains(lowerPreview, query) {
			score = 500
		} else if strings.Contains(lowerCWD, query) {
			score = 400
		} else if subsequenceMatch(lowerPreview, query) {
			score = 300
		} else if subsequenceMatch(lowerID, query) {
			score = 200
		}

		if score > 0 {
			// Boost by recency.
			recencyBoost := i * 10 / max(len(m.sessionPickerEntries), 1)
			results = append(results, scoredEntry{
				entry: e,
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

	m.sessionPickerFiltered = make([]session.Entry, 0, len(results))
	for _, r := range results {
		m.sessionPickerFiltered = append(m.sessionPickerFiltered, r.entry)
	}
	m.sessionPickerSel = 0
}

// renderSessionPickerOverlay renders the Ctrl+S session picker overlay.
func (m *chatModel) renderSessionPickerOverlay(viewWidth int) string {
	if !m.sessionPickerOpen {
		return ""
	}

	maxVisible := 8
	if viewWidth < 60 {
		viewWidth = 60
	}
	boxWidth := viewWidth - 4
	if boxWidth > 80 {
		boxWidth = 80
	}

	var b strings.Builder

	// Title
	b.WriteString(sessPickTitleStyle.Render("  Session Picker"))
	b.WriteString(sessPickDimStyle.Render("  (Esc to cancel, Enter to resume)"))
	b.WriteString("\n\n")

	// Search input display
	queryDisplay := m.sessionPickerInput
	if queryDisplay == "" {
		queryDisplay = "type to filter sessions..."
	}
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render(icons.Magnify() + " "))
	if m.sessionPickerInput == "" {
		b.WriteString(sessPickDimStyle.Italic(true).Render(queryDisplay))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Render(queryDisplay))
	}
	b.WriteString("\n\n")

	// Results
	if len(m.sessionPickerFiltered) == 0 {
		if len(m.sessionPickerEntries) == 0 {
			b.WriteString(sessPickEmptyStyle.Render("  No saved sessions found"))
		} else {
			b.WriteString(sessPickEmptyStyle.Render("  No matching sessions"))
		}
	} else {
		start := 0
		if m.sessionPickerSel >= maxVisible {
			start = m.sessionPickerSel - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(m.sessionPickerFiltered) {
			end = len(m.sessionPickerFiltered)
		}

		for i := start; i < end; i++ {
			entry := m.sessionPickerFiltered[i]
			line := formatSessionEntry(entry, m.sessionPickerInput, boxWidth-4)
			if i == m.sessionPickerSel {
				// Selected item: add a marker and use distinct style (consistent with history search).
				marker := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Render("> ")
				b.WriteString(sessPickSelStyle.Width(boxWidth).Render(marker + line))
			} else {
				b.WriteString(sessPickItemStyle.Width(boxWidth).Render("  " + line))
			}
			b.WriteString("\n")
		}

		// Scroll indicator
		if len(m.sessionPickerFiltered) > maxVisible {
			b.WriteString(sessPickDimStyle.Render("  " + strconv.Itoa(m.sessionPickerSel+1) + "/" + strconv.Itoa(len(m.sessionPickerFiltered)) + " sessions"))
		}
	}

	return sessPickBoxStyle.Width(boxWidth).Render(b.String())
}

// formatSessionEntry formats a single session entry for display.
// Shows: ID, preview, CWD (shortened), and time-ago.
func formatSessionEntry(e session.Entry, query string, maxWidth int) string {
	// Ensure minimum width to avoid negative calculations.
	if maxWidth < 20 {
		maxWidth = 20
	}

	// Format: "  ID  preview  [cwd]  time-ago"
	idStr := e.ID
	if len(idStr) > 8 {
		idStr = idStr[:8]
	}

	preview := e.Preview
	if preview == "" {
		preview = "(no messages)"
	}

	// Shorten CWD for display.
	cwdStr := ""
	if e.CWD != "" {
		cwdStr = shortenHomePath(e.CWD)
		if len(cwdStr) > 20 {
			tail := cwdStr[len(cwdStr)-17:]
			for len(tail) > 0 && !utf8.ValidString(tail) {
				tail = tail[1:]
			}
			cwdStr = "..." + tail
		}
		cwdStr = "[" + cwdStr + "]"
	}

	// Time ago.
	timeAgo := formatTimeAgo(e.UpdatedAt)

	// Build the line: ID + preview + cwd + time-ago
	leftPart := "  " + idStr + "  "
	rightPart := "  " + cwdStr + "  " + timeAgo + " "

	// Calculate available width for preview.
	leftW := runewidth.StringWidth(leftPart)
	rightW := runewidth.StringWidth(rightPart)
	previewMaxW := maxWidth - leftW - rightW
	if previewMaxW < 10 {
		previewMaxW = 10
	}

	previewDisplay := preview
	if runewidth.StringWidth(previewDisplay) > previewMaxW {
		previewDisplay = truncateString(previewDisplay, previewMaxW)
	}

	// Highlight match in preview. indexFold returns a byte span in the original
	// string, so the slice never splits a multi-byte rune even when case-folding
	// changes a rune's encoded length.
	if query != "" {
		if idx, matchLen := indexFold(previewDisplay, query); idx >= 0 {
			matched := previewDisplay[idx : idx+matchLen]
			previewDisplay = previewDisplay[:idx] + sessPickMatchStyle.Render(matched) + previewDisplay[idx+matchLen:]
		}
	}

	// Pad preview to fill the gap.
	previewW := runewidth.StringWidth(preview)
	if previewW < previewMaxW {
		previewDisplay += strings.Repeat(" ", previewMaxW-previewW)
	}

	return leftPart + previewDisplay + rightPart
}

// formatTimeAgo returns a human-readable relative time string.
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return strconv.Itoa(m) + "m ago"
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return strconv.Itoa(h) + "h ago"
	}
	if d < 30*24*time.Hour {
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return strconv.Itoa(days) + "d ago"
	}
	return t.Format("Jan 02")
}

// resumeSessionByID loads a session by ID and updates the model state.
func (m *chatModel) resumeSessionByID(id string) (tea.Model, tea.Cmd) {
	saved, err := session.Load(id)
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Failed to load session: " + err.Error()})
		m.viewDirty = true
		m.updateViewportContent()
		return m, nil
	}
	m.sessionID = saved.ID
	m.invalidateViewportCache()
	m.messages = []displayMsg{{role: "welcome", content: m.welcomeCache}}
	msgs := session.ToRuntimeMessages(saved.Messages)
	for _, sm := range saved.Messages {
		if sm.Role == "user" || sm.Role == "assistant" {
			m.messages = append(m.messages, displayMsg{role: sm.Role, content: sm.Content})
		}
	}
	m.session.LoadMessages(msgs)
	m.messages = append(m.messages, displayMsg{role: "system", content: "Resumed session " + saved.ID})
	m.viewDirty = true
	m.autoScroll = false
	return m, nil
}
