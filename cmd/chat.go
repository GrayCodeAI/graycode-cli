package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/GrayCodeAI/hawk/engine"
	"github.com/GrayCodeAI/hawk/prompt"
)

var (
	tealColor   = lipgloss.Color("#4ECDC4")
	dimColor    = lipgloss.Color("#666666")
	errorColor  = lipgloss.Color("#e05555")
	userStyle   = lipgloss.NewStyle().Foreground(tealColor).Bold(true)
	assistStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	dimStyle    = lipgloss.NewStyle().Foreground(dimColor)
	errorStyle  = lipgloss.NewStyle().Foreground(errorColor)
	headerStyle = lipgloss.NewStyle().Foreground(tealColor).Bold(true)
)

type streamChunkMsg string
type streamDoneMsg struct{}
type streamErrMsg struct{ err error }

// progRef holds a reference to the tea.Program, set after creation.
type progRef struct {
	mu sync.Mutex
	p  *tea.Program
}

func (r *progRef) Set(p *tea.Program) { r.mu.Lock(); r.p = p; r.mu.Unlock() }
func (r *progRef) Send(msg tea.Msg)   { r.mu.Lock(); p := r.p; r.mu.Unlock(); if p != nil { p.Send(msg) } }

type displayMsg struct {
	role    string
	content string
}

type chatModel struct {
	input    textarea.Model
	spinner  spinner.Model
	session  *engine.Session
	ref      *progRef
	messages []displayMsg
	partial  strings.Builder
	waiting  bool
	width    int
	height   int
	quitting bool
}

func newChatModel(ref *progRef) chatModel {
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

	sess := engine.NewSession(provider, model, prompt.System())

	return chatModel{input: ta, spinner: sp, session: sess, ref: ref}
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.waiting {
			if msg.Type == tea.KeyCtrlC {
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}
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
			m.messages = append(m.messages, displayMsg{role: "user", content: text})
			m.session.AddUser(text)
			m.input.Reset()
			m.waiting = true
			m.partial.Reset()
			m.startStream()
			return m, nil
		}

	case streamChunkMsg:
		m.partial.WriteString(string(msg))
		return m, nil

	case streamDoneMsg:
		content := m.partial.String()
		if content != "" {
			m.messages = append(m.messages, displayMsg{role: "assistant", content: content})
			m.session.AddAssistant(content)
		}
		m.partial.Reset()
		m.waiting = false
		m.input.Focus()
		return m, nil

	case streamErrMsg:
		m.messages = append(m.messages, displayMsg{role: "error", content: msg.err.Error()})
		m.partial.Reset()
		m.waiting = false
		m.input.Focus()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(msg.Width - 2)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	if !m.waiting {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *chatModel) startStream() {
	sess := m.session
	ref := m.ref
	go func() {
		ch, err := sess.Stream(context.Background())
		if err != nil {
			ref.Send(streamErrMsg{err: err})
			return
		}
		for ev := range ch {
			switch ev.Type {
			case "content":
				ref.Send(streamChunkMsg(ev.Content))
			case "error":
				ref.Send(streamErrMsg{err: fmt.Errorf("%s", ev.Content)})
				return
			}
		}
		ref.Send(streamDoneMsg{})
	}()
}

func (m chatModel) View() string {
	if m.quitting {
		return dimStyle.Render("Goodbye.") + "\n"
	}

	var b strings.Builder

	b.WriteString(headerStyle.Render(fmt.Sprintf("🦅 hawk v%s", version)))
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %s/%s", m.session.Provider(), m.session.Model())))
	b.WriteString(dimStyle.Render("  (ctrl+c to quit)"))
	b.WriteString("\n\n")

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

	if m.waiting {
		partial := m.partial.String()
		if partial != "" {
			b.WriteString(assistStyle.Render("hawk: "))
			b.WriteString(partial)
			b.WriteString("\n\n")
		} else {
			b.WriteString(m.spinner.View() + " Thinking...\n\n")
		}
	}

	if !m.waiting {
		b.WriteString(m.input.View())
		b.WriteString("\n")
	}

	return b.String()
}

func runChat() error {
	ref := &progRef{}
	m := newChatModel(ref)

	if promptFlag != "" {
		m.messages = append(m.messages, displayMsg{role: "user", content: promptFlag})
		m.session.AddUser(promptFlag)
		m.waiting = true
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	ref.Set(p)

	if promptFlag != "" {
		sess := m.session
		go func() {
			ch, err := sess.Stream(context.Background())
			if err != nil {
				p.Send(streamErrMsg{err: err})
				return
			}
			for ev := range ch {
				switch ev.Type {
				case "content":
					p.Send(streamChunkMsg(ev.Content))
				case "error":
					p.Send(streamErrMsg{err: fmt.Errorf("%s", ev.Content)})
					return
				}
			}
			p.Send(streamDoneMsg{})
		}()
	}

	_, err := p.Run()
	return err
}
