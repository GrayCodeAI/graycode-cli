package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/home"
)

// UI focus areas — Grok Build cycles Prompt ↔ Scrollback (Tab).
type uiFocusArea int

const (
	focusPrompt uiFocusArea = iota
	focusScrollback
)

const toolDisplayMaxChars = 12000 // preview cap in scrollback (Grok uses ~20k for bash)

func sessionExportPath(sessionID string) string {
	return filepath.Join(home.Dir(), ".hawk", "exports", sessionID+".md")
}

func (m *chatModel) inScrollbackFocus() bool {
	return m.uiFocus == focusScrollback && !m.configOpen
}

func (m *chatModel) cycleUIFocus() (chatModel, tea.Cmd) {
	if m.inScrollbackFocus() {
		m.uiFocus = focusPrompt
		m.viewDirty = true
		return *m, m.input.Focus()
	}
	m.uiFocus = focusScrollback
	m.autoScroll = false
	m.streamFollow = false
	m.input.Blur()
	m.viewDirty = true
	return *m, nil
}

func (m *chatModel) goHome() {
	m.rebuildWelcomeCache(false)
	hasWelcome := false
	for _, msg := range m.messages {
		if msg.role == "welcome" {
			hasWelcome = true
			break
		}
	}
	if !hasWelcome {
		m.messages = append([]displayMsg{{role: "welcome", content: m.welcomeCache}}, m.messages...)
	}
	m.uiFocus = focusScrollback
	m.autoScroll = false
	m.streamFollow = false
	m.viewport.SetYOffset(0)
	m.viewDirty = true
}

func (m chatModel) scrollPositionLabel() string {
	if m.contentLines <= 0 {
		return ""
	}
	visH := m.viewport.Height
	if visH <= 0 {
		visH = 1
	}
	if m.contentLines <= visH {
		return fmt.Sprintf("1-%d/%d", m.contentLines, m.contentLines)
	}
	top := m.viewport.YOffset + 1
	bottom := m.viewport.YOffset + visH
	if bottom > m.contentLines {
		bottom = m.contentLines
	}
	return fmt.Sprintf("%d-%d/%d", top, bottom, m.contentLines)
}

func (m chatModel) uiFocusLabel() string {
	if m.inScrollbackFocus() {
		return "scrollback"
	}
	return "prompt"
}

func formatSessionContextUsage(m *chatModel) string {
	if m == nil || m.session == nil {
		return "No active session."
	}
	window := m.contextWindowTokens()
	used := sessionContextUsedTokens(m.session)
	pct := contextFillPercent(used, window)
	compactAt := int(float64(window) * float64(engine.DefaultAutoCompactThresholdPct) / 100)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Context: %s / %s (%s)\n",
		formatContextUsedLabel(used), formatModelTableContext(window), formatContextPercentLabel(used, window)))
	b.WriteString(fmt.Sprintf("Auto-compact at %d%% (~%s tokens)\n",
		engine.DefaultAutoCompactThresholdPct, formatContextUsedLabel(compactAt)))
	if m.streamFollow {
		b.WriteString("Stream follow: on (scroll up to freeze; /follow off)\n")
	} else {
		b.WriteString("Stream follow: off (/follow on)\n")
	}
	b.WriteString("Tab: prompt ↔ scrollback · /home: welcome · /export: save transcript")
	if pct >= engine.DefaultAutoCompactThresholdPct {
		b.WriteString("\n⚠ Approaching auto-compact threshold — consider /compact")
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatToolResultDisplay(content string) string {
	if len(content) <= toolDisplayMaxChars {
		return content
	}
	if len(content) <= toolDisplayMaxChars*2 {
		omit := len(content) - toolDisplayMaxChars
		return content[:toolDisplayMaxChars] + fmt.Sprintf("\n… (%d more chars — full output in session)", omit)
	}
	half := toolDisplayMaxChars / 2
	omit := len(content) - toolDisplayMaxChars
	return content[:half] + fmt.Sprintf("\n… (%d chars omitted) …\n", omit) + content[len(content)-half:]
}

func (m chatModel) renderScrollbackFocusBar(width int) string {
	if !m.inScrollbackFocus() {
		return ""
	}
	hint := "scroll · Tab prompt · Up/Dn · PgUp/PgDn"
	if width < len(hint)+4 {
		width = len(hint) + 4
	}
	style := lipgloss.NewStyle().Foreground(infoSky).Bold(true).Inline(true)
	return style.Render("  " + hint)
}