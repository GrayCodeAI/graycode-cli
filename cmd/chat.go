package cmd

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hawk/eyrie/client"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
	"github.com/GrayCodeAI/hawk/engine"
	"github.com/GrayCodeAI/hawk/prompt"
	"github.com/GrayCodeAI/hawk/session"
	"github.com/GrayCodeAI/hawk/tool"
)

var (
	tealColor    = lipgloss.Color("#4ECDC4")
	dimColor     = lipgloss.Color("#666666")
	errorColor   = lipgloss.Color("#e05555")
	toolColor    = lipgloss.Color("#FFD700")
	userStyle    = lipgloss.NewStyle().Foreground(tealColor).Bold(true)
	assistStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	dimStyle     = lipgloss.NewStyle().Foreground(dimColor)
	errorStyle   = lipgloss.NewStyle().Foreground(errorColor)
	headerStyle  = lipgloss.NewStyle().Foreground(tealColor).Bold(true)
	toolStyle    = lipgloss.NewStyle().Foreground(toolColor).Bold(true)
	toolDimStyle = lipgloss.NewStyle().Foreground(dimColor)
)

type streamChunkMsg string
type streamDoneMsg struct{}
type streamErrMsg struct{ err error }
type toolUseMsg struct{ name, id string }
type toolResultMsg struct{ name, content string }
type permissionAskMsg struct{ req engine.PermissionRequest }
type thinkingMsg string

type displayMsg struct {
	role    string
	content string
}

type progRef struct {
	mu sync.Mutex
	p  *tea.Program
}

func (r *progRef) Set(p *tea.Program) { r.mu.Lock(); r.p = p; r.mu.Unlock() }
func (r *progRef) Send(msg tea.Msg)   { r.mu.Lock(); p := r.p; r.mu.Unlock(); if p != nil { p.Send(msg) } }

type chatModel struct {
	input     textarea.Model
	spinner   spinner.Model
	session   *engine.Session
	ref       *progRef
	cancel    context.CancelFunc // cancel current stream
	sessionID string
	messages  []displayMsg
	partial   strings.Builder
	waiting   bool
	permReq   *engine.PermissionRequest // pending permission prompt
	width     int
	height    int
	quitting  bool
}

func defaultRegistry() *tool.Registry {
	return tool.NewRegistry(
		tool.BashTool{},
		tool.FileReadTool{},
		tool.FileWriteTool{},
		tool.FileEditTool{},
		tool.GlobTool{},
		tool.GrepTool{},
		tool.WebFetchTool{},
		tool.WebSearchTool{},
	)
}

func genID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func newChatModel(ref *progRef) chatModel {
	ta := textarea.New()
	ta.Placeholder = "Message hawk... (type /help for commands)"
	ta.Focus()
	ta.CharLimit = 0
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(tealColor)

	systemPrompt := prompt.System() + "\n\n" + hawkconfig.BuildContext()
	sess := engine.NewSession(provider, model, systemPrompt, defaultRegistry())
	sid := genID()

	m := chatModel{input: ta, spinner: sp, session: sess, ref: ref, sessionID: sid}

	// Wire permission system: engine sends requests via PermissionFn, TUI handles them
	sess.PermissionFn = func(req engine.PermissionRequest) {
		ref.Send(permissionAskMsg{req: req})
	}

	// Resume a saved session
	if resumeID != "" {
		if saved, err := session.Load(resumeID); err == nil {
			m.sessionID = saved.ID
			var msgs []client.EyrieMessage
			for _, sm := range saved.Messages {
				msgs = append(msgs, client.EyrieMessage{Role: sm.Role, Content: sm.Content})
				m.messages = append(m.messages, displayMsg{role: sm.Role, content: sm.Content})
			}
			sess.LoadMessages(msgs)
		} else {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		}
	}

	return m
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Permission prompt active — handle y/n
		if m.permReq != nil {
			switch msg.String() {
			case "y", "Y":
				m.permReq.Response <- true
				m.messages = append(m.messages, displayMsg{role: "system", content: "✓ Allowed"})
				m.permReq = nil
			case "n", "N":
				m.permReq.Response <- false
				m.messages = append(m.messages, displayMsg{role: "system", content: "✗ Denied"})
				m.permReq = nil
			}
			return m, nil
		}
		if m.waiting {
			if msg.Type == tea.KeyCtrlC {
				// First Ctrl+C cancels stream, second quits
				if m.cancel != nil {
					m.cancel()
					m.cancel = nil
					m.messages = append(m.messages, displayMsg{role: "system", content: "⏹ Cancelled."})
					if m.partial.Len() > 0 {
						m.messages = append(m.messages, displayMsg{role: "assistant", content: m.partial.String()})
						m.partial.Reset()
					}
					m.waiting = false
					m.input.Focus()
					return m, nil
				}
				m.saveSession()
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			m.saveSession()
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.input.Reset()
			if strings.HasPrefix(text, "/") {
				return m.handleCommand(text)
			}
			m.messages = append(m.messages, displayMsg{role: "user", content: text})
			m.session.AddUser(text)
			m.waiting = true
			m.partial.Reset()
			m.startStream()
			return m, nil
		}

	case streamChunkMsg:
		m.partial.WriteString(string(msg))
		return m, nil

	case thinkingMsg:
		m.messages = append(m.messages, displayMsg{role: "thinking", content: string(msg)})
		return m, nil

	case toolUseMsg:
		if m.partial.Len() > 0 {
			m.messages = append(m.messages, displayMsg{role: "assistant", content: m.partial.String()})
			m.partial.Reset()
		}
		m.messages = append(m.messages, displayMsg{role: "tool_use", content: msg.name})
		return m, nil

	case toolResultMsg:
		content := msg.content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		m.messages = append(m.messages, displayMsg{role: "tool_result", content: fmt.Sprintf("[%s] %s", msg.name, content)})
		return m, nil

	case permissionAskMsg:
		m.permReq = &msg.req
		m.messages = append(m.messages, displayMsg{role: "permission", content: msg.req.Summary})
		return m, nil

	case streamDoneMsg:
		if m.partial.Len() > 0 {
			m.messages = append(m.messages, displayMsg{role: "assistant", content: m.partial.String()})
			m.partial.Reset()
		}
		m.waiting = false
		m.cancel = nil
		m.input.Focus()
		m.saveSession()
		return m, nil

	case streamErrMsg:
		m.messages = append(m.messages, displayMsg{role: "error", content: msg.err.Error()})
		m.partial.Reset()
		m.waiting = false
		m.cancel = nil
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

func (m *chatModel) handleCommand(text string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(text)
	cmd := parts[0]

	switch cmd {
	case "/quit", "/exit":
		m.saveSession()
		m.quitting = true
		return m, tea.Quit
	case "/clear":
		m.messages = nil
		m.messages = append(m.messages, displayMsg{role: "system", content: "Conversation cleared."})
		return m, nil
	case "/compact":
		before := m.session.MessageCount()
		m.session.Compact()
		after := m.session.MessageCount()
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Compacted: %d → %d messages", before, after)})
		return m, nil
	case "/diff":
		m.messages = append(m.messages, displayMsg{role: "user", content: "/diff"})
		m.session.AddUser("Show me a summary of all files you've modified or created in this session. Use bash with 'git diff --stat' or list the files.")
		m.waiting = true
		m.partial.Reset()
		m.startStream()
		return m, nil
	case "/help":
		help := `/clear         — Clear display
/compact       — Compact conversation to save context
/cost          — Show token usage and cost
/diff          — Show files modified this session
/model         — Show current model
/history       — List saved sessions
/resume <id>   — Resume a saved session
/quit          — Exit hawk`
		m.messages = append(m.messages, displayMsg{role: "system", content: help})
		return m, nil
	case "/cost":
		m.messages = append(m.messages, displayMsg{role: "system", content: m.session.Cost.Summary()})
		return m, nil
	case "/model":
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s/%s", m.session.Provider(), m.session.Model())})
		return m, nil
	case "/history":
		entries, err := session.List()
		if err != nil || len(entries) == 0 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No saved sessions."})
			return m, nil
		}
		var b strings.Builder
		for _, e := range entries {
			b.WriteString(fmt.Sprintf("  %s  %s  %s\n", e.ID, e.UpdatedAt.Format("Jan 02 15:04"), e.Preview))
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
		return m, nil
	case "/resume":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /resume <session-id>"})
			return m, nil
		}
		saved, err := session.Load(parts[1])
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.sessionID = saved.ID
		m.messages = nil
		var msgs []client.EyrieMessage
		for _, sm := range saved.Messages {
			msgs = append(msgs, client.EyrieMessage{Role: sm.Role, Content: sm.Content})
			m.messages = append(m.messages, displayMsg{role: sm.Role, content: sm.Content})
		}
		m.session.LoadMessages(msgs)
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Resumed session %s", saved.ID)})
		return m, nil
	default:
		m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Unknown command: %s (type /help)", cmd)})
		return m, nil
	}
}

func (m *chatModel) saveSession() {
	var msgs []session.Message
	for _, dm := range m.messages {
		if dm.role == "user" || dm.role == "assistant" {
			msgs = append(msgs, session.Message{Role: dm.role, Content: dm.content})
		}
	}
	if len(msgs) == 0 {
		return
	}
	_ = session.Save(&session.Session{
		ID: m.sessionID, Model: m.session.Model(), Provider: m.session.Provider(),
		Messages: msgs, CreatedAt: time.Now(),
	})
}

func (m *chatModel) startStream() {
	sess := m.session
	ref := m.ref
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go func() {
		ch, err := sess.Stream(ctx)
		if err != nil {
			ref.Send(streamErrMsg{err: err})
			return
		}
		for ev := range ch {
			switch ev.Type {
			case "content":
				ref.Send(streamChunkMsg(ev.Content))
			case "thinking":
				ref.Send(thinkingMsg(ev.Content))
			case "tool_use":
				ref.Send(toolUseMsg{name: ev.ToolName, id: ev.ToolID})
			case "tool_result":
				ref.Send(toolResultMsg{name: ev.ToolName, content: ev.Content})
			case "error":
				ref.Send(streamErrMsg{err: fmt.Errorf("%s", ev.Content)})
				return
			case "done":
				ref.Send(streamDoneMsg{})
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
	b.WriteString(dimStyle.Render(fmt.Sprintf("  [%s]", m.sessionID)))
	b.WriteString("\n\n")

	for _, msg := range m.messages {
		switch msg.role {
		case "user":
			b.WriteString(userStyle.Render("You: "))
			b.WriteString(msg.content)
		case "assistant":
			b.WriteString(assistStyle.Render("hawk: "))
			b.WriteString(msg.content)
		case "tool_use":
			b.WriteString(toolStyle.Render("⚡ " + msg.content))
		case "tool_result":
			b.WriteString(toolDimStyle.Render(msg.content))
		case "thinking":
			b.WriteString(dimStyle.Render("💭 " + msg.content))
		case "system":
			b.WriteString(dimStyle.Render(msg.content))
		case "permission":
			b.WriteString(toolStyle.Render("⚠ " + msg.content + "  [y/n]"))
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
				case "thinking":
					p.Send(thinkingMsg(ev.Content))
				case "tool_use":
					p.Send(toolUseMsg{name: ev.ToolName, id: ev.ToolID})
				case "tool_result":
					p.Send(toolResultMsg{name: ev.ToolName, content: ev.Content})
				case "error":
					p.Send(streamErrMsg{err: fmt.Errorf("%s", ev.Content)})
					return
				case "done":
					p.Send(streamDoneMsg{})
					return
				}
			}
			p.Send(streamDoneMsg{})
		}()
	}

	_, err := p.Run()
	return err
}
