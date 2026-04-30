package cmd

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	hawkmodel "github.com/GrayCodeAI/hawk/model"
	"github.com/GrayCodeAI/hawk/plugin"
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
	diffAddStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ECDC4"))
	diffDelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e05555"))
)

type streamChunkMsg string
type streamDoneMsg struct{}
type streamErrMsg struct{ err error }
type toolUseMsg struct{ name, id string }
type toolResultMsg struct{ name, content string }
type permissionAskMsg struct{ req engine.PermissionRequest }
type thinkingMsg string
type askUserMsg struct {
	question string
	response chan string
}

type displayMsg struct {
	role    string
	content string
}

type progRef struct {
	mu sync.Mutex
	p  *tea.Program
}

func (r *progRef) Set(p *tea.Program) { r.mu.Lock(); r.p = p; r.mu.Unlock() }
func (r *progRef) Send(msg tea.Msg) {
	r.mu.Lock()
	p := r.p
	r.mu.Unlock()
	if p != nil {
		p.Send(msg)
	}
}

type chatModel struct {
	input         textarea.Model
	spinner       spinner.Model
	session       *engine.Session
	registry      *tool.Registry
	settings      hawkconfig.Settings
	ref           *progRef
	cancel        context.CancelFunc // cancel current stream
	sessionID     string
	messages      []displayMsg
	partial       strings.Builder
	waiting       bool
	permReq       *engine.PermissionRequest // pending permission prompt
	askReq        *askUserMsg               // pending ask_user prompt
	width         int
	height        int
	quitting      bool
	pluginRuntime *plugin.Runtime
}

func baseTools() []tool.Tool {
	return []tool.Tool{
		tool.BashTool{},
		tool.FileReadTool{},
		tool.FileWriteTool{},
		tool.FileEditTool{},
		tool.LSTool{},
		tool.GlobTool{},
		tool.GrepTool{},
		tool.WebFetchTool{},
		tool.WebSearchTool{},
		tool.ToolSearchTool{},
		tool.SkillTool{},
		tool.AgentTool{},
		tool.AskUserQuestionTool{},
		tool.TodoWriteTool{},
		tool.TaskOutputTool{},
		tool.TaskStopTool{},
		tool.LSPTool{},
		tool.EnterPlanModeTool{},
		tool.ExitPlanModeTool{},
		tool.NotebookEditTool{},
		tool.EnterWorktreeTool{},
		tool.ExitWorktreeTool{},
		tool.ListMcpResourcesTool{},
		tool.ReadMcpResourceTool{},
		tool.ConfigTool{},
		tool.BriefTool{},
	}
}

func defaultRegistry(settings hawkconfig.Settings) (*tool.Registry, error) {
	tools := baseTools()
	for _, cfg := range settings.MCPServers {
		if cfg.Name == "" || cfg.Command == "" {
			continue
		}
		mcpTools, err := tool.LoadMCPTools(context.Background(), cfg.Name, cfg.Command, cfg.Args...)
		if err != nil {
			continue
		}
		tools = append(tools, mcpTools...)
	}
	// Load MCP server tools
	for _, cmd := range mcpServers {
		parts := strings.Fields(cmd)
		if len(parts) == 0 {
			continue
		}
		name := parts[0]
		mcpTools, err := tool.LoadMCPTools(context.Background(), name, parts[0], parts[1:]...)
		if err != nil {
			// MCP server failed to connect — skip silently, will show in /doctor
			continue
		}
		tools = append(tools, mcpTools...)
	}

	filtered, err := filterAvailableTools(
		tools,
		toolsFlagSet,
		parseToolListFromCLI(toolsFlag),
		parseToolListFromCLI(disallowedToolsFlag),
	)
	if err != nil {
		return nil, err
	}
	return tool.NewRegistry(filtered...), nil
}

func genID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func prepareSession(sess *engine.Session) (string, *session.Session, error) {
	id := genID()
	if sessionIDFlag != "" && resumeID == "" && !continueFlag {
		id = sessionIDFlag
	}
	if resumeID == "" && !continueFlag {
		return id, nil, nil
	}

	var (
		saved *session.Session
		err   error
	)
	if resumeID != "" {
		saved, err = session.Load(resumeID)
	} else {
		cwd, _ := os.Getwd()
		saved, err = session.LoadLatestForCWD(cwd)
	}
	if err != nil {
		return "", nil, err
	}
	sess.LoadMessages(toEyrieMessages(saved.Messages))
	if forkSessionFlag {
		if sessionIDFlag != "" {
			id = sessionIDFlag
		}
		return id, saved, nil
	}
	return saved.ID, saved, nil
}

func buildWelcomeMessage(sess *engine.Session, sessionID string, registry *tool.Registry, saved *session.Session, settings hawkconfig.Settings) string {
	cwd, _ := os.Getwd()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🦅 Welcome to hawk v%s\n\n", version))
	b.WriteString(fmt.Sprintf("Provider: %s  Model: %s\n", sess.Provider(), sess.Model()))
	b.WriteString(fmt.Sprintf("Session: %s", sessionID))
	if saved != nil {
		if forkSessionFlag {
			b.WriteString(fmt.Sprintf("  forked from: %s", saved.ID))
		} else {
			b.WriteString("  resumed")
		}
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Directory: %s\n", cwd))
	b.WriteString(fmt.Sprintf("Permission mode: %s\n", sess.Mode))
	if settings.Theme != "" {
		b.WriteString(fmt.Sprintf("Theme: %s\n", settings.Theme))
	}
	if hawkconfig.LoadHawkMD() != "" {
		b.WriteString("Project instructions: HAWK.md loaded\n")
	} else {
		b.WriteString("Project instructions: none found\n")
	}
	if len(settings.MCPServers) > 0 || len(mcpServers) > 0 {
		b.WriteString(fmt.Sprintf("MCP servers: %d configured", len(settings.MCPServers)+len(mcpServers)))
		if len(settings.MCPServers) > 0 {
			names := make([]string, 0, len(settings.MCPServers))
			for _, cfg := range settings.MCPServers {
				if cfg.Name != "" {
					names = append(names, cfg.Name)
				}
			}
			if len(names) > 0 {
				b.WriteString(" (" + strings.Join(names, ", ") + ")")
			}
		}
		b.WriteString("\n")
	} else {
		b.WriteString("MCP servers: none configured\n")
	}
	if registry != nil {
		tools := registry.EyrieTools()
		names := make([]string, 0, len(tools))
		for _, t := range tools {
			names = append(names, t.Name)
		}
		if len(names) > 8 {
			names = append(names[:8], fmt.Sprintf("+%d more", len(tools)-8))
		}
		b.WriteString(fmt.Sprintf("Tools: %d available", len(tools)))
		if len(names) > 0 {
			b.WriteString(" (" + strings.Join(names, ", ") + ")")
		}
		b.WriteString("\n")
	}
	if len(addDirs) > 0 {
		b.WriteString("Additional dirs: " + strings.Join(addDirs, ", ") + "\n")
	}
	b.WriteString("\n")
	b.WriteString("Common commands: /help, /status, /tools, /permissions mode plan, /doctor, /welcome\n")
	b.WriteString("Type a message to chat or Ctrl+C to quit.")
	return b.String()
}

func permissionCommandArg(text, action string) string {
	prefix := "/permissions " + action
	if !strings.HasPrefix(text, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(text, prefix))
}

func toolListSummary(registry *tool.Registry) string {
	if registry == nil {
		return "No tools enabled."
	}
	tools := registry.EyrieTools()
	if len(tools) == 0 {
		return "No tools enabled."
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Enabled tools (%d):\n", len(tools)))
	for _, t := range tools {
		desc := t.Description
		if len(desc) > 96 {
			desc = desc[:96] + "..."
		}
		b.WriteString(fmt.Sprintf("  %s — %s\n", t.Name, desc))
	}
	return strings.TrimRight(b.String(), "\n")
}

func envSummary(provider, model string) string {
	envKeys := []string{
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"GEMINI_API_KEY",
		"OPENROUTER_API_KEY",
		"GROQ_API_KEY",
		"XAI_API_KEY",
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Provider: %s\nModel: %s\n\nEnvironment:\n", provider, model))
	for _, key := range envKeys {
		status := "missing"
		if os.Getenv(key) != "" {
			status = "set"
		}
		b.WriteString(fmt.Sprintf("  %s: %s\n", key, status))
	}
	return strings.TrimRight(b.String(), "\n")
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func branchSummary() string {
	branch, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || branch == "" {
		return "No git repository detected."
	}
	head, _ := gitOutput("rev-parse", "--short", "HEAD")
	upstream, _ := gitOutput("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	status, _ := gitOutput("status", "--short", "--branch")
	var b strings.Builder
	b.WriteString("Branch: " + branch)
	if head != "" {
		b.WriteString(" @ " + head)
	}
	if upstream != "" {
		b.WriteString("\nUpstream: " + upstream)
	}
	if status != "" {
		b.WriteString("\n\n" + status)
	}
	return b.String()
}

func filesSummary() string {
	status, err := gitOutput("status", "--short")
	if err != nil {
		return "No git repository detected."
	}
	if strings.TrimSpace(status) == "" {
		return "No modified files."
	}
	return "Modified files:\n" + status
}

func additionalDirContext(dir string) (string, string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", "", fmt.Errorf("directory path is required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("%s is not a directory", abs)
	}
	var b strings.Builder
	b.WriteString("Additional directory: " + abs)
	if md := hawkconfig.LoadHawkMDFrom(abs); md != "" {
		b.WriteString("\nAdditional directory instructions (" + abs + "):\n" + md)
	}
	return abs, b.String(), nil
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (m *chatModel) mcpSummary() string {
	var b strings.Builder
	configured := len(m.settings.MCPServers) + len(mcpServers)
	if configured == 0 {
		b.WriteString("No MCP servers configured.")
	} else {
		b.WriteString(fmt.Sprintf("MCP servers configured: %d\n", configured))
		for _, cfg := range m.settings.MCPServers {
			name := cfg.Name
			if name == "" {
				name = cfg.Command
			}
			b.WriteString(fmt.Sprintf("  %s: %s %s\n", name, cfg.Command, strings.Join(cfg.Args, " ")))
		}
		for _, cmd := range mcpServers {
			b.WriteString("  cli: " + cmd + "\n")
		}
	}
	if m.registry != nil {
		var toolNames []string
		for _, t := range m.registry.EyrieTools() {
			if strings.HasPrefix(t.Name, "mcp__") {
				toolNames = append(toolNames, t.Name)
			}
		}
		if len(toolNames) > 0 {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString("Connected MCP tools:\n  " + strings.Join(toolNames, "\n  "))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *chatModel) startPromptCommand(display, prompt string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "user", content: display})
	m.session.AddUser(prompt)
	m.waiting = true
	m.partial.Reset()
	m.startStream()
	return m, nil
}

func newChatModel(ref *progRef, systemPrompt string, settings hawkconfig.Settings) (chatModel, error) {
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

	effectiveModel, effectiveProvider := effectiveModelAndProvider(settings)
	registry, err := defaultRegistry(settings)
	if err != nil {
		return chatModel{}, err
	}
	sess := engine.NewSession(effectiveProvider, effectiveModel, systemPrompt, registry)
	if err := configureSession(sess, settings); err != nil {
		return chatModel{}, err
	}
	sid, saved, err := prepareSession(sess)
	if err != nil {
		return chatModel{}, err
	}

	m := chatModel{input: ta, spinner: sp, session: sess, registry: registry, settings: settings, ref: ref, sessionID: sid}

	// Initialize plugin runtime
	pr := plugin.NewRuntime()
	_ = pr.LoadAll()
	pr.RegisterHooks()
	m.pluginRuntime = pr

	// Welcome message inside TUI
	m.messages = append(m.messages, displayMsg{role: "welcome", content: buildWelcomeMessage(sess, sid, registry, saved, settings)})

	// Wire permission system
	sess.PermissionFn = func(req engine.PermissionRequest) {
		ref.Send(permissionAskMsg{req: req})
	}

	// Wire ask_user tool
	sess.AskUserFn = func(question string) (string, error) {
		resp := make(chan string, 1)
		ref.Send(askUserMsg{question: question, response: resp})
		answer := <-resp
		return answer, nil
	}

	if saved != nil {
		for _, sm := range saved.Messages {
			if sm.Role == "user" || sm.Role == "assistant" {
				m.messages = append(m.messages, displayMsg{role: sm.Role, content: sm.Content})
			}
		}
	}

	return m, nil
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
		// AskUser prompt active — Enter submits answer
		if m.askReq != nil {
			if msg.Type == tea.KeyEnter {
				answer := strings.TrimSpace(m.input.Value())
				m.input.Reset()
				m.messages = append(m.messages, displayMsg{role: "user", content: answer})
				m.askReq.response <- answer
				m.askReq = nil
				return m, nil
			}
			// Let textarea handle other keys
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
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

	case askUserMsg:
		m.askReq = &msg
		m.messages = append(m.messages, displayMsg{role: "question", content: "❓ " + msg.question})
		m.input.Focus()
		m.input.SetValue("")
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
	case "/add-dir":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /add-dir <path>"})
			return m, nil
		}
		dirArg := strings.TrimSpace(strings.TrimPrefix(text, "/add-dir"))
		abs, contextBlock, err := additionalDirContext(dirArg)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		if !hasString(addDirs, abs) {
			addDirs = append(addDirs, abs)
			m.session.AppendSystemContext(contextBlock)
			m.session.SetAllowedDirs(addDirs)
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: "Added directory to context: " + abs})
		return m, nil
	case "/branch":
		m.messages = append(m.messages, displayMsg{role: "system", content: branchSummary()})
		return m, nil
	case "/clear":
		m.messages = nil
		m.messages = append(m.messages, displayMsg{role: "system", content: "Conversation cleared."})
		return m, nil
	case "/compact":
		before := m.session.MessageCount()
		m.session.SmartCompact()
		after := m.session.MessageCount()
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Compacted: %d → %d messages (LLM summary)", before, after)})
		return m, nil
	case "/diff":
		return m.startPromptCommand("/diff", "Show me a summary of all files you've modified or created in this session. Use git diff --stat or list the files.")
	case "/help":
		help := `/add-dir <path>     — Add a directory to context
/branch             — Show git branch/status
/bughunter          — Ask hawk to hunt for bugs
/clear              — Clear display
/compact            — Compact conversation (LLM summary)
/commit             — Auto-commit changes
/config             — Show settings
/context            — Show current context
/cost               — Token usage and cost
/metrics            — Show collected metrics
/diff               — Review changes
/doctor             — Run diagnostics
/env                — Show provider environment status
/files              — Show modified files
/history            — List saved sessions
/init               — Analyze project
/mcp                — Show MCP status
/memory             — Show loaded project instructions
/model              — Show current model
/permissions allow  — Always allow a tool or rule
/permissions deny   — Always deny a tool or rule
/permissions mode   — Set permission mode
/plan               — Enter plan mode (read-only)
/pr-comments        — Ask hawk to handle PR comments
/release-notes      — Draft release notes
/resume <id>        — Resume session
/review             — Ask hawk to review changes
/security-review    — Ask hawk to review security risks
/skills             — List local skills
/status             — Session status
/summary            — Summarize the current session
/tools              — List enabled tools
/usage              — Token usage
/version            — Show hawk version
/welcome            — Show startup summary
/quit               — Exit hawk`
		m.messages = append(m.messages, displayMsg{role: "system", content: help})
		return m, nil
	case "/cost":
		m.messages = append(m.messages, displayMsg{role: "system", content: m.session.Cost.Summary()})
		return m, nil
	case "/metrics":
		m.messages = append(m.messages, displayMsg{role: "system", content: m.session.Metrics().Format()})
		return m, nil
	case "/model":
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s/%s", m.session.Provider(), m.session.Model())})
		return m, nil
	case "/models":
		m.messages = append(m.messages, displayMsg{role: "system", content: modelCatalogSummary()})
		return m, nil
	case "/version":
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("hawk %s", version)})
		return m, nil
	case "/env":
		m.messages = append(m.messages, displayMsg{role: "system", content: envSummary(m.session.Provider(), m.session.Model())})
		return m, nil
	case "/files":
		m.messages = append(m.messages, displayMsg{role: "system", content: filesSummary()})
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
			em := client.EyrieMessage{Role: sm.Role, Content: sm.Content}
			for _, tc := range sm.ToolUse {
				em.ToolUse = append(em.ToolUse, client.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
			}
			if sm.ToolResult != nil {
				em.ToolResult = &client.ToolResult{ToolUseID: sm.ToolResult.ToolUseID, Content: sm.ToolResult.Content, IsError: sm.ToolResult.IsError}
			}
			msgs = append(msgs, em)
			if sm.Role == "user" || sm.Role == "assistant" {
				m.messages = append(m.messages, displayMsg{role: sm.Role, content: sm.Content})
			}
		}
		m.session.LoadMessages(msgs)
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Resumed session %s", saved.ID)})
		return m, nil
	case "/commit":
		return m.startPromptCommand("/commit", "Review the changes I've made, then create a git commit with an appropriate commit message. Use git add for specific files and git commit.")
	case "/doctor":
		return m.startPromptCommand("/doctor", "Run diagnostics on this project: check if it builds, run tests, check for lint errors. Report any issues found.")
	case "/init":
		return m.startPromptCommand("/init", "Analyze this project: read the README, check the directory structure, identify the language/framework, build system, and test runner. Give me a brief summary.")
	case "/review":
		return m.startPromptCommand("/review", "Review the current changes for bugs, regressions, missing tests, and risky behavior. Prioritize actionable findings with file references.")
	case "/security-review":
		return m.startPromptCommand("/security-review", "Review the repository for security risks. Focus on command execution, file permissions, secret exposure, network access, authentication, and unsafe defaults.")
	case "/bughunter":
		return m.startPromptCommand("/bughunter", "Hunt for likely bugs in the current codebase and changes. Prioritize concrete defects that can be reproduced or fixed.")
	case "/summary":
		return m.startPromptCommand("/summary", "Summarize the current session, important decisions, modified files, test status, and remaining work.")
	case "/release-notes":
		return m.startPromptCommand("/release-notes", "Draft concise release notes for the current changes, grouped by user-facing improvements, fixes, and compatibility notes.")
	case "/pr-comments":
		return m.startPromptCommand("/pr-comments", "Review open PR comments or, if unavailable, inspect the current diff and suggest responses or fixes for likely review comments.")
	case "/permissions":
		if len(parts) >= 2 {
			switch parts[1] {
			case "allow":
				spec := permissionCommandArg(text, "allow")
				if spec != "" {
					m.session.Permissions.AllowSpec(spec)
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Always allowing: %s", spec)})
					return m, nil
				}
			case "deny":
				spec := permissionCommandArg(text, "deny")
				if spec != "" {
					m.session.Permissions.DenySpec(spec)
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Always denying: %s", spec)})
					return m, nil
				}
			case "mode":
				mode := permissionCommandArg(text, "mode")
				if err := m.session.SetPermissionMode(mode); err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				} else {
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Permission mode: %s", m.session.Mode)})
				}
				return m, nil
			}
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /permissions allow <rule>, /permissions deny <rule>, /permissions mode <mode>\nExamples: /permissions allow Bash(git:*), /permissions deny Write(*.env), /permissions mode plan"})
		return m, nil
	case "/status":
		toolCount := 0
		if m.registry != nil {
			toolCount = len(m.registry.EyrieTools())
		}
		info := fmt.Sprintf("Session: %s\nModel: %s/%s\nPermission mode: %s\nMessages: %d\nTools: %d\n%s",
			m.sessionID, m.session.Provider(), m.session.Model(),
			m.session.Mode, m.session.MessageCount(), toolCount, m.session.Cost.Summary())
		if len(addDirs) > 0 {
			info += "\nAdditional dirs: " + strings.Join(addDirs, ", ")
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: info})
		return m, nil
	case "/context":
		m.messages = append(m.messages, displayMsg{role: "system", content: hawkconfig.BuildContextWithDirs(addDirs)})
		return m, nil
	case "/memory":
		md := strings.TrimSpace(hawkconfig.LoadHawkMD())
		if md == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No HAWK.md or .hawk/HAWK.md project instructions found."})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Project instructions:\n" + md})
		}
		return m, nil
	case "/config":
		settings, err := loadEffectiveSettings()
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		data, _ := json.MarshalIndent(settings, "", "  ")
		m.messages = append(m.messages, displayMsg{role: "system", content: "Settings:\n" + string(data)})
		return m, nil
	case "/mcp":
		m.messages = append(m.messages, displayMsg{role: "system", content: m.mcpSummary()})
		return m, nil
	case "/plan":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Plan mode: hawk will only read and discuss, no modifications."})
		_ = m.session.SetPermissionMode(string(engine.PermissionModePlan))
		m.session.AddUser("Enter plan mode. Only read files and discuss plans — do not write files or run commands that modify state until I say to proceed.")
		m.waiting = true
		m.partial.Reset()
		m.startStream()
		return m, nil
	case "/usage":
		m.messages = append(m.messages, displayMsg{role: "system", content: m.session.Cost.Summary()})
		return m, nil
	case "/tools":
		m.messages = append(m.messages, displayMsg{role: "system", content: toolListSummary(m.registry)})
		return m, nil
	case "/skills":
		out, err := (tool.SkillTool{}).Execute(context.Background(), nil)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: out})
		}
		return m, nil
	case "/welcome":
		m.messages = append(m.messages, displayMsg{role: "welcome", content: buildWelcomeMessage(m.session, m.sessionID, m.registry, nil, m.settings)})
		return m, nil
	default:
		// Check if it's a plugin command
		if m.pluginRuntime != nil && m.pluginRuntime.IsCommand(cmd[1:]) {
			out, err := m.pluginRuntime.ExecuteCommand(cmd[1:], parts[1:])
			if err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			} else {
				m.messages = append(m.messages, displayMsg{role: "system", content: out})
			}
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Unknown command: %s (type /help)", cmd)})
		return m, nil
	}
}

func (m *chatModel) saveSession() {
	raw := m.session.RawMessages()
	if len(raw) == 0 {
		return
	}
	var msgs []session.Message
	for _, rm := range raw {
		sm := session.Message{Role: rm.Role, Content: rm.Content}
		for _, tc := range rm.ToolUse {
			sm.ToolUse = append(sm.ToolUse, session.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
		}
		if rm.ToolResult != nil {
			sm.ToolResult = &session.ToolResult{ToolUseID: rm.ToolResult.ToolUseID, Content: rm.ToolResult.Content, IsError: rm.ToolResult.IsError}
		}
		msgs = append(msgs, sm)
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
			case "usage":
				// Usage events are only emitted in stream-json print mode
				// TUI mode ignores them since cost is tracked separately
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
		case "welcome":
			b.WriteString(headerStyle.Render(msg.content))
		case "system":
			b.WriteString(dimStyle.Render(msg.content))
		case "permission":
			b.WriteString(toolStyle.Render("⚠ " + msg.content + "  [y/n]"))
		case "question":
			b.WriteString(toolStyle.Render(msg.content))
		case "usage":
			// Usage events are not displayed in TUI
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

	if !m.waiting || m.askReq != nil {
		b.WriteString(m.input.View())
		b.WriteString("\n")
	}

	return b.String()
}

func runChat() error {
	ref := &progRef{}
	systemPrompt, err := buildSystemPrompt()
	if err != nil {
		return err
	}
	settings, err := loadEffectiveSettings()
	if err != nil {
		return err
	}
	m, err := newChatModel(ref, systemPrompt, settings)
	if err != nil {
		return err
	}

	if promptFlag != "" {
		m.messages = append(m.messages, displayMsg{role: "user", content: promptFlag})
		m.session.AddUser(promptFlag)
		m.waiting = true
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	ref.Set(p)

	if promptFlag != "" {
		sess := m.session
		ctx, cancel := context.WithCancel(context.Background())
		_ = cancel // will be cancelled when program exits
		go func() {
			ch, err := sess.Stream(ctx)
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
				case "usage":
					// Usage events are only emitted in stream-json print mode
					// TUI mode ignores them since cost is tracked separately
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

	_, err = p.Run()
	return err
}

func runPrint(text string) error {
	systemPrompt, err := buildSystemPrompt()
	if err != nil {
		return err
	}

	settings, err := loadEffectiveSettings()
	if err != nil {
		return err
	}
	effectiveModel, effectiveProvider := effectiveModelAndProvider(settings)
	registry, err := defaultRegistry(settings)
	if err != nil {
		return err
	}

	sess := engine.NewSession(effectiveProvider, effectiveModel, systemPrompt, registry)
	if err := configureSession(sess, settings); err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	sess.PermissionFn = func(req engine.PermissionRequest) {
		fmt.Fprintf(os.Stderr, "\nAllow %s: %s [y/N] ", req.ToolName, req.Summary)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		req.Response <- answer == "y" || answer == "yes"
	}
	sess.AskUserFn = func(question string) (string, error) {
		fmt.Fprintf(os.Stderr, "\n%s\n> ", question)
		answer, _ := reader.ReadString('\n')
		return strings.TrimSpace(answer), nil
	}

	sessionID, _, err := prepareSession(sess)
	if err != nil {
		return err
	}

	sess.AddUser(text)
	ch, err := sess.Stream(context.Background())
	if err != nil {
		return err
	}

	var printed strings.Builder
	for ev := range ch {
		switch ev.Type {
		case "content":
			if outputFormat == "text" {
				fmt.Print(ev.Content)
			} else if outputFormat == "stream-json" {
				writePrintEvent(sessionID, "content", ev.Content, "")
			}
			printed.WriteString(ev.Content)
		case "tool_use":
			if outputFormat == "stream-json" {
				writePrintEvent(sessionID, "tool_use", "", ev.ToolName)
			} else {
				fmt.Fprintf(os.Stderr, "\n[%s]\n", ev.ToolName)
			}
		case "tool_result":
			content := ev.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			if outputFormat == "stream-json" {
				writePrintEvent(sessionID, "tool_result", content, ev.ToolName)
			} else {
				fmt.Fprintf(os.Stderr, "[%s] %s\n", ev.ToolName, content)
			}
		case "usage":
			if outputFormat == "stream-json" && ev.Usage != nil {
				writePrintUsageEvent(sessionID, ev.Usage)
			}
		case "error":
			if outputFormat == "stream-json" {
				writePrintResult(printed.String(), sessionID, sess, true, []string{ev.Content})
			}
			return fmt.Errorf("%s", ev.Content)
		case "done":
			switch outputFormat {
			case "text":
				if !strings.HasSuffix(printed.String(), "\n") {
					fmt.Println()
				}
			case "json":
				writePrintResult(printed.String(), sessionID, sess, false, nil)
			case "stream-json":
				writePrintResult(printed.String(), sessionID, sess, false, nil)
			}
			if !noSessionPersistence {
				saveEyrieSession(sessionID, sess)
			}
			return nil
		}
	}
	switch outputFormat {
	case "text":
		if !strings.HasSuffix(printed.String(), "\n") {
			fmt.Println()
		}
	case "json":
		writePrintResult(printed.String(), sessionID, sess, false, nil)
	case "stream-json":
		writePrintResult(printed.String(), sessionID, sess, false, nil)
	}
	if !noSessionPersistence {
		saveEyrieSession(sessionID, sess)
	}
	return nil
}

func writePrintUsageEvent(sessionID string, usage *engine.StreamUsage) {
	event := map[string]interface{}{
		"type":       "usage",
		"uuid":       genID(),
		"session_id": sessionID,
		"usage": map[string]int{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
		},
	}
	if usage.CacheReadTokens > 0 {
		event["usage"].(map[string]int)["cache_read_tokens"] = usage.CacheReadTokens
	}
	if usage.CacheWriteTokens > 0 {
		event["usage"].(map[string]int)["cache_write_tokens"] = usage.CacheWriteTokens
	}
	data, _ := json.Marshal(event)
	fmt.Println(string(data))
}

func writePrintResult(result, sessionID string, sess *engine.Session, isError bool, errors []string) {
	event := map[string]interface{}{
		"type":           "result",
		"subtype":        "success",
		"is_error":       isError,
		"result":         result,
		"session_id":     sessionID,
		"uuid":           genID(),
		"total_cost_usd": sess.Cost.Total(),
	}
	if isError {
		event["subtype"] = "error_during_execution"
		event["errors"] = errors
	}
	data, _ := json.Marshal(event)
	fmt.Println(string(data))
}

func writePrintEvent(sessionID, eventType, content, toolName string) {
	event := map[string]string{
		"type":       eventType,
		"uuid":       genID(),
		"session_id": sessionID,
	}
	if content != "" {
		event["content"] = content
	}
	if toolName != "" {
		event["tool_name"] = toolName
	}
	data, _ := json.Marshal(event)
	fmt.Println(string(data))
}

func saveEyrieSession(id string, sess *engine.Session) {
	raw := sess.RawMessages()
	if len(raw) == 0 {
		return
	}
	var msgs []session.Message
	for _, rm := range raw {
		sm := session.Message{Role: rm.Role, Content: rm.Content}
		for _, tc := range rm.ToolUse {
			sm.ToolUse = append(sm.ToolUse, session.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
		}
		if rm.ToolResult != nil {
			sm.ToolResult = &session.ToolResult{ToolUseID: rm.ToolResult.ToolUseID, Content: rm.ToolResult.Content, IsError: rm.ToolResult.IsError}
		}
		msgs = append(msgs, sm)
	}
	_ = session.Save(&session.Session{
		ID:        id,
		Model:     sess.Model(),
		Provider:  sess.Provider(),
		Messages:  msgs,
		CreatedAt: time.Now(),
	})
}

func toEyrieMessages(saved []session.Message) []client.EyrieMessage {
	msgs := make([]client.EyrieMessage, 0, len(saved))
	for _, sm := range saved {
		em := client.EyrieMessage{Role: sm.Role, Content: sm.Content}
		for _, tc := range sm.ToolUse {
			em.ToolUse = append(em.ToolUse, client.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
		}
		if sm.ToolResult != nil {
			em.ToolResult = &client.ToolResult{ToolUseID: sm.ToolResult.ToolUseID, Content: sm.ToolResult.Content, IsError: sm.ToolResult.IsError}
		}
		msgs = append(msgs, em)
	}
	return msgs
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

func modelCatalogSummary() string {
	var b strings.Builder
	b.WriteString("Available models by provider:\n\n")
	for _, p := range hawkmodel.AllProviders() {
		b.WriteString(fmt.Sprintf("%s:\n", p))
		for _, m := range hawkmodel.ByProvider(p) {
			marker := ""
			if m.Recommended {
				marker = " *"
			}
			b.WriteString(fmt.Sprintf("  %s%s\n", m.Name, marker))
			if m.Description != "" {
				b.WriteString(fmt.Sprintf("    %s\n", m.Description))
			}
			b.WriteString(fmt.Sprintf("    Context: %dk, Input: $%.2f/M, Output: $%.2f/M\n",
				m.ContextSize/1000, m.InputPrice, m.OutputPrice))
		}
		b.WriteString("\n")
	}
	b.WriteString("* = recommended default\n")
	return strings.TrimRight(b.String(), "\n")
}
