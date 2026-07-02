package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

func (m chatModel) showWelcomeBanner() bool {
	return strings.TrimSpace(m.welcomeCache) != ""
}

func renderSetupCompleteMessage(model string) string {
	success := lipgloss.NewStyle().Foreground(doneGreen).Bold(true).Inline(true)
	muted := configMutedStyle().Inline(true)
	active := configActiveStyle().Inline(true)
	model = strings.TrimSpace(model)
	if model == "" {
		model = "selected model"
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		success.Render("Setup complete"),
		muted.Render(" · ready to chat with "),
		active.Render(model),
		muted.Render(" "),
		success.Render(icons.CheckBold()+" "),
	)
}

// sanitizeIdentity replaces model self-identifications with "hawk" / "GrayCode AI".
var (
	reModelName = regexp.MustCompile(`(?i)\b(I['` + "\u2018\u2019" + `]m|I am|my name is)\s+\*{0,2}(ChatGPT|GPT-?\d*[o]?|Claude|Gemini|Gemma|Kimi|DeepSeek|Llama|Qwen|Mistral|Mixtral|Grok|Copilot|Bard|Command R|Yi|Phi|Nova|Titan|BLOOM|Falcon|PaLM|LaMDA|Chinchilla|Vicuna|Alpaca|WizardLM|Orca|Nemotron|Granite|DBRX|OLMo|Pixtral|Ernie|PanGu|Sarvam|MiMo|GLM|Codex|Jurassic|Cohere|Jais|Step|Velvet|Alice|Apertus|Param|YandexGPT|MiniMax)\*{0,2}`)
	reCreator   = regexp.MustCompile(`(?i)(made|created|developed|built|trained|designed)\s+by\s+(?:a\s+company\s+called\s+|a\s+team\s+(?:at|called)\s+|the\s+team\s+at\s+)?\*{0,2}(Moonshot\s*AI|OpenAI|Anthropic|Google|Google\s*DeepMind|DeepMind|Meta|Meta\s*AI|Alibaba|Alibaba\s*Cloud|Mistral\s*AI|xAI|Microsoft|Microsoft\s*AI|Amazon|AWS|Cohere|01\.AI|Baidu|Huawei|IBM|Nvidia|EleutherAI|Hugging\s*Face|AI21\s*Labs|Yandex|Databricks|StepFun|Xiaomi|Sarvam\s*AI|MiniMax|BharatGen|Z\.ai|Zhipu\s*AI|Cerebras|Technology\s*Innovation\s*Institute|TII|Inflection\s*AI|Stability\s*AI|Anysphere|Cognition\s*AI|Scale\s*AI|Sakana\s*AI)\*{0,2}`)
)

func sanitizeIdentity(s string) string {
	s = reModelName.ReplaceAllStringFunc(s, func(m string) string {
		parts := reModelName.FindStringSubmatch(m)
		return parts[1] + " hawk"
	})
	s = reCreator.ReplaceAllString(s, "${1} by GrayCode AI")
	return s
}

// wrapText wraps text to fit within width columns total (including indent).
// The first line has no indent (caller provides the prefix).
// Continuation lines get indent prepended.
// wrapText wraps text to fit within the given width.
// prefixWidth is the visual width of the prefix already printed before the first line
// (e.g. icons.Robot() + " " = 2 columns). Continuation lines are indented to align with the first
// line's text start position.
// width is the total terminal width available.
func wrapText(text string, width int, prefixWidth int) string {
	if width < 20 {
		width = 80
	}
	// First line has less room because the prefix is already printed.
	firstLineWidth := width - prefixWidth
	if firstLineWidth < 10 {
		firstLineWidth = width
	}
	// Continuation indent: spaces to align under the first line's text.
	indent := strings.Repeat(" ", prefixWidth)
	indentW := prefixWidth
	contWidth := width - indentW
	if contWidth < 10 {
		contWidth = width
	}
	var result strings.Builder
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		isFirst := (i == 0)
		if !isFirst {
			result.WriteString(indent)
		}

		// Detect leading whitespace on this line so wrapped continuations
		// stay aligned with the original content (e.g. indented bullet lists).
		trimmed := strings.TrimLeft(line, " \t")
		lineLeading := line[:len(line)-len(trimmed)]
		lineLeadingW := runewidth.StringWidth(lineLeading)

		// For bullet-style lines ("* ", "- ", "N. "), add extra indent so
		// continuation text aligns past the bullet marker.
		bulletExtra := ""
		if len(trimmed) > 0 {
			if strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "+ ") {
				bulletExtra = "  "
			} else if idx := strings.Index(trimmed, ". "); idx > 0 && idx <= 3 {
				allDigits := true
				for _, ch := range trimmed[:idx] {
					if ch < '0' || ch > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					bulletExtra = strings.Repeat(" ", idx+2)
				}
			}
		}

		// Build the continuation indent for wrapped segments of this line.
		lineIndent := indent + lineLeading + bulletExtra
		lineIndentW := indentW + lineLeadingW + runewidth.StringWidth(bulletExtra)
		lineContWidth := width - lineIndentW
		if lineContWidth < 10 {
			lineContWidth = contWidth
			lineIndent = indent
		}

		maxW := firstLineWidth
		if !isFirst {
			maxW = contWidth - lineLeadingW // account for leading whitespace already written
		}
		if runewidth.StringWidth(line) <= maxW {
			result.WriteString(line)
			result.WriteByte('\n')
			continue
		}
		curWidth := 0
		var curLine strings.Builder
		for _, word := range strings.Fields(line) {
			wordW := runewidth.StringWidth(word)
			// Force-break words longer than available width
			if wordW > maxW && curWidth == 0 {
				runes := []rune(word)
				for len(runes) > 0 {
					chunk := 0
					chunkW := 0
					for chunk < len(runes) && chunkW+runewidth.RuneWidth(runes[chunk]) <= maxW {
						chunkW += runewidth.RuneWidth(runes[chunk])
						chunk++
					}
					if chunk == 0 {
						chunk = 1
						chunkW = runewidth.RuneWidth(runes[0])
					}
					if curLine.Len() > 0 {
						result.WriteString(curLine.String())
						result.WriteByte('\n')
						result.WriteString(lineIndent)
						curLine.Reset()
						maxW = lineContWidth
					}
					curLine.WriteString(string(runes[:chunk]))
					curWidth = chunkW
					runes = runes[chunk:]
					if len(runes) > 0 {
						result.WriteString(curLine.String())
						result.WriteByte('\n')
						result.WriteString(lineIndent)
						curLine.Reset()
						curWidth = 0
						maxW = lineContWidth
					}
				}
			} else if curWidth > 0 && curWidth+1+wordW > maxW {
				result.WriteString(curLine.String())
				result.WriteByte('\n')
				result.WriteString(lineIndent)
				curLine.Reset()
				curLine.WriteString(word)
				curWidth = wordW
				maxW = lineContWidth
			} else if curWidth > 0 {
				curLine.WriteByte(' ')
				curLine.WriteString(word)
				curWidth += 1 + wordW
			} else {
				curLine.WriteString(word)
				curWidth = wordW
			}
		}
		if curLine.Len() > 0 {
			result.WriteString(curLine.String())
			result.WriteByte('\n')
		}
	}
	return strings.TrimRight(result.String(), "\n")
}

// chatBottomBarLines counts fixed rows below the chat viewport (must stay in sync with View).
func (m chatModel) chatBottomBarLines() int {
	if m.configOpen {
		return 0
	}
	if m.cachedBottomBarLines > 0 {
		return m.cachedBottomBarLines
	}
	return m.computeChatBottomBarLines()
}

func (m chatModel) computeChatBottomBarLines() int {
	footerW := m.width
	if footerW < 40 {
		footerW = 80
	}
	inputBoxLines := m.measureInputBoxLines(footerW)
	lines := 1 + inputBoxLines // container/model row + input box (measured)
	if m.ghostText != nil {
		if ghost := m.ghostText.Get(); ghost != "" && m.input.Value() == "" {
			lines++
		}
	}
	lines += m.visibleSlashSuggestionLines()
	lines++ // session stats row (always shown — tokens · cost · duration)
	if m.manualCompacting {
		lines += 2 // "Compacting conversation..." + progress bar
	}
	if m.inScrollbackFocus() {
		lines++ // scrollback focus hint
	}
	return lines
}

func (m *chatModel) updateViewportContent() {
	viewWidth := m.width
	if viewWidth <= 0 {
		viewWidth = 80
	}

	*m = m.withSyncedLayout()

	if !m.viewDirty {
		return
	}
	m.viewDirty = false

	// /config overlay: config panel only.
	if m.configOpen {
		var content strings.Builder
		content.WriteString(m.configPanelView())
		m.viewport.SetContent(content.String())
		return
	}

	atBottom := m.viewport.AtBottom()
	preserveScroll := !m.autoScroll && !atBottom
	prevYOffset := m.viewport.YOffset
	contentStr := m.assembleViewportContent(viewWidth)
	m.contentLines = strings.Count(contentStr, "\n") + 1
	if m.contentLines < 1 {
		m.contentLines = 1
	}

	m.viewport.SetContent(contentStr)
	m.viewport.Width = m.chatViewportWidth(viewWidth)
	switch {
	case preserveScroll:
		m.viewport.SetYOffset(prevYOffset)
	case atBottom || (m.autoScroll && m.streamFollow):
		m.viewport.GotoBottom()
	}
}

func (m chatModel) View() string {
	if m.quitting {
		return ""
	}
	m = m.withSyncedLayout()

	viewWidth := m.width
	if viewWidth <= 0 {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
			viewWidth = w
		} else {
			viewWidth = 80
		}
	}

	// Build the fixed bottom bar
	var bottomBar strings.Builder

	if !m.configOpen {
		totalW := viewWidth
		if totalW < 40 {
			totalW = 80
		}
		slashOpen := m.slashMenuOpen()
		footerW := m.footerContentWidth(totalW)
		leftRendered := renderContainerFooterLeft(m)
		modelRendered, _, ctxRendered, ctxVisLen := m.renderConnectionStatusSplit()
		rightLine := modelRendered
		if ctxVisLen > 0 {
			if modelRendered != "" {
				rightLine += configMutedStyle().Inline(true).Render(" · ")
			}
			rightLine += ctxRendered
		}
		topRow := layoutFooterRow(leftRendered, rightLine, footerW)
		bottomBar.WriteString(m.finishFooterLine(topRow, totalW) + "\n")
		if m.manualCompacting {
			compactLine := clipFooterLine(m.renderCompactProgressPanel(footerW), footerW)
			bottomBar.WriteString(m.finishFooterLine(compactLine, totalW) + "\n")
		}
		if bar := m.renderScrollbackFocusBar(footerW); bar != "" {
			bottomBar.WriteString(m.finishFooterLine(bar, totalW) + "\n")
		}
		inputBox := inputBorderStyle.Width(footerW).Render(func() string {
			if m.useConfigInput {
				return m.configInput.View()
			}
			return m.input.View()
		}())
		inputBox = clipRenderedBlock(inputBox, footerW)
		bottomBar.WriteString(inputBox + "\n")
		if m.ghostText != nil {
			if ghost := m.ghostText.Get(); ghost != "" && m.input.Value() == "" {
				ghostLine := ghostHintStyle.Render("  → " + ghost + " (Tab to accept)")
				bottomBar.WriteString(m.finishFooterLine(ghostLine, totalW) + "\n")
			}
		}
		if slashOpen {
			if sugs := m.slashSuggestionsFor(m.input.Value()); len(sugs) > 0 {
				if m.slashSel < 0 || m.slashSel >= len(sugs) {
					m.slashSel = 0
				}
				cmdStyle := slashCmdStyle
				descStyle := slashDescStyle
				selCmdStyle := slashSelCmdStyle
				selDescStyle := slashSelDescStyle
				maxVisible := 6
				start := 0
				if m.slashSel >= maxVisible {
					start = m.slashSel - maxVisible + 1
				}
				end := start + maxVisible
				if end > len(sugs) {
					end = len(sugs)
				}
				for i := start; i < end; i++ {
					s := sugs[i]
					cmdPart := s
					descPart := ""
					if fields := strings.SplitN(s, "  ", 2); len(fields) == 2 {
						cmdPart = fields[0]
						descPart = fields[1]
					}
					pad := 20 - runewidth.StringWidth(cmdPart)
					if pad < 2 {
						pad = 2
					}
					if i == m.slashSel {
						bottomBar.WriteString("  " + selCmdStyle.Render(cmdPart) + strings.Repeat(" ", pad) + selDescStyle.Render(descPart) + "\n")
					} else {
						bottomBar.WriteString("  " + cmdStyle.Render(cmdPart) + strings.Repeat(" ", pad) + descStyle.Render(descPart) + "\n")
					}
				}
			}
		}
		stats := renderStatusBar(&m, footerW)
		bottomBar.WriteString(m.finishFooterLine(stats, totalW) + "\n")
	}

	var frame strings.Builder
	frame.WriteString(m.renderChatPane())

	// Command palette overlay
	if m.commandPalette != nil && m.commandPalette.IsOpen() {
		paletteView := m.commandPalette.Render(viewWidth)
		frame.WriteByte('\n')
		frame.WriteString(paletteView)
		return frame.String()
	}

	// Agent Status HUD overlay
	if m.hudOpen {
		hudView := renderAgentStatusPanel(m.hudData, viewWidth)
		frame.WriteByte('\n')
		frame.WriteString(hudView)
		return frame.String()
	}

	frame.WriteByte('\n')
	frame.WriteString(bottomBar.String())
	return frame.String()
}

// renderPermissionBox renders a visually distinct permission prompt box.
func renderPermissionBox(summary string, width int) string {
	boxW := width - 4
	if boxW < 40 {
		boxW = 40
	}
	// Permission dialog: amber border + amber title (warning palette,
	// distinct from tool gold which is the gold for tool names). White
	// body so the user can read the summary. Brand-orange options to
	// match the prompt/cursor voice.
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(warnAmber).
		Width(boxW).
		Padding(0, 1)

	title := lipgloss.NewStyle().Foreground(warnAmber).Bold(true).Render(icons.Alert() + " Permission Required")
	body := lipgloss.NewStyle().Foreground(textWhite).Render(summary)
	options := lipgloss.NewStyle().Foreground(hawkColor).Render("[y]es  [n]o  [a]lways")

	return border.Render(title + "\n" + body + "\n" + options)
}

// renderDiffSummary renders a diff summary line with colored +/- indicators.
func renderDiffSummary(diffLine string, width int) string {
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46"))  // green
	delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red
	fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Italic(true)

	// Parse "diff <file>: +N -N lines"
	parts := strings.SplitN(diffLine, ":", 2)
	if len(parts) != 2 {
		return dimStyle.Render(diffLine)
	}

	filePart := strings.TrimSpace(parts[0])  // "diff <file>"
	statsPart := strings.TrimSpace(parts[1]) // "+N -N lines"

	// Color the +/- numbers
	styled := statsPart
	styled = strings.ReplaceAll(styled, "+", addStyle.Render("+"))
	styled = strings.ReplaceAll(styled, "-", delStyle.Render("-"))

	return fileStyle.Render(filePart) + ": " + styled
}

// renderReflectionBox renders a self-reflection in a distinct styled box.
func renderReflectionBox(reflection string, width int) string {
	boxW := width - 2
	if boxW < 40 {
		boxW = 40
	}

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true) // orange
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)  // blue
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))          // light gray

	var b strings.Builder
	lines := strings.Split(reflection, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			b.WriteString(titleStyle.Render(line) + "\n")
		} else if strings.HasPrefix(line, "**") && strings.Contains(line, ":**") {
			// "**What failed:** ..." format
			colonIdx := strings.Index(line, ":**")
			if colonIdx >= 0 {
				label := line[:colonIdx+3]
				rest := line[colonIdx+3:]
				b.WriteString(labelStyle.Render(label) + contentStyle.Render(rest) + "\n")
			} else {
				b.WriteString(contentStyle.Render(line) + "\n")
			}
		} else {
			b.WriteString(contentStyle.Render(line) + "\n")
		}
	}

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("214")).
		Width(boxW).
		Padding(0, 1)

	return border.Render(strings.TrimRight(b.String(), "\n"))
}

// renderWaitingSpinnerLine is the live status strip while the model works.
func (m chatModel) renderWaitingSpinnerLine() string {
	sep := ansiDim + " " + icons.CircleOutline() + " " + ansiReset

	var b strings.Builder
	b.WriteString(m.brailleSpinner.Frame())
	b.WriteString(sep)
	b.WriteString(ansiTeal + fmt.Sprintf("%.1fs", m.spinnerElapsed().Seconds()) + ansiReset)
	b.WriteString(sep)
	b.WriteString(m.renderTokenCounters())
	b.WriteString(" ")
	b.WriteString(ansiDim + "(esc stop)" + ansiReset)
	return b.String()
}

// renderTokenCounters formats the live per-turn token counters that ride
// next to the spinner. Uses ↑ for input (prompt) and ↓ for output
// (completion) tokens. The displayed numbers are lerped each render
// frame toward the engine's actual values (factor 0.10) so the counter
// slides smoothly instead of jumping when a usage event arrives
// mid-stream.
//
// Both arrows are always rendered (even at 0). ↓ is magenta (live model
// output) and ↑ is cyan (session context) — each purpose its own hue,
// both at the same bright intensity as the rest of the line.
func (m *chatModel) renderTokenCounters() string {
	inTok := int(m.displayInTok + 0.5)
	outTok := int(m.displayOutTok + 0.5)

	var b strings.Builder
	b.WriteString(ansiMagenta + ansiBold + "↓" + ansiReset)
	b.WriteString(ansiMagenta)
	b.WriteString(formatHawkTokenCount(outTok))
	b.WriteString(ansiReset)
	b.WriteString(ansiDim + "  " + ansiReset)
	b.WriteString(ansiCyan + ansiBold + "↑" + ansiReset)
	b.WriteString(ansiCyan)
	b.WriteString(formatHawkTokenCount(inTok))
	b.WriteString(ansiReset)
	return b.String()
}

// spinnerElapsed returns how long the spinner has been running. Tool
// start time wins (so per-tool elapsed resets between tools), otherwise
// we fall back to the moment the current turn started. Lazy-initialized
// on first render so every submit path gets a valid elapsed time without
// having to remember to set it explicitly.
func (m *chatModel) spinnerElapsed() time.Duration {
	if !m.toolStartTime.IsZero() {
		return time.Since(m.toolStartTime)
	}
	if m.startedAt.IsZero() {
		m.startedAt = time.Now()
	}
	return time.Since(m.startedAt)
}

// tokenInputTarget is the target value the input-token display lerps
// toward. Uses the engine's reported number when available, else the
// session context estimate (real measurement of session state).
func (m *chatModel) tokenInputTarget() int {
	if m.turnInputTokens > 0 {
		return m.turnInputTokens
	}
	return sessionContextUsedTokens(m.session)
}

// tokenOutputTarget is the target value the output-token display lerps
// toward. Uses the engine's reported number when available, else a live
// rune count of the streamed partial / 4.
func (m *chatModel) tokenOutputTarget() int {
	if m.turnOutputTokens > 0 {
		return m.turnOutputTokens
	}
	return utf8.RuneCountInString(m.partial.String()) / 4
}

// formatHawkTokenCount renders a token count in hawk's compact form:
// ≥1m  → "1.5m", ≥10k → "150k", else raw digits.
func formatHawkTokenCount(tokens int) string {
	if tokens <= 0 {
		return "0"
	}
	switch {
	case tokens >= 1_000_000:
		v := float64(tokens) / 1_000_000
		if v == float64(int(v)) {
			return fmt.Sprintf("%dm", int(v))
		}
		return fmt.Sprintf("%.1fm", v)
	case tokens >= 10_000:
		return fmt.Sprintf("%dk", tokens/1000)
	default:
		return fmt.Sprintf("%d", tokens)
	}
}
