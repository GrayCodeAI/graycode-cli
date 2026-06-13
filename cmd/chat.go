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
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/GrayCodeAI/eyrie/runtime"
	"github.com/GrayCodeAI/eyrie/storage"
	"github.com/GrayCodeAI/hawk/internal/bridge/sessioncapture"
	"github.com/GrayCodeAI/hawk/internal/codegraph"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/feature/shellmode"
	"github.com/GrayCodeAI/hawk/internal/feature/taste"
	"github.com/GrayCodeAI/hawk/internal/home"
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/intelligence/repomap"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/startup"
	"github.com/GrayCodeAI/hawk/internal/system/staleness"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// Types, styles, and model struct are in chat_model.go
// Welcome message and config summary helpers are in chat_welcome.go
// Slash command handling and helpers are in chat_commands.go

func essentialTools() []tool.Tool {
	// Core tools needed for basic agent operation - always loaded at startup
	return []tool.Tool{
		tool.BashTool{},
		tool.FileReadTool{},
		tool.FileWriteTool{},
		tool.FileEditTool{},
		tool.StructuredEditTool{},
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
		tool.MultiEditTool{},
	}
}

func optionalTools() []tool.Tool {
	// Specialized tools that can be lazy-loaded on demand
	return []tool.Tool{
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
		tool.DownloadTool{},
		tool.AgenticFetchTool{},
		tool.ImpactTool{},
		tool.GitHistoryTool{},
		tool.CodeGraphTool{},
		tool.NilAwayTool{},
		tool.ReviveTool{},
		tool.MCPLanguageServerTool{},
		tool.SQLTool{},
	}
}

func defaultRegistry(settings hawkconfig.Settings) (*tool.Registry, error) {
	// Load essential tools first for fast startup
	tools := essentialTools()
	if tool.IsPowerShellAvailable() {
		tools = append(tools, tool.PowerShellTool{})
	}
	// Detect project-level MCP servers (supply chain attack vector).
	// Project .hawk/settings.json can be committed to a repo and define
	// arbitrary commands that execute on clone. Gate behind --allow-project-mcp.
	projectMCPServers := hawkconfig.ProjectMCPServers()
	projectMCPNames := make(map[string]bool, len(projectMCPServers))
	for _, cfg := range projectMCPServers {
		if cfg.Name != "" {
			projectMCPNames[cfg.Name] = true
		}
	}
	for _, cfg := range settings.MCPServers {
		if cfg.Name == "" || cfg.Command == "" {
			continue
		}
		if projectMCPNames[cfg.Name] && !allowProjectMCP {
			fmt.Fprintf(os.Stderr, "hawk: skipping project-level MCP server %q (defined in .hawk/settings.json); use --allow-project-mcp to enable\n", cfg.Name)
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
	registry := tool.NewRegistry(filtered...)

	// Lazy-load optional tools in background
	go func() {
		for _, t := range optionalTools() {
			_ = registry.Register(t)
		}
	}()

	return registry, nil
}

func allTools() []tool.Tool {
	t := essentialTools()
	t = append(t, optionalTools()...)
	return t
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
	startup.MarkPhase("newChatModel:total")

	startup.MarkPhase("newChatModel:ui-init")
	ta := textarea.New()
	ta.Placeholder = workInputPlaceholder
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
	ta.FocusedStyle.Base = lipgloss.NewStyle().Foreground(textPrimary)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(textPlaceholder)
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(hawkColor).Bold(true)
	ta.BlurredStyle = ta.FocusedStyle
	ta.Cursor.Style = lipgloss.NewStyle().Foreground(hawkColor)
	ta.Prompt = iconPrompt + " "
	// Enter submits; Shift+Enter inserts newline
	ta.KeyMap.InsertNewline.SetKeys("shift+enter")

	// Secondary textinput for config panel password entry
	ci := textinput.New()
	ci.EchoMode = textinput.EchoNormal

	sp := spinner.New()
	sp.Spinner = spinner.Spinner{Frames: hawkSpinnerFrames, FPS: hawkSpinnerFrameInterval}
	sp.Style = lipgloss.NewStyle().Foreground(hawkColor).Bold(true)
	startup.EndPhase("newChatModel:ui-init")

	startup.MarkPhase("newChatModel:effectiveModelAndProvider")
	effectiveModel, effectiveProvider := effectiveModelAndProvider(settings)
	startup.EndPhase("newChatModel:effectiveModelAndProvider")

	startup.MarkPhase("newChatModel:defaultRegistry")
	registry, err := defaultRegistry(settings)
	if err != nil {
		return chatModel{}, err
	}
	startup.EndPhase("newChatModel:defaultRegistry")

	startup.MarkPhase("newChatModel:newHawkSession")
	sess := newHawkSession(settings, effectiveProvider, effectiveModel, systemPrompt, registry)
	startup.EndPhase("newChatModel:newHawkSession")

	startup.MarkPhase("newChatModel:configureSession")
	syncSessionFromPersistedSelection(sess, settings)
	sess.SetLogger(logger.New(io.Discard, logger.Error))
	if cfgErr := configureSession(sess, settings); cfgErr != nil {
		return chatModel{}, cfgErr
	}
	startup.EndPhase("newChatModel:configureSession")

	startup.MarkPhase("newChatModel:prepareSession")
	sid, saved, err := prepareSession(sess)
	if err != nil {
		return chatModel{}, err
	}
	startup.EndPhase("newChatModel:prepareSession")

	// Initialize conversation DAG for branching support
	startup.MarkPhase("newChatModel:dag")
	if home, err := os.UserHomeDir(); err == nil {
		dagPath := filepath.Join(home, ".hawk", "sessions", "convo.db")
		if dag, err := storage.NewDAG(dagPath, sid); err == nil {
			sess.ConvoDAG = dag
		}
	}
	startup.EndPhase("newChatModel:dag")

	initWidth := 80
	initHeight := 24
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		initWidth = w
		if h > 0 {
			initHeight = h
		}
	}
	vp := viewport.New(initWidth, minChatViewportLines)

	now := time.Now()
	m := chatModel{input: ta, configInput: ci, spinner: sp, viewport: vp, session: sess, registry: registry, settings: settings, ref: ref, sessionID: sid, partial: &strings.Builder{}, spinnerVerb: spinnerVerbs[rand.Intn(len(spinnerVerbs))], width: initWidth, height: initHeight, historyIdx: 0, autoScroll: true, streamFollow: true, uiFocus: focusPrompt, startedAt: now, sessionStartedAt: now, activeSkills: make(map[string]plugin.SmartSkill)}
	applyLiveModelMetadata(sess, effectiveProvider, effectiveModel)

	startup.MarkPhase("newChatModel:commandPalette")
	m.commandPalette = NewCommandPalette(initWidth)
	startup.EndPhase("newChatModel:commandPalette")

	// Pre-warm footer connection line so ctx (e.g. 0k/1.0m) shows on first paint.
	if m.session != nil && m.session.ContextWindowCached > 0 {
		m.connStatusVal = m.buildConnectionStatusPlain()
		m.connStatusKey = m.connStatusFingerprint()
	}
	m.phase = initialUIPhase(m.hasChatMessages(), promptFlag != "")
	m.invalidateInputLayoutCache()
	(&m).refreshInputLayoutIfNeeded()
	m = m.syncViewportMouseWheel().withSyncedLayout()
	m.containerEnabled = shouldUseContainer()
	bindChatSession(sess, sid, m.containerEnabled)
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
	startup.MarkPhase("newChatModel:lacy-features")
	m.termCtx = sessioncapture.NewTerminalContext()
	m.inputIndicator = &InputIndicator{}
	m.ghostText = NewGhostText()
	m.modeManager = shellmode.NewModeManager()
	m.modeManager.LoadPersistedMode()
	m.brailleSpinner = NewBrailleSpinner(SpinnerHawk, "")
	m.brailleSpinner.SetLabel(m.spinnerVerb)
	startup.EndPhase("newChatModel:lacy-features")

	// Initialize BMAD/Aeon features
	startup.MarkPhase("newChatModel:bmad-features")
	m.hintsLoader = engine.NewHintsLoader()
	m.sourceRoots = engine.NewSourceRoots()
	m.selfImprover = engine.NewSelfImprover()
	m.codingSoul = engine.LoadCodingSoul()
	startup.EndPhase("newChatModel:bmad-features")

	// Initialize taste and staleness subsystems.
	startup.MarkPhase("newChatModel:taste-staleness")
	m.stalenessDetector = staleness.NewDetector()
	if store, err := taste.NewStore(""); err == nil {
		cwd, _ := os.Getwd()
		projectID := filepath.Base(cwd)
		if hooks, err := taste.NewHooks(projectID, store); err == nil {
			m.tasteHooks = hooks
		}
	}
	startup.EndPhase("newChatModel:taste-staleness")

	// Initialize write-ahead log for crash recovery
	startup.MarkPhase("newChatModel:wal")
	if wal, err := session.NewWAL(sid); err == nil {
		m.wal = wal
		_ = wal.AppendMeta(effectiveModel, effectiveProvider, "")
	}
	startup.EndPhase("newChatModel:wal")

	// Check for crash recovery
	startup.MarkPhase("newChatModel:crash-recovery")
	if recovered := session.CheckForRecovery(); len(recovered) > 0 {
		home := home.Dir()
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
	startup.EndPhase("newChatModel:crash-recovery")

	// Warm code index in background so first CodeSearch is fast
	go func() {
		if bridge := memory.NewYaadBridge(); bridge != nil && bridge.Ready() {
			_ = bridge.InitCodeIndex()
			bridge.Close()
		}
	}()

	// Prefetch live models for the active provider so footer ctx/pricing stay current.
	go func() {
		providerName := effectiveProvider
		entries, _ := runtime.ListModels(context.Background(), runtime.ListModelsOpts{ProviderID: providerName, Source: runtime.ListSourceAuto})
		opts := configModelOptionsFromEyrie(entries)
		if len(opts) > 0 {
			modelCacheMu.Lock()
			modelCache[providerName] = opts
			modelCacheMu.Unlock()
			if ref != nil {
				ref.Send(modelsFetchedMsg{options: opts, provider: providerName})
			}
		}
	}()

	startup.EndPhase("newChatModel:total")

	// Warm credential + catalog caches so typing and status bar stay instant.
	_ = hawkconfig.CompiledCatalogV1()
	hawkconfig.RefreshConfigCredSnapshot(context.Background())

	// Initialize plugin runtime
	pr := plugin.NewRuntime()
	_ = pr.LoadAll()
	pr.RegisterHooks()

	// Print startup profile if requested (after critical path is done)
	if startupProfileFlag {
		startup.PrintReport()
	}
	m.pluginRuntime = pr

	// Welcome message inside TUI
	var dockerRunning *bool
	if m.containerEnabled {
		ok := sandbox.DockerAvailable()
		dockerRunning = &ok
	}
	m.welcomeCache = buildWelcomeMessage(sess, sid, registry, saved, settings, false, initWidth, dockerRunning, m.phase == phaseWelcomeGate)
	// Welcome scrollback only when skipping the gate (resume / -p). Gate users already saw the splash.
	if m.phase == phaseWork {
		m.messages = append(m.messages, displayMsg{role: "welcome", content: m.welcomeCache})
	}
	m.openConfigOnStart = hawkconfig.NeedsFirstRunSetup(context.Background()) &&
		(saved == nil || len(saved.Messages) == 0)

	// Wire permission system
	sess.PermissionFn = func(req engine.PermissionRequest) {
		ref.Send(permissionAskMsg{req: req})
	}

	// High-risk action gate (network, destructive bash) — additive layer on top
	// of the permission engine; falls back to AskUserFn for confirmation.
	sess.Approval = &engine.ApprovalGate{
		Enabled:        true,
		MaxAutoApprove: engine.AutonomySemi,
	}

	// Wire ask_user tool (5-minute timeout matches permission prompts).
	sess.AskUserFn = func(question string) (string, error) {
		resp := make(chan string, 1)
		ref.Send(askUserMsg{question: question, response: resp})
		select {
		case answer := <-resp:
			return answer, nil
		case <-time.After(5 * time.Minute):
			return "", fmt.Errorf("question timed out")
		}
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

	// --watch: build initial symbol graph and start file watcher for incremental PageRank updates
	if watchFlag {
		cwd, err := os.Getwd()
		if err == nil {
			sg, graphErr := repomap.BuildSymbolGraph(cwd, repomap.Options{
				MaxFiles:  500,
				MaxTokens: 2000,
			})
			if graphErr == nil && sg != nil {
				changes := make(chan string, 100)
				done := make(chan struct{})

				go func() {
					ticker := time.NewTicker(time.Second)
					defer ticker.Stop()
					var pending []string
					for {
						select {
						case <-done:
							if len(pending) > 0 {
								sg.UpdateGraph(cwd, pending)
							}
							return
						case <-ticker.C:
							if len(pending) > 0 {
								sg.UpdateGraph(cwd, pending)
								pending = nil
							}
						case p, ok := <-changes:
							if !ok {
								if len(pending) > 0 {
									sg.UpdateGraph(cwd, pending)
								}
								return
							}
							pending = append(pending, p)
						}
					}
				}()

				fw, watcherErr := repomap.NewFileWatcher(cwd, func(path string) {
					rel, err := filepath.Rel(cwd, path)
					if err == nil {
						select {
						case changes <- rel:
						default:
						}
					}
				})
				if watcherErr == nil {
					var stopOnce sync.Once
					fw.Start()
					m.watcherStop = func() {
						stopOnce.Do(func() {
							fw.Stop()
							close(done)
						})
					}
				} else {
					close(done)
				}
			}
		}
	}

	return m, nil
}

func (m chatModel) Init() tea.Cmd {
	cmds := []tea.Cmd{initTerminalMouseCmd(m.mouseEnabled()), m.spinner.Tick, blinkTickCmd(), spinnerVerbTickCmd()}
	if gw, _ := m.sessionGatewayModel(); strings.TrimSpace(gw) != "" {
		cmds = append(cmds, fetchModelsAsync(gw))
	}
	if m.containerEnabled {
		m.containerStatus = "checking docker…"
		cwd, _ := os.Getwd()
		cmds = append(cmds, bootContainerCmd(cwd))
	}
	if m.phase == phaseWork {
		cmds = append(cmds, m.input.Focus())
	}
	if m.phase == phaseWork && m.openConfigOnStart {
		cmds = append(cmds, func() tea.Msg { return autoOpenConfigMsg{} })
	}
	return tea.Batch(cmds...)
}

// applyPromptArrowKey handles Up/Down in the prompt: slash menu navigation or input history.
// Returns true when the key was consumed so callers skip textarea/updateInput handling.
func (m *chatModel) applyPromptArrowKey(msg tea.KeyMsg) bool {
	if m.uiFocus != focusPrompt || m.configOpen {
		return false
	}
	switch msg.Type {
	case tea.KeyUp, tea.KeyDown:
	default:
		return false
	}
	sugs := m.slashSuggestionsFor(m.input.Value())
	if len(sugs) > 0 {
		switch msg.Type {
		case tea.KeyUp:
			if m.slashSel <= 0 {
				m.slashSel = len(sugs) - 1
			} else {
				m.slashSel--
			}
		case tea.KeyDown:
			m.slashSel = (m.slashSel + 1) % len(sugs)
		}
		return true
	}
	switch msg.Type {
	case tea.KeyUp:
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
		return true
	case tea.KeyDown:
		if m.historyIdx < len(m.history)-1 {
			m.historyIdx++
			m.input.SetValue(m.history[m.historyIdx])
			m.input.CursorEnd()
		} else if m.historyIdx == len(m.history)-1 {
			m.historyIdx = len(m.history)
			m.input.SetValue(m.historyDraft)
			m.input.CursorEnd()
		}
		return true
	}
	return false
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.MouseMsg:
		if m.mouseEnabled() {
			if tea.MouseEvent(msg).IsWheel() {
				m.trackMousePosition(msg)
				cmds = append(cmds, m.applyMouseScroll(msg))
				m.sanitizeInputIfNeeded()
				m = m.syncViewportMouseWheel().withSyncedLayout()
				if m.syncInputLayout() {
					m.updateViewportContent()
				}
				if focus := m.ensurePromptInputFocus(); focus != nil {
					cmds = append(cmds, focus)
				}
			} else {
				// Motion events (?1003): track pointer only — avoid layout/sanitize/focus per move.
				m.trackMousePosition(msg)
			}
		}
		return m, tea.Batch(cmds...)

	case autoOpenConfigMsg:
		if !m.openConfigOnStart || m.configOpen {
			return m, nil
		}
		m.openConfigOnStart = false
		return m.openConfigPanel()
	case tea.KeyMsg:
		// Ctrl+\ enters native terminal selection mode. Available in every UI
		// state (welcome gate, permissions, prompt, scrollback) so users always
		// have a way to copy text out of the chat — the alt-screen +
		// mouse-tracking combination otherwise breaks native text selection.
		if msg.Type == tea.KeyCtrlBackslash {
			return m, enterSelectionMode(m.ref, m.copyableTranscript(), m.mouseEnabled())
		}
		if isCopyToClipboardKey(msg) {
			return m.handleCopyShortcut()
		}
		if isMouseSequenceLeak(msg) {
			if handled, cmd := m.tryScrollFromMouseLeak(msg); handled {
				m.sanitizeInputIfNeeded()
				if focus := m.ensurePromptInputFocus(); focus != nil {
					return m, tea.Batch(cmd, focus)
				}
				return m, cmd
			}
			m.sanitizeInputIfNeeded()
			if focus := m.ensurePromptInputFocus(); focus != nil {
				return m, focus
			}
			return m, nil
		}
		if next, cmd, handled := m.handleWelcomeGateKey(msg); handled {
			return next, cmd
		}

		// Command palette (Ctrl+K) — intercept all input when open
		if m.commandPalette != nil && m.commandPalette.IsOpen() {
			action, handled := m.commandPalette.Update(msg)
			if handled {
				if action != "" {
					// Execute the selected command
					m.commandPalette.Close()
					result, _ := m.handleCommand(action)
					if cm, ok := result.(chatModel); ok {
						m = cm
					}
					m.viewDirty = true
					m.updateViewportContent()
				}
				return m, nil
			}
		}

		if m.manualCompacting {
			if isCompactCancelKey(msg) {
				return m.cancelManualCompact("Compaction cancelled.")
			}
			if msg.Type == tea.KeyEnter {
				return m, nil
			}
			// Allow typing in the input while compaction runs (Esc cancels).
		}

		if m.inScrollbackFocus() {
			switch msg.Type {
			case tea.KeyTab:
				return m.cycleUIFocus()
			case tea.KeyEsc:
				m.uiFocus = focusPrompt
				m.viewDirty = true
				return m, m.input.Focus()
			}
			if scrolled, cmd := m.applyViewportScroll(msg); scrolled {
				return m, cmd
			}
			if m.routeKeyToViewport(msg) {
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				if m.viewport.AtBottom() {
					m.autoScroll = true
				} else {
					m.autoScroll = false
				}
				return m, cmd
			}
			return m, nil
		}

		if scrolled, cmd := m.applyViewportScroll(msg); scrolled {
			return m, cmd
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
			return m, m.updateInput(msg)
		}
		if m.waiting {
			if msg.Type == tea.KeyCtrlC {
				// First Ctrl+C cancels stream, second quits
				if m.cancel != nil {
					m.cancel()
					m.cancel = nil
					m.streamCancelled = true
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
				if m.watcherStop != nil {
					m.watcherStop()
				}
				m.quitting = true
				return m, tea.Quit
			}
			if msg.Type == tea.KeyEsc {
				if m.cancel != nil {
					m.cancel()
					m.cancel = nil
					m.streamCancelled = true
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
			// Queue message on Enter while agent is working
			if msg.Type == tea.KeyEnter {
				text := strings.TrimSpace(m.input.Value())
				if text != "" {
					m.messageQueue = append(m.messageQueue, text)
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("📩 Queued: %s", text)})
					m.input.Reset()
					m.viewDirty = true
					m.updateViewportContent()
				}
				return m, nil
			}
			if m.applyPromptArrowKey(msg) {
				return m, nil
			}
			return m, m.updateInput(msg)
		}
		if m.configOpen {
			switch msg.Type {
			case tea.KeyCtrlC:
				if time.Since(m.lastCtrlC) < 1*time.Second {
					m.saveSession()
					if m.watcherStop != nil {
						m.watcherStop()
					}
					m.quitting = true
					return m, tea.Quit
				}
				m.lastCtrlC = time.Now()
				m.messages = append(m.messages, displayMsg{role: "system", content: quitAgainMsg})
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
		case tea.KeyCtrlA:
			// Toggle the Agent Status HUD overlay.
			m.hudOpen = !m.hudOpen
			if m.hudOpen {
				m.hudData = m.collectHUDData()
			}
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		case tea.KeyCtrlK:
			// Open command palette
			if m.commandPalette == nil {
				m.commandPalette = NewCommandPalette(m.width)
			}
			m.commandPalette.Open()
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
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
			if m.containerEnabled && !m.containerReady {
				m.messages = append(m.messages, displayMsg{role: "system", content: "Waiting for sandbox — tiers unlock when container is ready."})
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			}
			next := nextAutonomyTier(m.session.Autonomy)
			if m.session.Autonomy == 0 || autonomyTierIndex(m.session.Autonomy) < 0 {
				next = DefaultContainerAutonomy
			}
			m.session.Autonomy = next
			m.invalidateConnStatus()
			m.messages = append(m.messages, displayMsg{role: "system", content: formatAutonomyTierMessage(next)})
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		case tea.KeyCtrlC:
			if time.Since(m.lastCtrlC) < 1*time.Second {
				m.saveSession()
				if m.watcherStop != nil {
					m.watcherStop()
				}
				m.quitting = true
				return m, tea.Quit
			}
			m.lastCtrlC = time.Now()
			m.messages = append(m.messages, displayMsg{role: "system", content: quitAgainMsg})
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
			return m.cycleUIFocus()
		case tea.KeyUp, tea.KeyDown:
			if m.applyPromptArrowKey(msg) {
				return m, nil
			}
		case tea.KeyEsc:
			if len(m.slashSuggestionsFor(m.input.Value())) > 0 {
				m.slashSel = 0
				return m, nil
			}
		case tea.KeyEnter:
			return m.submitUserMessage()
		}

	case modelsFetchedMsg:
		m.configSaving = false
		if msg.err != nil {
			if m.configOpen {
				m.configNotice = sanitizeConfigNotice(hawkconfig.FormatConfigProviderError(msg.provider, msg.err))
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
		if m.session != nil && msg.provider != "" {
			gw, _ := m.sessionGatewayModel()
			if gw == "" {
				gw = msg.provider
			}
			if strings.TrimSpace(gw) == strings.TrimSpace(msg.provider) {
				applyLiveModelMetadata(m.session, gw, m.session.Model())
			}
		}
		m.invalidateConnStatus()
		m.viewDirty = true
		if m.configOpen {
			if m.configTab == configTabModels {
				m = m.focusConfigActiveModelSelection()
			}
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
		if m.compacting && !m.manualCompacting {
			m.compacting = false
			m.brailleSpinner.SetLabel(m.spinnerVerb)
		}
		m.turnHadAssistantOutput = true
		m.partial.WriteString(string(msg))
		m.markPartialDirty()
		if m.viewDirty {
			m.updateViewportContent()
		}
		return m, nil

	case thinkingMsg:
		m.turnSawThinking = true
		return m, nil

	case streamRetryMsg:
		m.partial.Reset()
		m.messages = stripCurrentTurnThinking(m.messages)
		m.turnSawThinking = false
		m.turnHadAssistantOutput = false
		m.turnHadToolActivity = false
		m.messages = append(m.messages, displayMsg{role: "system", content: "↻ " + msg.content})
		m.viewDirty = true
		return m, nil

	case toolUseMsg:
		m.turnHadToolActivity = true
		if m.partial.Len() > 0 {
			m.messages = append(m.messages, displayMsg{role: "assistant", content: m.partial.String()})
			m.partial.Reset()
		}
		m.messages = append(m.messages, displayMsg{role: "tool_use", content: msg.name})
		m.toolStartTime = time.Now()
		m.viewDirty = true
		return m, nil

	case toolResultMsg:
		m.turnHadToolActivity = true
		m.messages = append(m.messages, displayMsg{role: "tool_result", content: fmt.Sprintf("[%s] %s", msg.name, msg.content)})
		m.viewDirty = true
		return m, nil

	case blastRadiusMsg:
		m.messages = append(m.messages, displayMsg{role: "system", content: msg.message})
		m.viewDirty = true
		return m, nil

	case selectionResumedMsg:
		// Returned from enterSelectionMode. The terminal has been
		// restored; just trigger a redraw so the viewport reflects the
		// state that was visible before selection.
		m.viewDirty = true
		m.updateViewportContent()
		return m, nil

	case permissionAskMsg:
		m.permReq = &msg.req
		m.messages = append(m.messages, displayMsg{role: "permission", content: msg.req.Summary})
		m.viewDirty = true
		return m, nil

	case askUserMsg:
		m.askReq = &msg
		m.messages = append(m.messages, displayMsg{role: "question", content: "❓ " + msg.question})
		m.viewDirty = true
		m.input.Focus()
		m.input.SetValue("")
		return m, nil

	case usageUpdateMsg:
		if msg.usage != nil {
			m.turnInputTokens += msg.usage.PromptTokens
			m.turnOutputTokens += msg.usage.CompletionTokens
			m.invalidateConnStatus()
			m.viewDirty = true
		}
		return m, nil

	case compactTickMsg:
		if m.manualCompacting {
			if m.brailleSpinner != nil {
				m.brailleSpinner.Tick()
			}
			m.viewDirty = true
			m.updateViewportContent()
			localCmds := []tea.Cmd{compactTickCmd()}
			if !m.input.Focused() {
				localCmds = append(localCmds, m.input.Focus())
			}
			return m, tea.Batch(localCmds...)
		}
		return m, nil

	case compactDoneMsg:
		return m.finishManualCompact(msg)

	case compactStartMsg:
		if !m.manualCompacting {
			m.compacting = true
			m.brailleSpinner.SetLabel("Compacting context")
			m.viewDirty = true
		}
		return m, nil

	case compactMsg:
		m.compacting = false
		m.brailleSpinner.SetLabel(m.spinnerVerb)
		line := fmt.Sprintf(
			"Context compacted (%s): ~%s → ~%s tokens",
			msg.strategy,
			formatHawkTokenCount(msg.tokensBefore),
			formatHawkTokenCount(msg.tokensAfter),
		)
		m.messages = append(m.messages, displayMsg{role: "system", content: line})
		m.invalidateConnStatus()
		m.viewDirty = true
		return m, nil

	case streamDoneMsg:
		if m.streamCancelled {
			m.streamCancelled = false
			m.waiting = false
			m.cancel = nil
			m.toolStartTime = time.Time{}
			m.viewDirty = true
			return m, nil
		}
		if m.compacting {
			m.compacting = false
			m.brailleSpinner.SetLabel(m.spinnerVerb)
		}
		m.invalidateConnStatus()
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
		} else if m.turnSawThinking && !m.turnHadAssistantOutput && !m.turnHadToolActivity {
			// Model sent reasoning tokens but no answer — common with reasoning
			// models when the provider drops the post-reasoning content.
			m.messages = append(m.messages, displayMsg{
				role:    "error",
				content: friendlyError(fmt.Errorf("error_only_reasoning: model produced reasoning but no answer")),
			})
		}
		m.turnSawThinking = false
		m.turnHadAssistantOutput = false
		m.turnHadToolActivity = false
		m.waiting = false
		m.cancel = nil
		m.toolStartTime = time.Time{}
		m.viewDirty = true
		m.input.Focus()
		m.saveSession()

		// Process queued messages
		if len(m.messageQueue) > 0 {
			nextMsg := m.messageQueue[0]
			m.messageQueue = m.messageQueue[1:]
			m.messages = append(m.messages, displayMsg{role: "user", content: nextMsg})
			m.session.AddUser(nextMsg)
			m.waiting = true
			m.autoScroll = true
			m.viewDirty = true
			m.spinnerVerb = spinnerVerbs[rand.Intn(len(spinnerVerbs))]
			m.brailleSpinner.SetLabel(m.spinnerVerb)
			m.turnSawThinking = false
			m.turnHadAssistantOutput = false
			m.turnHadToolActivity = false
			m.turnInputTokens = 0
			m.turnOutputTokens = 0
			m.startedAt = time.Time{}
			m.partial.Reset()
			m.startStream()
		}

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
		m.rebuildWelcomeCache(m.blinkClosed)
		m.viewDirty = true
		cmds = append(cmds, blinkTickCmd())
		return m, tea.Batch(cmds...)

	case spinnerVerbTickMsg:
		cmds = append(cmds, spinnerVerbTickCmd())
		if m.waiting && m.partial.Len() == 0 {
			m.spinnerVerb = spinnerVerbs[rand.Intn(len(spinnerVerbs))]
			m.brailleSpinner.SetLabel(m.spinnerVerb)
			m.viewDirty = true
		}
		return m, tea.Batch(cmds...)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.onWelcomeGate() {
			m.input.SetWidth(msg.Width - 4)
		}
		m.invalidateInputLayoutCache()
		m.rebuildWelcomeCache(false)
		m.viewDirty = true
		m.refreshInputLayoutIfNeeded()
		m = m.withSyncedLayout()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.waiting && m.partial.Len() == 0 {
			m.brailleSpinner.Tick()
			// Lazy-init startedAt here (Update path) so the spinner
			// line's elapsed timer has a reference point. Kept out of
			// the View path so render stays a pure function.
			if m.startedAt.IsZero() {
				m.startedAt = time.Now()
			}
			// Lerp the displayed token counters toward the engine's
			// actual numbers — also done here, not in View.
			m.displayInTok += (float64(m.tokenInputTarget()) - m.displayInTok) * 0.10
			m.displayOutTok += (float64(m.tokenOutputTarget()) - m.displayOutTok) * 0.10
			m.viewDirty = true
		}
		cmds = append(cmds, cmd)

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
		if msg.ready && m.session != nil {
			if m.session.Autonomy == 0 {
				m.session.Autonomy = DefaultContainerAutonomy
			}
			if m.phase == phaseWelcomeGate {
				m.sandboxReadyPending = true
			} else {
				m.messages = append(m.messages, displayMsg{role: "system", content: formatSandboxReadyAutonomyMessage(m.session.Autonomy)})
			}
			m.invalidateConnStatus()
		}
		if msg.err != nil {
			// Fall back to host mode so chat still works (container is optional).
			m.containerEnabled = false
			m.containerReady = false
			if m.session != nil {
				m.session.ContainerRequired = false
				m.session.ContainerExecutor = nil
			}
			m.messages = append(m.messages, displayMsg{
				role:    "system",
				content: "Container unavailable — running on host. " + msg.err.Error(),
			})
			m.input.Focus()
		}
		m.rebuildWelcomeCache(m.blinkClosed)
		m.viewDirty = true
		m.updateViewportContent()
	}

	if !m.waiting && m.uiFocus == focusPrompt {
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
		if shouldForwardToInput(msg) {
			cmds = append(cmds, m.updateInput(msg))
		}
	}
	if m.uiFocus == focusPrompt && !m.input.Focused() {
		cmds = append(cmds, m.input.Focus())
	}

	layoutChanged := m.refreshInputLayoutIfNeeded()
	if layoutChanged {
		m = m.withSyncedLayout()
	}
	if m.viewDirty || layoutChanged {
		m.updateViewportContent()
	}

	return m, tea.Batch(cmds...)
}

// autoIndexCodegraph runs codegraph indexing in the background on startup.
// Only indexes if .codegraph/ already exists (user has initialized it before).
// Uses Sync for incremental updates (only re-indexes changed files).
func autoIndexCodegraph() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	dbPath := filepath.Join(cwd, ".codegraph", "codegraph.db")
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		return // Not initialized, skip
	}

	cg, err := codegraph.Open(cwd)
	if err != nil {
		return
	}
	defer cg.Close() //nolint:errcheck // best-effort close in background goroutine

	// Incremental sync — only processes changed files
	if _, err := cg.Sync(); err != nil {
		log.Printf("codegraph sync: %v", err)
	}
}

func runChat() error {
	startBackgroundCatalogRefresh(context.Background())

	// Auto-index codegraph in background if .codegraph exists
	go autoIndexCodegraph()

	// One-time, gated codebase analysis for projects with no context file.
	// Runs in the background and never blocks startup; fully opt-out via
	// HAWK_DISABLE_AUTO_INIT. No-op for projects that already have context.
	maybeAutoInit(context.Background())

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
		m.turnSawThinking = false
		m.turnHadAssistantOutput = false
		m.turnHadToolActivity = false
		m.waiting = true
	}

	programOpts := []tea.ProgramOption{tea.WithAltScreen()}
	if m.mouseEnabled() {
		programOpts = append(programOpts, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(m, programOpts...)
	// Suppress library log output (e.g. eyrie retry warnings) from corrupting the TUI.
	log.SetOutput(io.Discard)
	ref.Set(p)

	if promptFlag != "" {
		sess := m.session
		ctx, cancel := context.WithCancel(context.Background())
		_ = cancel // will be cancelled when program exits
		go func() {
			ch, streamErr := sess.Stream(ctx)
			if streamErr != nil {
				p.Send(streamErrMsg{err: streamErr})
				return
			}
			pumpStreamEvents(ref, ch)
		}()
	}

	finalModel, err := p.Run()
	writeTerminalMouse(disableMouseCSI)
	if err != nil {
		return err
	}
	fm, ok := finalModel.(chatModel)
	if !ok {
		return fmt.Errorf("unexpected final model type: %T", finalModel)
	}
	if fm.quitting {
		fm.saveSession()
		fmt.Print(formatQuitResumeMessage(fm.sessionID))
		return nil
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
			fmt.Println(hawkC + iconAssistantPrefix + " " + rst + msg.content)
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
	borderStyle := lipgloss.NewStyle().Foreground(borderDim)
	fmt.Println(borderStyle.Render(border))
	fmt.Println(lipgloss.NewStyle().Foreground(hawkColor).Bold(true).Render(">") + " ")
	fmt.Println(borderStyle.Render(border))
	fmt.Println(dimStyle.Render("? for help"))

	if fm.sessionID != "" {
		fmt.Println(dimStyle.Render(fmt.Sprintf("To resume this session, run: hawk --resume %s", fm.sessionID)))
	}
	return nil
}
