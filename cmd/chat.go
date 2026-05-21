package cmd

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/GrayCodeAI/eyrie/storage"
	"github.com/GrayCodeAI/hawk/internal/bridge/sessioncapture"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/eyrieclient"
	"github.com/GrayCodeAI/hawk/internal/feature/shellmode"
	"github.com/GrayCodeAI/hawk/internal/feature/taste"
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/system/staleness"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// Types, styles, and model struct are in chat_model.go
// Welcome message and config summary helpers are in chat_welcome.go
// Slash command handling and helpers are in chat_commands.go

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
		tool.TaskCreateTool{},
		tool.TaskGetTool{},
		tool.TaskListTool{},
		tool.TaskUpdateTool{},
		tool.SleepTool{},
		tool.CronCreateTool{},
		tool.CronDeleteTool{},
		tool.CronListTool{},
		tool.VerifyPlanExecutionTool{},
		tool.WorkflowTool{},
		tool.McpAuthTool{},
		tool.DiagnosticsTool{},
		tool.CodeSearchTool{},
		tool.CoreMemoryAppendTool{},
		tool.CoreMemoryReplaceTool{},
		tool.CoreMemoryRethinkTool{},
		tool.MultiEditTool{},
		tool.DownloadTool{},
		tool.AgenticFetchTool{},
	}
}

func defaultRegistry(settings hawkconfig.Settings) (*tool.Registry, error) {
	tools := baseTools()
	if tool.IsPowerShellAvailable() {
		tools = append(tools, tool.PowerShellTool{})
	}
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
	b := make([]byte, 8)
	_, _ = cryptorand.Read(b)
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

func newChatModel(ref *progRef, systemPrompt string, settings hawkconfig.Settings) (chatModel, error) {
	ta := textarea.New()
	ta.Placeholder = `Try "Create a PR with these changes" (Shift+Enter for newline)`
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.MaxHeight = 10
	ta.SetHeight(1)
	taWidth := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 10 {
		taWidth = w
	}
	ta.SetWidth(taWidth - 4)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2F2F2"))
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("#7A7A7A"))
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	ta.BlurredStyle = ta.FocusedStyle
	ta.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E"))
	ta.Prompt = "❯ "
	// Enter submits; Shift+Enter inserts newline
	ta.KeyMap.InsertNewline.SetKeys("shift+enter")

	// Secondary textinput for config panel password entry
	ci := textinput.New()
	ci.EchoMode = textinput.EchoNormal

	sp := spinner.New()
	sp.Spinner = spinner.Spinner{Frames: hawkSpinnerFrames, FPS: 200 * time.Millisecond}
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)

	effectiveModel, effectiveProvider := effectiveModelAndProvider(settings)
	registry, err := defaultRegistry(settings)
	if err != nil {
		return chatModel{}, err
	}
	sess := newHawkSession(settings, effectiveProvider, effectiveModel, systemPrompt, registry)
	sess.SetLogger(logger.New(io.Discard, logger.Error))
	if err := configureSession(sess, settings); err != nil {
		return chatModel{}, err
	}
	sid, saved, err := prepareSession(sess)
	if err != nil {
		return chatModel{}, err
	}

	// Initialize conversation DAG for branching support
	if home, err := os.UserHomeDir(); err == nil {
		dagPath := filepath.Join(home, ".hawk", "sessions", "convo.db")
		if dag, err := storage.NewDAG(dagPath, sid); err == nil {
			sess.ConvoDAG = dag
		}
	}

	initWidth := 80
	initHeight := 24
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		initWidth = w
		if h > 0 {
			initHeight = h
		}
	}
	// Reserve lines for the bottom bar
	vpHeight := initHeight - 6
	if vpHeight < 4 {
		vpHeight = 4
	}
	vp := viewport.New(initWidth, vpHeight)
	vp.MouseWheelEnabled = true

	m := chatModel{input: ta, configInput: ci, spinner: sp, viewport: vp, session: sess, registry: registry, settings: settings, ref: ref, sessionID: sid, partial: &strings.Builder{}, spinnerVerb: spinnerVerbs[rand.Intn(len(spinnerVerbs))], width: initWidth, height: initHeight, historyIdx: 0, autoScroll: true, startedAt: time.Now(), activeSkills: make(map[string]plugin.SmartSkill)}
	m.containerEnabled = shouldUseContainer()
	if m.containerEnabled {
		m.containerStatus = "checking docker…"
	} else if noContainer {
		m.messages = append(m.messages, displayMsg{
			role: "system",
			content: "--no-container runs tools on the host without sandbox isolation. " +
				"Use default container mode for safer agent execution.",
		})
	}

	// Initialize lacy-inspired features
	m.termCtx = sessioncapture.NewTerminalContext()
	m.inputIndicator = &InputIndicator{}
	m.ghostText = NewGhostText()
	m.modeManager = shellmode.NewModeManager()
	m.modeManager.LoadPersistedMode()
	m.brailleSpinner = NewBrailleSpinner(SpinnerBrailleWave, "")

	// Initialize BMAD/Aeon features
	m.hintsLoader = engine.NewHintsLoader()
	m.sourceRoots = engine.NewSourceRoots()
	m.selfImprover = engine.NewSelfImprover()
	m.codingSoul = engine.LoadCodingSoul()

	// Initialize taste and staleness subsystems.
	m.stalenessDetector = staleness.NewDetector()
	if store, err := taste.NewStore(""); err == nil {
		cwd, _ := os.Getwd()
		projectID := filepath.Base(cwd)
		if hooks, err := taste.NewHooks(projectID, store); err == nil {
			m.tasteHooks = hooks
		}
	}

	// Initialize write-ahead log for crash recovery
	if wal, err := session.NewWAL(sid); err == nil {
		m.wal = wal
		_ = wal.AppendMeta(effectiveModel, effectiveProvider, "")
	}

	// Check for crash recovery
	if recovered := session.CheckForRecovery(); len(recovered) > 0 {
		home, _ := os.UserHomeDir()
		walDir := filepath.Join(home, ".hawk", "sessions")
		for _, rid := range recovered {
			if rid == sid {
				continue // current session WAL
			}
			if rs, err := session.RecoverFromWAL(rid); rs != nil && err == nil {
				_ = session.Save(rs)
				_ = os.Remove(filepath.Join(walDir, rid+".wal"))
			}
		}
	}

	// Warm code index in background so first CodeSearch is fast
	go func() {
		if bridge := memory.NewYaadBridge(); bridge != nil && bridge.Ready() {
			_ = bridge.InitCodeIndex()
			bridge.Close()
		}
	}()

	// Prefetch models for current provider in background so /config and /model are instant
	go func() {
		provider := effectiveProvider
		entries, _ := eyrieclient.ListModelsForProvider(context.Background(), provider)
		opts := configModelOptionsFromEyrie(entries)
		if len(opts) > 0 {
			modelCacheMu.Lock()
			modelCache[provider] = opts
			modelCacheMu.Unlock()
		}
	}()

	// Warm credential + catalog caches so typing and status bar stay instant.
	_ = hawkconfig.CompiledCatalogV1()
	hawkconfig.RefreshConfigCredSnapshot(context.Background())

	// Initialize plugin runtime
	pr := plugin.NewRuntime()
	_ = pr.LoadAll()
	pr.RegisterHooks()
	m.pluginRuntime = pr

	// Welcome message inside TUI
	var dockerRunning *bool
	if m.containerEnabled {
		ok := sandbox.DockerAvailable()
		dockerRunning = &ok
	}
	m.welcomeCache = buildWelcomeMessage(sess, sid, registry, saved, settings, false, initWidth, dockerRunning)
	m.messages = append(m.messages, displayMsg{role: "welcome", content: m.welcomeCache})

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

	m.history = loadInputHistory()
	m.historyIdx = len(m.history)

	return m, nil
}

func (m chatModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.input.Focus(), m.spinner.Tick, blinkTickCmd()}
	if m.containerEnabled {
		m.containerStatus = "checking docker…"
		cwd, _ := os.Getwd()
		cmds = append(cmds, bootContainerCmd(cwd))
	}
	return tea.Batch(cmds...)
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Container failed — block all input except quit
		if m.containerEnabled && m.containerErr != nil {
			if msg.String() == "ctrl+c" || msg.String() == "q" {
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}
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
			case "a", "A":
				m.permReq.Response <- true
				m.session.Perm.Memory.AlwaysAllow(m.permReq.ToolName)
				m.messages = append(m.messages, displayMsg{role: "system", content: "✓ Always allowed: " + m.permReq.ToolName})
				m.permReq = nil
			}
			m.viewDirty = true
			m.updateViewportContent()
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
				m.viewDirty = true
				m.updateViewportContent()
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
					m.viewDirty = true
					m.updateViewportContent()
					return m, nil
				}
				m.saveSession()
				m.quitting = true
				return m, tea.Quit
			}
			if msg.Type == tea.KeyEsc {
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
				}
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			}
			// Allow typing in input while streaming
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		if m.configOpen {
			switch msg.Type {
			case tea.KeyCtrlC:
				if time.Since(m.lastCtrlC) < 1*time.Second {
					m.saveSession()
					m.quitting = true
					return m, tea.Quit
				}
				m.lastCtrlC = time.Now()
				m.messages = append(m.messages, displayMsg{role: "system", content: "Press Ctrl+C again to quit."})
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			default:
				next, cmd := m.handleConfigKey(msg)
				next.viewDirty = true
				next.updateViewportContent()
				return next, cmd
			}
		}
		switch msg.Type {
		case tea.KeyCtrlN:
			models := configModelChoices(m.configModelOptions, false)
			if len(models) > 1 {
				current := m.session.Model()
				idx := 0
				for i, md := range models {
					if md == current {
						idx = (i + 1) % len(models)
						break
					}
				}
				m.session.SetModel(models[idx])
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Model → %s", models[idx])})
			}
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		case tea.KeyCtrlL:
			modes := []string{"default", "acceptEdits", "bypassPermissions"}
			current := string(m.session.Mode)
			idx := 0
			for i, md := range modes {
				if md == current {
					idx = (i + 1) % len(modes)
					break
				}
			}
			_ = m.session.SetPermissionMode(modes[idx])
			labels := map[string]string{"default": "Off", "acceptEdits": "Auto-edit", "bypassPermissions": "Full Auto"}
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Autonomy → %s", labels[modes[idx]])})
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		case tea.KeyCtrlC:
			if time.Since(m.lastCtrlC) < 1*time.Second {
				m.saveSession()
				m.quitting = true
				return m, tea.Quit
			}
			m.lastCtrlC = time.Now()
			m.messages = append(m.messages, displayMsg{role: "system", content: "Press Ctrl+C again to quit."})
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		case tea.KeyTab:
			// Accept ghost text suggestion if active and input is empty
			if m.ghostText.Active() && strings.TrimSpace(m.input.Value()) == "" {
				accepted := m.ghostText.Accept()
				m.input.SetValue(accepted)
				m.input.CursorEnd()
				return m, nil
			}
			sugs := m.slashSuggestionsFor(m.input.Value())
			if len(sugs) > 0 {
				if m.slashSel < 0 || m.slashSel >= len(sugs) {
					m.slashSel = 0
				}
				m.input.SetValue(applySlashSuggestion(sugs[m.slashSel]))
				m.input.CursorEnd()
				return m, nil
			}
		case tea.KeyUp:
			sugs := m.slashSuggestionsFor(m.input.Value())
			if len(sugs) > 0 {
				if m.slashSel <= 0 {
					m.slashSel = len(sugs) - 1
				} else {
					m.slashSel--
				}
				return m, nil
			}
			if len(m.history) > 0 {
				if m.historyIdx == len(m.history) {
					m.historyDraft = m.input.Value()
				}
				if m.historyIdx > 0 {
					m.historyIdx--
					m.input.SetValue(m.history[m.historyIdx])
					m.input.CursorEnd()
				}
			}
			return m, nil
		case tea.KeyDown:
			sugs := m.slashSuggestionsFor(m.input.Value())
			if len(sugs) > 0 {
				m.slashSel = (m.slashSel + 1) % len(sugs)
				return m, nil
			}
			if m.historyIdx < len(m.history)-1 {
				m.historyIdx++
				m.input.SetValue(m.history[m.historyIdx])
				m.input.CursorEnd()
			} else if m.historyIdx == len(m.history)-1 {
				m.historyIdx = len(m.history)
				m.input.SetValue(m.historyDraft)
				m.input.CursorEnd()
			}
			return m, nil
		case tea.KeyEsc:
			if len(m.slashSuggestionsFor(m.input.Value())) > 0 {
				m.slashSel = 0
				return m, nil
			}
		case tea.KeyEnter:
			if m.containerEnabled && m.containerErr != nil {
				return m, nil
			}
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			if sugs := m.slashSuggestionsFor(text); len(sugs) > 0 {
				if m.slashSel < 0 || m.slashSel >= len(sugs) {
					m.slashSel = 0
				}
				m.input.SetValue(applySlashSuggestion(sugs[m.slashSel]))
				m.input.CursorEnd()
				return m, nil
			}
			m.history = append(m.history, text)
			m.historyIdx = len(m.history)
			m.historyDraft = ""
			m.input.Reset()
			if strings.HasPrefix(text, "/") {
				result, cmd := m.handleCommand(text)
				m.viewDirty = true
				m.updateViewportContent()
				return result, cmd
			}
			// Shell escape: !command runs directly without AI
			if strings.HasPrefix(text, "!") {
				m.termCtx.MarkCommand(text[1:])
				return m.handleShellEscape(text[1:])
			}
			// Mode-aware classification: in auto mode, classify input
			classification := m.modeManager.ClassifyWithMode(text)
			if classification == shellmode.ClassShell && !strings.HasPrefix(text, "!") {
				// Auto-detected as shell command — execute directly
				m.termCtx.MarkCommand(text)
				return m.handleShellEscape(text)
			}
			// ClassAgent or ClassNeutral → route to AI
			if setup := hawkconfig.EvaluateSetupCached(context.Background()); setup.NeedsSetup {
				hint := setup.Hint
				if hint == "" {
					hint = "Complete setup in /config (API key and model) before chatting."
				}
				m.messages = append(m.messages, displayMsg{role: "system", content: hint})
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			}
			// @ mention: resolve file references and include as context.
			text = m.handleMentions(text)
			// Build delta-based terminal context for the query
			text = m.termCtx.BuildContext(text)
			// Scale-adaptive: classify task complexity
			scale := engine.ClassifyScale(text)
			behavior := engine.GetBehavior(scale)
			_ = behavior // used for future turn limiting
			// Inject self-improvement lessons
			if lessons := m.selfImprover.ForPrompt(5); lessons != "" {
				m.session.AppendSystemContext(lessons)
			}
			// Inject coding soul
			if soul := m.codingSoul.ForPrompt(); soul != "" {
				m.session.AppendSystemContext(soul)
			}
			// Load hints from CWD
			cwd, _ := os.Getwd()
			if hints := m.hintsLoader.LoadHints(cwd); hints != "" {
				m.session.AppendSystemContext(hints)
			}
			m.messages = append(m.messages, displayMsg{role: "user", content: text})
			m.session.AddUser(text)
			if m.wal != nil {
				_ = m.wal.Append(session.Message{Role: "user", Content: text})
			}
			m.waiting = true
			m.autoScroll = true
			m.viewDirty = true
			m.spinnerVerb = spinnerVerbs[rand.Intn(len(spinnerVerbs))]
			m.partial.Reset()
			m.startStream()
			return m, nil
		}

	case modelsFetchedMsg:
		m.configSaving = false
		if msg.err != nil {
			if m.configOpen {
				m.configNotice = sanitizeConfigNotice(eyrieclient.FormatSetupError(msg.provider, msg.err))
				m.viewDirty = true
				m.updateViewportContent()
			}
			return m, nil
		}
		if len(msg.options) > 0 {
			m.configModelOptions = msg.options
			if msg.provider != "" {
				modelCacheMu.Lock()
				modelCache[msg.provider] = msg.options
				modelCacheMu.Unlock()
			}
			if m.configOpen && strings.Contains(m.configNotice, "Loading") {
				m.configNotice = ""
			}
		} else if m.configOpen {
			m.configNotice = hawkconfig.CatalogEmptyHint(context.Background())
		}
		if m.configOpen {
			m.viewDirty = true
			m.updateViewportContent()
		}
		return m, nil

	case configApplyCredentialsMsg:
		next, cmd := m.handleConfigApplyCredentialsMsg(msg)
		if m.configOpen {
			next.viewDirty = true
			next.updateViewportContent()
		}
		return next, cmd

	case configGatewayRefreshMsg:
		next := m.handleConfigGatewayRefreshMsg(msg)
		if m.configOpen {
			next.viewDirty = true
			next.updateViewportContent()
		}
		return next, nil

	case configKeyResolvedMsg:
		next, cmd := m.handleConfigKeyResolvedMsg(msg)
		if m.configOpen {
			next.viewDirty = true
			next.updateViewportContent()
		}
		return next, cmd

	case configRemoveCredentialMsg:
		next, cmd := m.handleConfigRemoveCredentialMsg(msg)
		if m.configOpen {
			next.viewDirty = true
			next.updateViewportContent()
		}
		return next, cmd

	case loopTickMsg:
		if !m.waiting {
			result, cmd := m.handleCommand(msg.command)
			m.viewDirty = true
			m.updateViewportContent()
			return result, cmd
		}
		return m, nil

	case streamChunkMsg:
		m.partial.WriteString(string(msg))
		m.markPartialDirty()
		return m, nil

	case thinkingMsg:
		m.messages = append(m.messages, displayMsg{role: "thinking", content: string(msg)})
		m.viewDirty = true
		return m, nil

	case toolUseMsg:
		if m.partial.Len() > 0 {
			m.messages = append(m.messages, displayMsg{role: "assistant", content: m.partial.String()})
			m.partial.Reset()
		}
		m.messages = append(m.messages, displayMsg{role: "tool_use", content: msg.name})
		m.toolStartTime = time.Now()
		m.viewDirty = true
		return m, nil

	case toolResultMsg:
		content := msg.content
		totalLen := len(content)
		if totalLen > 800 {
			content = content[:800] + fmt.Sprintf(" … (%d more chars)", totalLen-800)
		}
		m.messages = append(m.messages, displayMsg{role: "tool_result", content: fmt.Sprintf("[%s] %s", msg.name, content)})
		m.viewDirty = true
		return m, nil

	case permissionAskMsg:
		m.permReq = &msg.req
		m.messages = append(m.messages, displayMsg{role: "permission", content: msg.req.Summary})
		return m, nil

	case askUserMsg:
		m.askReq = &msg
		m.messages = append(m.messages, displayMsg{role: "question", content: "❓ " + msg.question})
		m.viewDirty = true
		m.input.Focus()
		m.input.SetValue("")
		return m, nil

	case streamDoneMsg:
		m.flushPartialDirty()
		if m.partial.Len() > 0 {
			content := sanitizeIdentity(m.partial.String())
			m.messages = append(m.messages, displayMsg{role: "assistant", content: content})
			if m.wal != nil {
				_ = m.wal.Append(session.Message{Role: "assistant", Content: content})
			}
			// Generate ghost text suggestion from AI response
			m.ghostText.Suggest(content)
			m.partial.Reset()
		}
		// Inline cost summary after each response
		cost := m.session.Cost.Total()
		tokens := m.session.Cost.PromptTokens + m.session.Cost.CompletionTokens
		elapsed := time.Since(m.toolStartTime)
		if m.toolStartTime.IsZero() {
			elapsed = 0
		}
		if tokens > 0 {
			costLine := fmt.Sprintf("$%.3f • %s • %.1fs", cost, formatTokenCount(tokens), elapsed.Seconds())
			m.messages = append(m.messages, displayMsg{role: "usage", content: costLine})
		}
		m.waiting = false
		m.cancel = nil
		m.toolStartTime = time.Time{}
		m.viewDirty = true
		m.input.Focus()
		m.saveSession()
		return m, nil

	case streamErrMsg:
		m.messages = append(m.messages, displayMsg{role: "error", content: friendlyError(msg.err)})
		m.partial.Reset()
		m.waiting = false
		m.cancel = nil
		m.toolStartTime = time.Time{}
		m.viewDirty = true
		m.input.Focus()
		return m, nil

	case blinkTickMsg:
		m.blinkClosed = !m.blinkClosed
		cmds = append(cmds, blinkTickCmd())
		return m, tea.Batch(cmds...)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(msg.Width - 4)
		m.rebuildWelcomeCache(false)
		m.viewDirty = true

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
		if m.waiting && m.partial.Len() == 0 {
			m.viewDirty = true
		}

	case containerStatusMsg:
		m.containerStatus = msg.status
		m.containerReady = msg.ready
		m.containerErr = msg.err
		if msg.sandbox != nil {
			m.containerSandbox = msg.sandbox
			if m.session != nil {
				m.session.ContainerExecutor = msg.sandbox
			}
		}
		if msg.err != nil {
			m.input.Blur()
		}
		m.rebuildWelcomeCache(m.blinkClosed)
		m.viewDirty = true
		m.updateViewportContent()
	}

	if !m.waiting {
		// Clear ghost text when user starts typing
		if m.ghostText.Active() && m.input.Value() != "" {
			m.ghostText.Clear()
		}
		// Vim mode key interception (operates on full textarea value)
		if m.vim != nil && m.vim.IsEnabled() {
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				text := m.input.Value()
				// textarea doesn't expose cursor column; use text length as approximation
				cursor := len(text)
				newText, newCursor, consumed := m.vim.HandleKey(keyMsg, text, cursor)
				if consumed {
					if newText != text {
						m.input.SetValue(newText)
					}
					m.input.SetCursor(newCursor)
				}
				if consumed && m.vim.Mode == VimNormal {
					return m, tea.Batch(cmds...)
				}
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}
	if !m.input.Focused() {
		cmds = append(cmds, m.input.Focus())
	}

	// Update viewport for scroll events (mouse wheel, page up/down)
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	// If user scrolled away from bottom, disable auto-scroll.
	// Re-enable when they scroll back to bottom.
	if m.viewport.AtBottom() {
		m.autoScroll = true
	} else {
		m.autoScroll = false
	}

	// Update viewport content when messages change or input layout shifts (slash menu / multiline).
	if m.viewDirty || m.syncInputLayout() {
		m.updateViewportContent()
	}

	return m, tea.Batch(cmds...)
}

func runChat() error {
	startBackgroundCatalogRefresh(context.Background())

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

	p := tea.NewProgram(m)
	// Suppress library log output (e.g. eyrie retry warnings) from corrupting the TUI.
	log.SetOutput(io.Discard)
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

	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	fm, ok := finalModel.(chatModel)
	if !ok {
		return fmt.Errorf("unexpected final model type: %T", finalModel)
	}
	hawkC := "\033[38;2;255;94;14m"
	rst := "\033[0m"

	fmt.Print(fm.welcomeCache)
	fmt.Println()
	for _, msg := range fm.messages {
		switch msg.role {
		case "user":
			fmt.Println(hawkC + "█" + rst + "  " + msg.content)
			fmt.Println()
		case "assistant":
			fmt.Println(hawkC + "⛬ " + rst + msg.content)
			fmt.Println()
		case "system":
			fmt.Println(dimStyle.Render("●  " + msg.content))
			fmt.Println()
		case "error":
			fmt.Println(errorStyle.Render("●  " + msg.content))
			fmt.Println()
		}
	}

	viewWidth := fm.width
	if viewWidth <= 0 {
		viewWidth = 80
	}
	leftBold := "Auto (Off)"
	leftDim := " - all actions require approval"
	rightStatus := fmt.Sprintf("%s %s", fm.session.Provider(), fm.session.Model())
	leftVisLen := len(leftBold) + len(leftDim)
	gap := viewWidth - leftVisLen - len(rightStatus)
	if gap < 1 {
		gap = 1
	}
	fmt.Printf("%s%s%s%s\n",
		lipgloss.NewStyle().Bold(true).Render(leftBold),
		dimStyle.Render(leftDim),
		strings.Repeat(" ", gap),
		dimStyle.Render(rightStatus))

	border := strings.Repeat("─", viewWidth)
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	fmt.Println(borderStyle.Render(border))
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true).Render(">") + " ")
	fmt.Println(borderStyle.Render(border))
	fmt.Println(dimStyle.Render("? for help"))

	if fm.sessionID != "" {
		fmt.Println(dimStyle.Render(fmt.Sprintf("To resume this session, run: hawk --resume %s", fm.sessionID)))
	}
	return nil
}
