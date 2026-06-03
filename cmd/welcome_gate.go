package cmd

import (
	"context"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

type uiPhase int

const (
	phaseWork uiPhase = iota
	phaseWelcomeGate
)

const workInputPlaceholder = `Try "Create a PR with these changes" (Shift+Enter for newline)`

const (
	welcomeGateMinTopPad  = 2
	welcomeGateMaxTopPad  = 8
	welcomeGateClusterGap = 1 // blank lines between status chips and footer rule
)

// initialUIPhase picks welcome gate vs work from how hawk was launched.
func initialUIPhase(hasChat bool, oneShotPrompt bool) uiPhase {
	if oneShotPrompt || hasChat {
		return phaseWork
	}
	return phaseWelcomeGate
}

func (m chatModel) onWelcomeGate() bool {
	return m.phase == phaseWelcomeGate && !m.configOpen && !m.quitting
}

func (m chatModel) gateActionHint() (primary string, needsSetup bool) {
	if hawkconfig.NeedsFirstRunSetup(context.Background()) {
		return "Press Enter to set up and start", true
	}
	return "Press Enter to start", false
}

func welcomeGateRule(width int) string {
	const ornament = " · "
	ruleW := width - 4
	if ruleW > 56 {
		ruleW = 56
	}
	if ruleW < len(ornament)+8 {
		ruleW = len(ornament) + 8
	}
	side := (ruleW - len(ornament)) / 2
	if side < 2 {
		side = 2
	}
	rule := strings.Repeat("─", side) + ornament + strings.Repeat("─", ruleW-side-len(ornament))
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(borderDim).
		Render(rule)
}

func (m chatModel) renderWelcomeGateActionRow(width int) string {
	primary, needsSetup := m.gateActionHint()
	glyph := lipgloss.NewStyle().Foreground(hawkColor).Bold(true).Inline(true).Render("↵  ")
	textStyle := lipgloss.NewStyle().Foreground(hawkColor).Bold(true).Inline(true)
	if needsSetup {
		textStyle = lipgloss.NewStyle().Foreground(warnAmber).Bold(true).Inline(true)
	}
	line := glyph + textStyle.Render(primary)
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(line)
}

func (m chatModel) renderWelcomeGateChromeRow(width int) string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	left := lipgloss.NewStyle().Foreground(statusCWDColor).Inline(true).Render("  " + shortenHomePath(cwd))
	right := lipgloss.NewStyle().Foreground(textDisabled).Inline(true).Render(quitFooterHint)
	return layoutFooterRow(left, right, width)
}

func (m chatModel) renderWelcomeGateFooter(width int) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		welcomeGateRule(width),
		m.renderWelcomeGateActionRow(width),
		"",
		m.renderWelcomeGateChromeRow(width),
	)
}

// renderWelcomeGate lays out hero + footer as one cluster (no huge gap between logo and chrome).
func (m chatModel) renderWelcomeGate(width, height int) string {
	if width < 40 {
		width = 80
	}
	if height < 10 {
		height = 24
	}

	footer := m.renderWelcomeGateFooter(width)
	footerH := lipgloss.Height(footer)

	hero := strings.Trim(m.welcomeCache, "\n")
	heroLines := strings.Split(hero, "\n")
	contentH := len(heroLines)

	clusterH := contentH + welcomeGateClusterGap + footerH
	slack := height - clusterH
	topPad := welcomeGateMinTopPad
	if slack > welcomeGateMinTopPad {
		topPad = slack / 5
	}
	if topPad > welcomeGateMaxTopPad {
		topPad = welcomeGateMaxTopPad
	}

	maxContent := height - topPad - welcomeGateClusterGap - footerH
	if maxContent < 1 {
		maxContent = 1
		topPad = 0
	}
	if contentH > maxContent {
		heroLines = heroLines[:maxContent]
		contentH = maxContent
		hero = strings.Join(heroLines, "\n")
	}

	var b strings.Builder
	if topPad > 0 {
		b.WriteString(strings.Repeat("\n", topPad))
	}
	if hero != "" {
		if !strings.HasSuffix(hero, "\n") {
			b.WriteString(hero)
		} else {
			b.WriteString(strings.TrimRight(hero, "\n"))
		}
	}
	if welcomeGateClusterGap > 0 {
		b.WriteString(strings.Repeat("\n", welcomeGateClusterGap))
	}
	b.WriteString(footer)
	return b.String()
}

// stripWelcomeMessages removes splash copy from chat scrollback after the gate.
func (m chatModel) stripWelcomeMessages() chatModel {
	filtered := m.messages[:0]
	for _, msg := range m.messages {
		if msg.role != "welcome" {
			filtered = append(filtered, msg)
		}
	}
	m.messages = filtered
	return m
}

// enterWorkPhase transitions from the welcome gate into the normal work TUI.
func (m chatModel) enterWorkPhase() (chatModel, tea.Cmd) {
	m.phase = phaseWork
	m = m.stripWelcomeMessages()
	m.viewDirty = true
	m.autoScroll = true
	m.streamFollow = true
	m.uiFocus = focusPrompt

	if m.width > 0 {
		m.input.SetWidth(m.width - 4)
	}
	m.rebuildWelcomeCache(m.blinkClosed)
	m.updateViewportContent()

	if m.sandboxReadyPending {
		m = m.flushSandboxReadyMessage()
	}

	var cmds []tea.Cmd
	cmds = append(cmds, m.input.Focus())

	if m.openConfigOnStart {
		m.openConfigOnStart = false
		cm, c := m.openConfigPanel()
		m = cm
		cmds = append(cmds, c)
	}

	return m, tea.Batch(cmds...)
}

func (m chatModel) flushSandboxReadyMessage() chatModel {
	if !m.sandboxReadyPending || m.session == nil {
		return m
	}
	m.sandboxReadyPending = false
	m.messages = append(m.messages, displayMsg{
		role:    "system",
		content: formatSandboxReadyAutonomyMessage(m.session.Autonomy),
	})
	m.viewDirty = true
	return m
}

func (m chatModel) handleWelcomeGateKey(msg tea.KeyMsg) (chatModel, tea.Cmd, bool) {
	if !m.onWelcomeGate() {
		return m, nil, false
	}

	if m.commandPalette != nil && m.commandPalette.IsOpen() {
		action, handled := m.commandPalette.Update(msg)
		if handled {
			if action != "" {
				m.commandPalette.Close()
				result, cmd := m.handleCommand(action)
				if cm, ok := result.(chatModel); ok {
					m = cm
				}
				if m.phase == phaseWork {
					m.viewDirty = true
					m.updateViewportContent()
					return m, cmd, true
				}
			}
			m.viewDirty = true
			return m, nil, true
		}
	}

	switch msg.String() {
	case "enter":
		next, cmd := m.enterWorkPhase()
		return next, cmd, true
	case "ctrl+c":
		if time.Since(m.lastCtrlC) < time.Second {
			if m.watcherStop != nil {
				m.watcherStop()
			}
			m.quitting = true
			return m, tea.Quit, true
		}
		m.lastCtrlC = time.Now()
		return m, nil, true
	case "ctrl+k":
		if m.commandPalette == nil {
			m.commandPalette = NewCommandPalette(m.width)
		}
		m.commandPalette.Open()
		m.viewDirty = true
		return m, nil, true
	}
	return m, nil, true
}
