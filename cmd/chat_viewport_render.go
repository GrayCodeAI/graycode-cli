package cmd

import (
	"strings"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// Viewport render cache — avoids re-wrapping and re-rendering markdown for the
// entire scrollback on every 50ms stream tick. Stable prefix is cached; only
// new/changed messages and the live tail (partial + spinner) are rebuilt.

func (m *chatModel) invalidateViewportCache() {
	m.vpStableContent = ""
	m.vpRenderedMsgs = 0
	m.vpRenderWidth = 0
	m.vpLastMsgLen = 0
}

func renderDisplayMessage(msg displayMsg, i int, messages []displayMsg, viewWidth int) string {
	hawkC := "\033[38;2;255;94;14m"
	rst := "\033[0m"
	bgDark := "\033[48;2;30;30;40m"

	var b strings.Builder

	switch msg.role {
	case "user":
		if i > 0 {
			b.WriteByte('\n')
		}
		wrapped := wrapText(msg.content, viewWidth-1, 3)
		wrappedLines := strings.Split(wrapped, "\n")
		for li, wl := range wrappedLines {
			if li == 0 {
				b.WriteString(bgDark + hawkC + "█" + rst + bgDark + "  " + wl)
			} else {
				b.WriteString(bgDark + "   " + wl)
			}
			visW := 3 + visibleWidth(wl)
			if pad := viewWidth - visW; pad > 0 {
				b.WriteString(strings.Repeat(" ", pad))
			}
			b.WriteString(rst)
			if li < len(wrappedLines)-1 {
				b.WriteByte('\n')
			}
		}
	case "assistant":
		content := strings.TrimLeft(msg.content, "\n\r")
		b.WriteString(hawkC + icons.Robot() + " " + rst + renderMarkdown(content, viewWidth-3))
	case "tool_use":
		b.WriteString(toolStyle.Render(icons.Bolt() + " " + msg.content))
	case "tool_result":
		if strings.Contains(msg.content, "diff ") && strings.Contains(msg.content, " lines") {
			parts := strings.SplitN(msg.content, "\ndiff ", 2)
			mainContent := parts[0]
			diffPart := ""
			if len(parts) > 1 {
				diffPart = "diff " + parts[1]
			}
			toolWrapped := wrapText(mainContent, viewWidth-6, 0)
			b.WriteString(toolDimStyle.Render("    " + strings.ReplaceAll(toolWrapped, "\n", "\n    ")))
			if diffPart != "" {
				b.WriteString("\n")
				diffStyled := renderDiffSummary(diffPart, viewWidth-6)
				b.WriteString("    " + diffStyled)
			}
		} else if strings.Contains(msg.content, "Self-review found issues") {
			b.WriteString(errorStyle.Render("    " + icons.CloseThick() + " " + msg.content))
		} else if strings.Contains(msg.content, "## Self-Reflection") {
			parts := strings.SplitN(msg.content, "## Self-Reflection", 2)
			mainContent := parts[0]
			reflectionPart := ""
			if len(parts) > 1 {
				reflectionPart = "## Self-Reflection" + parts[1]
			}
			toolWrapped := wrapText(mainContent, viewWidth-6, 0)
			b.WriteString(toolDimStyle.Render("    " + strings.ReplaceAll(toolWrapped, "\n", "\n    ")))
			if reflectionPart != "" {
				b.WriteString("\n")
				reflStyled := renderReflectionBox(reflectionPart, viewWidth-6)
				b.WriteString("    " + reflStyled)
			}
		} else {
			display := formatToolResultDisplay(msg.content)
			toolWrapped := wrapText(display, viewWidth-6, 0)
			b.WriteString(toolDimStyle.Render("    " + strings.ReplaceAll(toolWrapped, "\n", "\n    ")))
		}
	case "thinking":
		thinkWrapped := wrapText(msg.content, viewWidth-4, 3)
		b.WriteString(dimStyle.Render(icons.Brain() + " " + thinkWrapped))
	case "welcome":
		// rendered in fixed welcome pane, not scrollback
	case "system":
		sysWrapped := wrapText(msg.content, viewWidth-2, 0)
		b.WriteString(dimStyle.Render(sysWrapped))
	case "setup_complete":
		b.WriteString(renderSetupCompleteMessage(msg.content))
	case "permission":
		b.WriteString(renderPermissionBox(msg.content, viewWidth))
	case "question":
		qWrapped := wrapText(msg.content, viewWidth-2, 2)
		b.WriteString(toolStyle.Render(qWrapped))
	case "usage":
		b.WriteString(dimStyle.Render("  " + msg.content))
	case "warning":
		warnWrapped := wrapText(msg.content, viewWidth-8, 0)
		b.WriteString(warnStyle.Render(warnWrapped))
	case "error":
		errWrapped := wrapText(msg.content, viewWidth-8, 7)
		b.WriteString(errorStyle.Render("error: " + errWrapped))
	}

	switch msg.role {
	case "tool_use":
		if i+1 < len(messages) && messages[i+1].role == "tool_result" {
			b.WriteByte('\n')
		} else {
			b.WriteString("\n\n")
		}
	case "tool_result":
		if i+1 < len(messages) && messages[i+1].role == "tool_use" {
			b.WriteByte('\n')
		} else {
			b.WriteString("\n\n")
		}
	case "usage":
		b.WriteByte('\n')
	case "welcome":
		// no trailing space
	default:
		if msg.role != "" {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

func renderMessagesRange(messages []displayMsg, start, end int, viewWidth int) string {
	var b strings.Builder
	for i := start; i < end && i < len(messages); i++ {
		b.WriteString(renderDisplayMessage(messages[i], i, messages, viewWidth))
	}
	return b.String()
}

func (m chatModel) renderStreamTail(viewWidth int) string {
	hawkC := "\033[38;2;255;94;14m"
	rst := "\033[0m"

	partial := sanitizeIdentity(strings.TrimLeft(m.partial.String(), "\n\r"))
	if partial != "" {
		return hawkC + icons.Robot() + " " + rst + renderMarkdown(partial, viewWidth-3) + "\n\n"
	}
	return m.renderWaitingSpinnerLine() + "\n\n"
}

// assembleViewportContent builds scrollback using the render cache. Returns the
// full viewport string ready for SetContent.
func (m *chatModel) assembleViewportContent(viewWidth int) string {
	fullRebuild := m.vpRenderWidth != viewWidth ||
		m.vpRenderedMsgs > len(m.messages) ||
		m.vpStableContent == ""

	if !fullRebuild && m.vpRenderedMsgs < len(m.messages) {
		var b strings.Builder
		b.WriteString(m.vpStableContent)
		for i := m.vpRenderedMsgs; i < len(m.messages); i++ {
			b.WriteString(renderDisplayMessage(m.messages[i], i, m.messages, viewWidth))
		}
		m.vpStableContent = b.String()
		m.vpRenderedMsgs = len(m.messages)
	} else if !fullRebuild && m.vpRenderedMsgs == len(m.messages) && m.vpRenderedMsgs > 0 {
		last := m.messages[m.vpRenderedMsgs-1]
		if len(last.content) != m.vpLastMsgLen {
			prefix := renderMessagesRange(m.messages, 0, m.vpRenderedMsgs-1, viewWidth)
			tail := renderDisplayMessage(last, m.vpRenderedMsgs-1, m.messages, viewWidth)
			m.vpStableContent = prefix + tail
		}
	}

	if fullRebuild {
		m.vpStableContent = renderMessagesRange(m.messages, 0, len(m.messages), viewWidth)
		m.vpRenderedMsgs = len(m.messages)
		m.vpRenderWidth = viewWidth
	}

	if m.vpRenderedMsgs > 0 {
		m.vpLastMsgLen = len(m.messages[m.vpRenderedMsgs-1].content)
	} else {
		m.vpLastMsgLen = 0
	}

	content := m.vpStableContent
	if m.waiting && !m.manualCompacting {
		content += m.renderStreamTail(viewWidth)
	}
	return content
}
