package cmd

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// scrollbarWidth is the number of terminal columns reserved for the scrollbar column.
const scrollbarWidth = 1

// Scrollbar glyph palette — tuned to look premium in dark terminals.
const (
	scrollbarTrackGlyph  = "│" // thin vertical line for the track
	scrollbarThumbGlyph  = "▊" // left three-quarters block for the thumb (thicker, solid, modern CLI style)
	scrollbarTopGlyph    = "╷" // cap at the very top of the track
	scrollbarBottomGlyph = "╵" // cap at the very bottom of the track
)

var (
	// scrollbarThumbStyle — brand orange thumb so it pops.
	scrollbarThumbStyle = lipgloss.NewStyle().Foreground(hawkColor)
	// scrollbarTrackStyle — very dim grey track so it doesn't distract.
	scrollbarTrackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3A3A"))
)

// chatHasOverflow reports whether chat content exceeds the viewport height.
func (m chatModel) chatHasOverflow() bool {
	h := m.viewport.Height
	if h <= 0 {
		return false
	}
	return m.contentLines > h
}

// chatScrollbarVisible reports when chat scrollbar should be displayed.
// Per professional CLI UX standards, the scrollbar is hidden during live auto-scroll (at bottom),
// and appears smoothly when the user scrolls back into history.
func (m chatModel) chatScrollbarVisible() bool {
	return m.chatHasOverflow() && !m.autoScroll
}

// chatViewportWidth returns the usable viewport width.
// When chat content exceeds the viewport height, one column is reserved for the scrollbar slider
// so text wrapping remains completely stable and smooth as the user scrolls up and down.
func (m chatModel) chatViewportWidth(totalWidth int) int {
	if totalWidth < 20 {
		return 80
	}
	if m.chatHasOverflow() {
		return totalWidth - scrollbarWidth
	}
	return totalWidth
}

// renderScrollbar builds a vertical scrollbar string of exactly vpHeight rows.
//
// Design:
//   - Track character: │  (dim grey — structural, not distracting)
//   - Thumb character: ▊  (brand orange — three-quarters block, thick & solid CLI style)
//   - Thumb size: proportional and compact (min 1 row, reduced by 25%)
//   - Thumb position: tracks YOffset relative to max scrollable range
//
// Returns an empty string when there is no overflow.
func (m chatModel) renderScrollbar() string {
	if !m.chatHasOverflow() {
		return ""
	}

	vpH := m.viewport.Height
	totalLines := m.contentLines
	if vpH <= 0 || totalLines <= 0 {
		return ""
	}

	// Thumb size: compact, proportional, minimum 1 row. Reduced by 25% per UX requirement.
	thumbSize := (vpH * vpH * 3) / (totalLines * 8)
	if thumbSize < 1 {
		thumbSize = 1
	}
	maxThumb := vpH / 5
	if maxThumb < 1 {
		maxThumb = 1
	}
	if thumbSize > maxThumb {
		thumbSize = maxThumb
	}

	// Track space available for the thumb to move within.
	trackSpace := vpH - thumbSize

	// Thumb position: map YOffset into [0, trackSpace].
	maxOffset := totalLines - vpH
	if maxOffset <= 0 {
		maxOffset = 1
	}
	yOffset := m.viewport.YOffset
	if yOffset < 0 {
		yOffset = 0
	}
	if yOffset > maxOffset {
		yOffset = maxOffset
	}

	thumbTop := 0
	if trackSpace > 0 {
		thumbTop = (yOffset * trackSpace) / maxOffset
	}
	thumbBottom := thumbTop + thumbSize - 1

	// Build the scrollbar column line by line.
	var sb strings.Builder
	for row := 0; row < vpH; row++ {
		if row >= thumbTop && row <= thumbBottom {
			sb.WriteString(scrollbarThumbStyle.Render(scrollbarThumbGlyph))
		} else if row == 0 {
			sb.WriteString(scrollbarTrackStyle.Render(scrollbarTopGlyph))
		} else if row == vpH-1 {
			sb.WriteString(scrollbarTrackStyle.Render(scrollbarBottomGlyph))
		} else {
			sb.WriteString(scrollbarTrackStyle.Render(scrollbarTrackGlyph))
		}
		if row < vpH-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// padToHeight pads a multi-line string with empty lines until it has exactly height lines.
func padToHeight(s string, height int) string {
	if height <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) >= height {
		return s
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// renderChatPane renders the chat viewport plus the scrollbar slider as a
// side-by-side layout, returning the full-width chat area string.
func (m chatModel) renderChatPane() string {
	chatView := m.viewport.View()
	vpH := m.viewport.Height
	if !m.chatScrollbarVisible() {
		return padToHeight(chatView, vpH)
	}

	scrollbar := m.renderScrollbar()
	if scrollbar == "" {
		return padToHeight(chatView, vpH)
	}

	targetW := m.viewport.Width
	if targetW <= 0 {
		targetW = 80
	}

	// Join each line of the chat view with the corresponding scrollbar row.
	chatLines := strings.Split(chatView, "\n")
	for len(chatLines) < vpH {
		chatLines = append(chatLines, "")
	}
	barLines := strings.Split(scrollbar, "\n")

	var out strings.Builder
	for i, line := range chatLines {
		out.WriteString(line)
		if visW := visibleWidth(line); visW < targetW {
			out.WriteString(strings.Repeat(" ", targetW-visW))
		}
		if i < len(barLines) {
			out.WriteString(barLines[i])
		}
		if i < len(chatLines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
