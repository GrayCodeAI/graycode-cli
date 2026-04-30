package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	tealColor    = lipgloss.Color("#4ECDC4")
	dimColor     = lipgloss.Color("#666666")
	errorColor   = lipgloss.Color("#e05555")
	userStyle    = lipgloss.NewStyle().Foreground(tealColor).Bold(true)
	assistStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	dimStyle     = lipgloss.NewStyle().Foreground(dimColor)
	errorStyle   = lipgloss.NewStyle().Foreground(errorColor)
	headerStyle  = lipgloss.NewStyle().Foreground(tealColor).Bold(true)
)

type chatMsg struct {
	role    string
	content string
}

type errMsg struct{ err error }

type chatModel struct {
	input    textarea.Model
	spinner  spinner.Model
	messages []chatMsg
	waiting  bool
	width    int
	height   int
	err      error
	quitting bool
}

func newChatModel() chatModel {
	ta := textarea.New()
	ta.Placeholder = "Message hawk..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(tealColor)

	return chatModel{input: ta, spinner: sp}
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			if text == "/quit" || text == "/exit" {
				m.quitting = true
				return m, tea.Quit
			}
			m.messages = append(m.messages, chatMsg{role: "user", content: text})
			m.input.Reset()
			m.waiting = true
			// TODO: Phase 1 — send to eyrie LLM here
			m.messages = append(m.messages, chatMsg{role: "assistant", content: "🚧 LLM not wired yet — eyrie integration coming in Phase 1."})
			m.waiting = false
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(msg.Width - 2)

	case errMsg:
		m.err = msg.err
		m.waiting = false
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m chatModel) View() string {
	if m.quitting {
		return dimStyle.Render("Goodbye.") + "\n"
	}

	var b strings.Builder

	// Header
	b.WriteString(headerStyle.Render(fmt.Sprintf("🦅 hawk v%s", version)))
	b.WriteString(dimStyle.Render("  (ctrl+c to quit)"))
	b.WriteString("\n\n")

	// Messages
	for _, msg := range m.messages {
		switch msg.role {
		case "user":
			b.WriteString(userStyle.Render("You: "))
			b.WriteString(msg.content)
		case "assistant":
			b.WriteString(assistStyle.Render("hawk: "))
			b.WriteString(msg.content)
		case "error":
			b.WriteString(errorStyle.Render("error: "))
			b.WriteString(msg.content)
		}
		b.WriteString("\n\n")
	}

	// Spinner
	if m.waiting {
		b.WriteString(m.spinner.View() + " Thinking...\n\n")
	}

	// Error
	if m.err != nil {
		b.WriteString(errorStyle.Render("Error: "+m.err.Error()) + "\n\n")
	}

	// Input
	b.WriteString(m.input.View())
	b.WriteString("\n")

	return b.String()
}

func runChat() error {
	p := tea.NewProgram(newChatModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
