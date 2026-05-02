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
			if curWidth > 0 && curWidth+1+wordW > maxW {
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
	if !m.viewDirty {
		return
	}
	m.viewDirty = false

	viewWidth := m.width
	if viewWidth <= 0 {
		viewWidth = 80
	}

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
			wrapped := wrapText(msg.content, viewWidth, 3)
			line := hawkC + "█" + rst + "  " + wrapped
			chatContent.WriteString(bgDark + line + rst)
		case "assistant":
			content := strings.TrimLeft(msg.content, "\n\r")
			chatContent.WriteString(hawkC + "⛬ " + rst + renderMarkdown(content, viewWidth-3))
		case "tool_use":
			chatContent.WriteString(toolStyle.Render("⚡ " + msg.content))
		case "tool_result":
			chatContent.WriteString(toolDimStyle.Render(msg.content))
		case "thinking":
			chatContent.WriteString(dimStyle.Render("💭 " + msg.content))
		case "welcome":
			// Skip welcome in viewport — it's rendered statically in View()
		case "system":
			chatContent.WriteString(dimStyle.Render(msg.content))
		case "permission":
			chatContent.WriteString(toolStyle.Render("⚠ " + msg.content + "  [y/n]"))
		case "question":
			chatContent.WriteString(toolStyle.Render(msg.content))
		case "usage":
		case "error":
			chatContent.WriteString(errorStyle.Render("error: " + msg.content))
		}
		chatContent.WriteString("\n\n")
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

	// Calculate bottom bar height to size viewport correctly
	bottomBarLines := 0
	if !m.configOpen {
		bottomBarLines = 5 // status(1) + input borders+content(3) + help(1)
	}
	vpHeight := m.height - bottomBarLines - 1
	if vpHeight < 4 {
		vpHeight = 4
	}
	m.viewport.Width = viewWidth
	m.viewport.Height = vpHeight

	atBottom := m.viewport.AtBottom()
	contentStr := chatContent.String()

	contentLines := strings.Count(contentStr, "\n") + 1
	if contentLines < vpHeight {
		m.viewport.Height = contentLines
	}

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
		leftBold := "Auto (Off)"
		leftDim := " - all actions require approval"
		rightStatus := fmt.Sprintf("%s %s", m.session.Provider(), m.session.Model())
		leftVisLen := len(leftBold) + len(leftDim)
		gap := totalW - leftVisLen - len(rightStatus)
		if gap < 1 {
			gap = 1
		}
		leftRendered := lipgloss.NewStyle().Bold(true).Render(leftBold) + dimStyle.Render(leftDim)
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
		bottomBarLines += 3
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
		bottomBar.WriteString(dimStyle.Render("? for help") + "\n")
		bottomBarLines++
	}

	return m.viewport.View() + "\n" + bottomBar.String()
}

func formatDiff(diff string) string {
	var b strings.Builder
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+"):
			b.WriteString(diffAddStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			b.WriteString(diffDelStyle.Render(line))
		default:
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
