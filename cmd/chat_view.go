package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

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

// reBold matches markdown **bold** syntax.
var reBold = regexp.MustCompile(`\*\*(.+?)\*\*`)

// renderInlineMarkdown converts inline markdown to ANSI terminal formatting.
// Currently handles **bold** → ANSI bold.
func renderInlineMarkdown(s string) string {
	return reBold.ReplaceAllString(s, "\033[1m${1}\033[22m")
}

// wrapText wraps text to fit within width columns total (including indent).
// The first line has no indent (caller provides the prefix).
// Continuation lines get indent prepended.
// wrapText wraps text to fit within the given width.
// prefixWidth is the visual width of the prefix already printed before the first line
// (e.g. "⛬ " = 2 columns). Continuation lines are indented to align with the first
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

func (m *chatModel) hasRealMessages() bool {
	for _, msg := range m.messages {
		if msg.role != "welcome" {
			return true
		}
	}
	return m.waiting
}

func (m *chatModel) updateViewportContent() {
	viewWidth := m.width
	if viewWidth <= 0 {
		viewWidth = 80
	}

	// Always recalculate viewport height to track input box size changes
	bottomBarLines := 0
	if !m.configOpen {
		inputLines := strings.Count(m.input.Value(), "\n") + 1
		if inputLines > 10 {
			inputLines = 10
		}
		// status(1) + border-top(1) + input(N) + border-bottom(1) + help(1) + newline-separator(1)
		bottomBarLines = 1 + 2 + inputLines + 1 + 1
		// Account for slash suggestion menu
		if sugs := slashSuggestions(m.input.Value()); len(sugs) > 0 {
			visible := len(sugs)
			if visible > 6 {
				visible = 6
			}
			bottomBarLines += visible
		}
	}
	newVPHeight := m.height - bottomBarLines
	if newVPHeight < 4 {
		newVPHeight = 4
	}
	if m.viewport.Height != newVPHeight {
		m.viewport.Height = newVPHeight
		m.viewport.Width = viewWidth
	}

	if !m.viewDirty {
		return
	}
	m.viewDirty = false

	hawkC := "\033[38;2;255;94;14m"
	rst := "\033[0m"
	bgDark := "\033[48;2;30;30;40m"

	var chatContent strings.Builder
	chatContent.WriteString(m.welcomeCache + "\n")

	for i, msg := range m.messages {
		switch msg.role {
		case "user":
			if i > 0 {
				chatContent.WriteString("\n")
			}
			wrapped := wrapText(msg.content, viewWidth-1, 3)
			wrappedLines := strings.Split(wrapped, "\n")
			for li, wl := range wrappedLines {
				if li == 0 {
					chatContent.WriteString(bgDark + hawkC + "█" + rst + bgDark + "  " + wl)
				} else {
					chatContent.WriteString(bgDark + "   " + wl)
				}
				// Pad to full width for consistent background
				visW := 3 + visibleWidth(wl)
				if pad := viewWidth - visW; pad > 0 {
					chatContent.WriteString(strings.Repeat(" ", pad))
				}
				chatContent.WriteString(rst)
				if li < len(wrappedLines)-1 {
					chatContent.WriteByte('\n')
				}
			}
		case "assistant":
			content := strings.TrimLeft(msg.content, "\n\r")
			chatContent.WriteString(hawkC + "⛬ " + rst + renderMarkdown(content, viewWidth-3))
		case "tool_use":
			chatContent.WriteString(toolStyle.Render("⚡ " + msg.content))
		case "tool_result":
			toolWrapped := wrapText(msg.content, viewWidth-6, 0)
			chatContent.WriteString(toolDimStyle.Render("    " + strings.ReplaceAll(toolWrapped, "\n", "\n    ")))
		case "thinking":
			thinkWrapped := wrapText(msg.content, viewWidth-4, 3)
			chatContent.WriteString(dimStyle.Render("💭 " + thinkWrapped))
		case "welcome":
			// Skip welcome in viewport — it's rendered statically in View()
		case "system":
			sysWrapped := wrapText(msg.content, viewWidth-2, 0)
			chatContent.WriteString(dimStyle.Render(sysWrapped))
		case "permission":
			chatContent.WriteString(renderPermissionBox(msg.content, viewWidth))
		case "question":
			qWrapped := wrapText(msg.content, viewWidth-2, 2)
			chatContent.WriteString(toolStyle.Render(qWrapped))
		case "usage":
			chatContent.WriteString(dimStyle.Render("  " + msg.content))
		case "error":
			errWrapped := wrapText(msg.content, viewWidth-8, 7)
			chatContent.WriteString(errorStyle.Render("error: " + errWrapped))
		}
		// Tighter spacing between tool_use → tool_result pairs
		if msg.role == "tool_use" && i+1 < len(m.messages) && m.messages[i+1].role == "tool_result" {
			chatContent.WriteByte('\n')
		} else if msg.role == "tool_result" && i+1 < len(m.messages) && m.messages[i+1].role == "tool_use" {
			chatContent.WriteByte('\n')
		} else if msg.role == "usage" {
			chatContent.WriteByte('\n')
		} else {
			chatContent.WriteString("\n\n")
		}
	}

	if m.waiting {
		partial := sanitizeIdentity(strings.TrimLeft(m.partial.String(), "\n\r"))
		if partial != "" {
			chatContent.WriteString(hawkC + "⛬ " + rst + renderMarkdown(partial, viewWidth-3))
			chatContent.WriteString("\n\n")
		} else {
			spinnerLine := m.spinner.View() + "  " + renderGlimmerVerb(m.spinnerVerb, m.glimmerPos) + "\033[1;38;2;255;94;14m...\033[0m"
			if !m.toolStartTime.IsZero() {
				if elapsed := time.Since(m.toolStartTime); elapsed > 2*time.Second {
					spinnerLine += fmt.Sprintf(" (%.1fs)", elapsed.Seconds())
				}
			}
			spinnerLine += " " + dimStyle.Render("(Press ESC to stop)")
			chatContent.WriteString(spinnerLine + "\n\n")
		}
	}

	if m.configOpen {
		chatContent.WriteString(m.configPanelView())
		chatContent.WriteString("\n\n")
	}

	atBottom := m.viewport.AtBottom()
	contentStr := chatContent.String()

	m.viewport.SetContent(contentStr)
	if atBottom || m.autoScroll {
		m.viewport.GotoBottom()
	}
}

func (m chatModel) View() string {
	if m.quitting {
		return ""
	}

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
	bottomBarLines := 0

	if !m.configOpen {
		totalW := viewWidth
		if totalW < 40 {
			totalW = 80
		}
		var leftBold, leftDim string
		if m.containerEnabled && m.containerReady {
			leftBold = "Container"
			leftDim = " - no approval needed"
		} else if m.containerEnabled && m.containerErr != nil {
			leftBold = "Container"
			leftDim = " - Docker is not running. Start Docker and try again."
		} else if m.containerEnabled {
			leftBold = "Container"
			leftDim = " - " + m.containerStatus
		} else {
			leftBold = permissionModeLabel(m.session)
			leftDim = permissionModeHint(m.session)
		}
		rightStatus := fmt.Sprintf("%s %s", m.session.Provider(), m.session.Model())
		leftVisLen := len(leftBold) + len(leftDim)
		gap := totalW - leftVisLen - len(rightStatus)
		if gap < 1 {
			gap = 1
		}
		var leftRendered string
		if m.containerEnabled && m.containerErr != nil {
			redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
			leftRendered = redStyle.Bold(true).Render(leftBold) + redStyle.Render(leftDim)
		} else {
			leftRendered = lipgloss.NewStyle().Bold(true).Render(leftBold) + dimStyle.Render(leftDim)
		}
		bottomBar.WriteString(leftRendered + strings.Repeat(" ", gap) + dimStyle.Render(rightStatus) + "\n")
		bottomBarLines++
		inputBox := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, true, false).
			BorderForeground(lipgloss.Color("#555555")).
			Width(totalW).
			Render(func() string {
				if m.useConfigInput {
					return m.configInput.View()
				}
				return m.input.View()
			}())
		bottomBar.WriteString(inputBox + "\n")
		// borders(2) + input content lines
		inputLines := strings.Count(m.input.Value(), "\n") + 1
		if inputLines > 10 {
			inputLines = 10
		}
		bottomBarLines += 2 + inputLines
		if sugs := slashSuggestions(m.input.Value()); len(sugs) > 0 {
			if m.slashSel < 0 || m.slashSel >= len(sugs) {
				m.slashSel = 0
			}
			cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#73767E"))
			descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#73767E"))
			selCmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
			selDescStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E"))
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
				bottomBarLines++
			}
		}
		if m.containerEnabled && m.containerStatus != "" {
			style := dimStyle
			text := "container: " + m.containerStatus
			if m.containerErr != nil {
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
				text = "Docker is not running. Start Docker and try again."
			}
			bottomBar.WriteString(style.Render(text) + "\n")
		}
		_ = bottomBarLines
	}

	return m.viewport.View() + "\n" + bottomBar.String()
}

func formatDiff(diff string) string {
	var b strings.Builder
	lineNum := 0
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ "):
			// File headers
			b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(line))
		case strings.HasPrefix(line, "@@"):
			// Hunk headers — extract line numbers
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(line))
			// Parse starting line from @@ -X,Y +Z,W @@
			if idx := strings.Index(line, "+"); idx >= 0 {
				fmt.Sscanf(line[idx:], "+%d", &lineNum)
				if lineNum > 0 {
					lineNum-- // will be incremented on first content line
				}
			}
		case strings.HasPrefix(line, "+"):
			lineNum++
			num := fmt.Sprintf("%4d ", lineNum)
			b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(num))
			b.WriteString(diffAddStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render("     "))
			b.WriteString(diffDelStyle.Render(line))
		default:
			lineNum++
			num := fmt.Sprintf("%4d ", lineNum)
			b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(num))
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderPermissionBox renders a visually distinct permission prompt box.
func renderPermissionBox(summary string, width int) string {
	boxW := width - 4
	if boxW < 40 {
		boxW = 40
	}
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FFD700")).
		Width(boxW).
		Padding(0, 1)

	title := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Bold(true).Render("⚠ Permission Required")
	body := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Render(summary)
	options := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ECDC4")).Render("[y]es  [n]o  [a]lways")

	return border.Render(title + "\n" + body + "\n" + options)
}
