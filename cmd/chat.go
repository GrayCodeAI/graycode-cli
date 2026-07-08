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
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/intelligence/repomap"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/startup"
	hawkstorage "github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/GrayCodeAI/hawk/internal/system/staleness"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// Types, styles, and model struct are in chat_model.go
// Welcome message and config summary helpers are in chat_welcome.go
// Slash command handling and helpers are in chat_commands.go
// Tool-registry construction (essential/optional tools) is in chat_tools.go
// The Bubble Tea event loop (Update, applyPromptArrowKey) is in chat_update.go

const workInputPlaceholder = `Ask Hawk to inspect, edit, or run something... (Shift+Enter for newline)`

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
		saved, _, err = session.ResumeSession(resumeID)
	} else {
		cwd, _ := os.Getwd()
		saved, err = session.LoadLatestForCWD(cwd)
	}
	if err != nil {
		return "", nil, err
	}
	sess.LoadMessages(session.ToRuntimeMessages(saved.Messages))
	if forkSessionFlag {
		if sessionIDFlag != "" {
			id = sessionIDFlag
		}
		return id, saved, nil
	}
	return saved.ID, saved, nil
}

func newChatModel(ref *progRef, systemPrompt string, settings hawkconfig.Settings) (chatModel, error) {
	registry, err := defaultRegistry(settings)
	if err != nil {
		return chatModel{}, err
	}
	return newChatModelWithRegistry(ref, systemPrompt, settings, registry)
}

func newChatModelWithRegistry(ref *progRef, systemPrompt string, settings hawkconfig.Settings, registry *tool.Registry) (chatModel, error) {
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
	ta.Prompt = icons.ChevronRight() + " "
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
	selection := startupSelection(settings)
	effectiveModel, effectiveProvider := selection.Model, selection.Provider
	startup.EndPhase("newChatModel:effectiveModelAndProvider")

	startup.MarkPhase("newChatModel:defaultRegistry")
	startup.EndPhase("newChatModel:defaultRegistry")

	startup.MarkPhase("newChatModel:newHawkSession")
	sess := newStartupHawkSession(selection, systemPrompt, registry)
	startup.EndPhase("newChatModel:newHawkSession")

	startup.MarkPhase("newChatModel:configureSession")
	syncSessionFromPersistedSelection(sess)
	sess.SetLogger(logger.New(io.Discard, logger.Error))
	if cfgErr := configureSessionStartup(sess, settings); cfgErr != nil {
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
	dagPath := filepath.Join(hawkstorage.SessionsDir(), "convo.db")
	if dag, err := storage.NewDAG(dagPath, sid); err == nil {
		sess.SetConvoDAG(dag)
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
	m := chatModel{input: ta, configInput: ci, spinner: sp, viewport: vp, session: sess, registry: registry, settings: settings, ref: ref, sessionID: sid, partial: &strings.Builder{}, spinnerVerb: spinnerVerbs[rand.Intn(len(spinnerVerbs))], width: initWidth, height: initHeight, historyIdx: 0, autoScroll: true, streamFollow: true, uiFocus: focusPrompt, startedAt: now, sessionStartedAt: now, activeSkills: make(map[string]plugin.SmartSkill)} // #nosec G404 -- non-cryptographic use (random spinner verb selection)
	applyLiveModelMetadata(sess, effectiveProvider, effectiveModel)

	startup.MarkPhase("newChatModel:commandPalette")
	m.commandPalette = NewCommandPalette(initWidth)
	startup.EndPhase("newChatModel:commandPalette")

	// Give the footer an immediate cwd value without paying for a git probe
	// before first paint. The background warmup fills in branch/provider data.
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		m.statusLeftKey = cwd
		m.statusLeftVal = shortenHomePath(cwd)
		m.statusLeftAt = time.Now()
	}
	m.invalidateInputLayoutCache()
	(&m).refreshInputLayoutIfNeeded()
	m = m.syncViewportMouseWheel().withSyncedLayout()
	m.containerEnabled = shouldUseContainer()
	bindChatSession(sess, sid, m.containerEnabled)
	if m.containerEnabled {
		m.containerStatus = "checking docker…"
	} else {
		applyDefaultHostAutonomy(sess)
		if noContainer {
			m.messages = append(m.messages, displayMsg{
				role: "system",
				content: "--no-container runs tools on the host without sandbox isolation. " +
					"Use default container mode for safer agent execution.",
			})
		}
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

	// Start with an empty plugin runtime so first paint stays fast.
	startup.MarkPhase("newChatModel:plugin-runtime")
	pr := plugin.NewRuntime()
	startup.EndPhase("newChatModel:plugin-runtime")
	m.pluginRuntime = pr

	// Welcome message inside TUI. Use a cheap initial snapshot and let the
	// async warmup fill in setup/agents status after the first frame.
	startup.MarkPhase("newChatModel:welcome")
	quickSnapshot := welcomeStatusSnapshot{}
	m.welcomeSetupState = quickSnapshot.setup
	m.welcomeAgentsOK = quickSnapshot.agentsOK
	m.welcomeCache = buildWelcomeMessageWithSnapshot(sess, sid, registry, saved, settings, len(pr.SmartSkills), false, initWidth, initHeight, nil, quickSnapshot)
	m.messages = append(m.messages, displayMsg{role: "welcome", content: m.welcomeCache})
	startup.EndPhase("newChatModel:welcome")

	// Wire permission system
	sess.PermSvc().SetPermissionFn(func(req engine.PermissionRequest) {
		ref.Send(permissionAskMsg{req: req})
	})

	// High-risk action gate (network, destructive bash) — additive layer on top
	// of the permission engine; falls back to AskUserFn for confirmation.
	sess.SetApproval(&engine.ApprovalGate{
		Enabled:        true,
		MaxAutoApprove: engine.AutonomySemi,
	})

	// Wire ask_user tool (5-minute timeout matches permission prompts).
	sess.SetAskUserFn(func(question string) (string, error) {
		resp := make(chan string, 1)
		ref.Send(askUserMsg{question: question, response: resp})
		select {
		case answer := <-resp:
			return answer, nil
		case <-time.After(5 * time.Minute):
			return "", fmt.Errorf("question timed out")
		}
	})

	if saved != nil {
		for _, sm := range saved.Messages {
			if sm.Role == "user" || sm.Role == "assistant" {
				m.messages = append(m.messages, displayMsg{role: sm.Role, content: sm.Content})
			}
		}
	}

	startup.MarkPhase("newChatModel:history")
	m.history = loadInputHistory()
	m.historyIdx = len(m.history)
	startup.EndPhase("newChatModel:history")

	startup.MarkPhase("newChatModel:first-paint")
	m.primeInitialViewportContent()
	startup.EndPhase("newChatModel:first-paint")

	// Recover interrupted sessions after first paint so cleanup never delays
	// the initial UI.
	go func(currentSessionID string) {
		if recovered := session.CheckForRecovery(); len(recovered) > 0 {
			walDir := hawkstorage.SessionsDir()
			for _, rid := range recovered {
				if rid == currentSessionID {
					continue // current session WAL
				}
				if rs, err := session.RecoverFromWAL(rid); rs != nil && err == nil {
					_ = session.Save(rs)
					_ = os.Remove(filepath.Join(walDir, rid+".wal"))
				}
			}
		}
	}(sid)

	// Warm footer data and the model catalog after the first frame.
	go func(model chatModel) {
		startup.MarkPhase("newChatModel:ui-cache-warm")
		hawkconfig.RefreshConfigCredSnapshot(context.Background())
		welcomeSnapshot := loadWelcomeStatusSnapshot()
		model.refreshStatusBarLeft(true)
		connStatusVal := ""
		connStatusKey := ""
		if model.session != nil {
			connStatusVal = model.buildConnectionStatusPlain()
			connStatusKey = model.connStatusFingerprint()
		}
		startup.EndPhase("newChatModel:ui-cache-warm")
		if model.ref != nil {
			model.ref.Send(startupWarmMsg{
				statusLeftKey:    model.statusLeftKey,
				statusLeftVal:    model.statusLeftVal,
				statusLeftBranch: model.statusLeftBranch,
				connStatusVal:    connStatusVal,
				connStatusKey:    connStatusKey,
				welcomeSetup:     welcomeSnapshot.setup,
				welcomeAgentsOK:  welcomeSnapshot.agentsOK,
			})
		}
	}(m)

	go func() {
		_ = hawkconfig.CompiledCatalogV1()
	}()

	// Load plugins/skills after startup and refresh welcome indicators when ready.
	go func() {
		runtime := plugin.NewRuntime()
		if err := runtime.LoadAll(); err != nil {
			return
		}
		runtime.RegisterHooks()
		if ref != nil {
			ref.Send(pluginRuntimeReadyMsg{runtime: runtime})
		}
	}()

	// --watch: build initial symbol graph and start file watcher for incremental PageRank updates
	startup.MarkPhase("newChatModel:watch")
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
	startup.EndPhase("newChatModel:watch")

	startup.EndPhase("newChatModel:total")

	// Print startup profile after the full synchronous chat init path completes.
	if startupProfileFlag {
		startup.PrintReport()
	}

	return m, nil
}

func (m chatModel) Init() tea.Cmd {
	cmds := []tea.Cmd{initTerminalMouseCmd(m.mouseEnabled()), promptKeepAliveCmd()}
	if gw, _ := m.sessionGatewayModel(); strings.TrimSpace(gw) != "" {
		cmds = append(cmds, fetchModelsAsync(gw))
		if isXiaomiMimoProvider(gw) {
			cmds = append(cmds, fetchPlatformContextIndexCmd())
		}
	}
	if m.containerEnabled {
		m.containerStatus = "checking docker…"
		cwd, _ := os.Getwd()
		cmds = append(cmds, bootContainerCmd(cwd))
	}
	cmds = append(cmds, m.input.Focus())
	return tea.Batch(cmds...)
}

func chatProgramOptions(mouseEnabled bool) []tea.ProgramOption {
	programOpts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithReportFocus()}
	if mouseEnabled {
		programOpts = append(programOpts, tea.WithMouseCellMotion())
	}
	return programOpts
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
	startup.Reset()
	startBackgroundCatalogRefresh(context.Background())

	// Auto-index codegraph in background if .codegraph exists
	go autoIndexCodegraph()

	// One-time, gated codebase analysis for projects with no context file.
	// Runs in the background and never blocks startup; fully opt-out via
	// HAWK_DISABLE_AUTO_INIT. No-op for projects that already have context.
	maybeAutoInit(context.Background())

	ref := &progRef{}
	type startupPromptResult struct {
		text string
		err  error
	}
	type startupSettingsResult struct {
		settings hawkconfig.Settings
		err      error
	}
	type startupRegistryResult struct {
		registry *tool.Registry
		err      error
	}
	promptCh := make(chan startupPromptResult, 1)
	settingsCh := make(chan startupSettingsResult, 1)
	registryCh := make(chan startupRegistryResult, 1)
	go func() {
		startup.MarkPhase("runChat:startup-prompt")
		text, err := buildStartupSystemPrompt()
		startup.EndPhase("runChat:startup-prompt")
		promptCh <- startupPromptResult{text: text, err: err}
	}()
	go func() {
		startup.MarkPhase("runChat:settings")
		settings, err := loadEffectiveSettings()
		if err != nil {
			startup.EndPhase("runChat:settings")
			settingsCh <- startupSettingsResult{err: err}
			registryCh <- startupRegistryResult{err: err}
			return
		}
		startup.EndPhase("runChat:settings")
		settingsCh <- startupSettingsResult{settings: settings}
		startup.MarkPhase("runChat:registry")
		registry, regErr := defaultRegistry(settings)
		startup.EndPhase("runChat:registry")
		registryCh <- startupRegistryResult{registry: registry, err: regErr}
	}()
	promptRes := <-promptCh
	settingsRes := <-settingsCh
	registryRes := <-registryCh
	if promptRes.err != nil {
		return promptRes.err
	}
	if settingsRes.err != nil {
		return settingsRes.err
	}
	if registryRes.err != nil {
		return registryRes.err
	}
	systemPrompt := promptRes.text
	settings := settingsRes.settings
	m, err := newChatModel(ref, systemPrompt, settings)
	if err != nil {
		return err
	}

	if promptFlag != "" {
		if e := (&m).ensureSessionReadyForChat(); e != nil {
			return e
		}
		m.messages = append(m.messages, displayMsg{role: "user", content: promptFlag})
		m.session.AddUser(promptFlag)
		m.turnSawThinking = false
		m.turnHadAssistantOutput = false
		m.turnHadToolActivity = false
		m.waiting = true
	}

	p := tea.NewProgram(m, chatProgramOptions(m.mouseEnabled())...)
	// Suppress library log output (e.g. eyrie retry warnings) from corrupting the TUI.
	log.SetOutput(io.Discard)
	ref.Set(p)

	go func() {
		if extra := strings.TrimSpace(buildDeferredWorkspacePromptContext()); extra != "" {
			ref.Send(systemPromptContextReadyMsg{context: extra})
		}
	}()

	if promptFlag != "" {
		sess := m.session
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
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
	hawkC := ansiOrange
	rst := ansiReset

	fmt.Print(fm.welcomeCache)
	fmt.Println()
	for _, msg := range fm.messages {
		switch msg.role {
		case "user":
			fmt.Println(hawkC + "█" + rst + "  " + msg.content)
			fmt.Println()
		case "assistant":
			fmt.Println(hawkC + icons.Robot() + " " + rst + msg.content)
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
