package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/GrayCodeAI/hawk/internal/bridge/sessioncapture"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/feature/shellmode"
	"github.com/GrayCodeAI/hawk/internal/feature/taste"
	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/system/staleness"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// All hawk color/icon/glyph constants live in theme.go. This file holds
// the pre-built lipgloss styles that combine a color with attributes
// (bold, italic, border, etc.) for the most common patterns.

var (
	// Pre-built styles for the most common patterns. The color constants
	// themselves are defined in theme.go; this block only attaches
	// attributes (bold, italic, border) on top.
	dimStyle     = lipgloss.NewStyle().Foreground(textDisabled)
	errorStyle   = lipgloss.NewStyle().Foreground(errorCoral)
	warnStyle    = lipgloss.NewStyle().Foreground(warnAmber)
	toolStyle    = lipgloss.NewStyle().Foreground(toolGold).Bold(true)
	toolDimStyle = lipgloss.NewStyle().Foreground(textDisabled)

	slashCmdStyle       = lipgloss.NewStyle().Foreground(textDisabled)
	slashDescStyle      = lipgloss.NewStyle().Foreground(textDisabled)
	slashSelCmdStyle    = lipgloss.NewStyle().Foreground(hawkColor).Bold(true)
	slashSelDescStyle   = lipgloss.NewStyle().Foreground(hawkColor)
	inputBorderStyle    = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false, true, false).BorderForeground(borderDim)
	ghostHintStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Italic(true)
	containerErrStyle   = lipgloss.NewStyle().Foreground(errorCoral)
	containerLabelStyle = lipgloss.NewStyle().Foreground(containerBlue)

	// Backwards-compatible alias for callers that still use the old name.
	// New code should use the purpose-named constants in theme.go.
	dimColor = textDisabled
)

// hawkSpinnerFrames feeds the bubbles spinner (matches BrailleSpinner default).
var hawkSpinnerFrames = hawkSpinnerGlyphs

// hawkSpinnerFrameInterval — compass frame cadence.
const hawkSpinnerFrameInterval = 80 * time.Millisecond

// Spinner verbs (from hawk-archive) — picked randomly per session
var spinnerVerbs = []string{
	"Abstracting", "Architecting", "Brewing", "Calculating", "Cogitating",
	"Compiling", "Computing", "Conjuring", "Contemplating", "Cooking",
	"Crafting", "Crunching", "Debugging", "Deciphering", "Deliberating",
	"Distilling", "Elucidating", "Encoding", "Envisioning", "Forging",
	"Generating", "Hatching", "Ideating", "Imagining", "Incubating",
	"Inferencing", "Infusing", "Linting", "Manifesting", "Mulling",
	"Musing", "Optimizing", "Orchestrating", "Parsing", "Pondering",
	"Processing", "Reasoning", "Refactoring", "Refining", "Reticulating",
	"Ruminating", "Scaffolding", "Simmering", "Sketching", "Spelunking",
	"Spinning", "Synthesizing", "Tempering", "Thinking", "Tinkering",
	"Tokenizing", "Transpiling", "Unfurling", "Validating", "Vibing",
	"Weaving", "Whisking", "Wizarding", "Working", "Wrangling",
}

type (
	streamChunkMsg      string
	streamRenderTickMsg struct{}
	streamDoneMsg       struct{}
	streamRetryMsg      struct{ content string }
	streamErrMsg        struct{ err error }
	spinnerVerbTickMsg  struct{}
	promptKeepAliveMsg  struct{}
	usageUpdateMsg      struct{ usage *engine.StreamUsage }
	compactStartMsg     struct{}
	compactMsg          struct {
		strategy                  string
		tokensBefore, tokensAfter int
	}
	compactTickMsg struct{}
	compactDoneMsg struct {
		strategy                  string
		tokensBefore, tokensAfter int
		err                       error
		beforeCount, afterCount   int
	}
)

type (
	modelsFetchedMsg struct {
		options  []configModelOption
		provider string
		err      error
	}
	pluginRuntimeReadyMsg struct {
		runtime *plugin.Runtime
	}
	startupWarmMsg struct {
		statusLeftKey    string
		statusLeftVal    string
		statusLeftBranch string
		connStatusVal    string
		connStatusKey    string
		welcomeSetup     hawkconfig.SetupState
		welcomeAgentsOK  bool
	}
	processArrowTickMsg struct {
		seq int
	}
	systemPromptContextReadyMsg struct {
		context string
	}
	loopTickMsg                struct{ command string }
	toolUseMsg                 struct{ name, id string }
	toolResultMsg              struct{ name, content string }
	permissionAskMsg           struct{ req engine.PermissionRequest }
	permissionPromptTimeoutMsg struct{ seq int }
	thinkingMsg                string
	blastRadiusMsg             struct{ message string }
	askUserMsg                 struct {
		question string
		response chan string
	}
	askUserPromptTimeoutMsg struct{ seq int }
)

type displayMsg struct {
	role      string
	content   string
	timeoutAt time.Time // deadline for permission prompts (zero = none)
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
	input                      textarea.Model
	configInput                textinput.Model // secondary input for config panel password entry
	useConfigInput             bool            // true when config panel needs textinput (e.g. password)
	spinner                    spinner.Model
	viewport                   viewport.Model
	session                    *engine.Session
	registry                   *tool.Registry
	settings                   hawkconfig.Settings
	ref                        *progRef
	cancel                     context.CancelFunc // cancel current stream
	sessionID                  string
	messages                   []displayMsg
	partial                    *strings.Builder
	waiting                    bool
	streamCancelled            bool // user cancelled; suppress late streamDone side effects
	turnSawThinking            bool // current turn received hidden reasoning
	arrowSeq                   int
	pendingArrow               *tea.KeyMsg
	arrowBurstActive           bool
	lastArrowTime              time.Time
	processingGenuineArrow     bool
	turnHadAssistantOutput     bool                      // current turn produced assistant text
	turnHadToolActivity        bool                      // current turn produced tool activity
	messageQueue               []string                  // queued messages while agent is working
	permReq                    *engine.PermissionRequest // pending permission prompt
	permReqSeq                 int
	permTimeoutAt              time.Time   // deadline for the active permission prompt (zero = none)
	askReq                     *askUserMsg // pending ask_user prompt
	askReqSeq                  int
	width                      int
	height                     int
	quitting                   bool
	blinkClosed                bool
	slashSel                   int
	hudOpen                    bool    // Agent Status HUD overlay (Ctrl+A)
	hudData                    HUDData // latest HUD snapshot
	configOpen                 bool
	configTab                  int // configTabGateways, configTabModels
	configSel                  int
	configScroll               int // scroll offset for long lists
	configNotice               string
	configEntry                string              // configEntryNone, configEntryAPIKeyPaste, configEntryOllamaURL
	configProvider             string              // e.g. configProviderOllama while entry overlay is open
	configModelOptions         []configModelOption // labels + ids from eyrie catalog
	configModelProvider        string              // filter models after API key paste
	configModelSearch          string              // active model filter query
	configModelSearchActive    bool                // typing into model search input
	configGuideAfterKey        bool                // open model picker when discover finishes
	configGatewayFocus         int                 // last highlighted gateway row (for refresh action)
	configGatewayRowsCache     []configGatewayRow
	configGatewayRowsDirty     bool
	configKeysPendingRemove    string // provider awaiting delete confirmation
	configKeysRemoveStep       int    // 1 = first prompt, 2 = final confirm
	configReplaceProvider      string // replace-key flow target gateway
	configPostSaveKeysProvider string // return to Gateways tab after replace save
	configSaving               bool   // blocks hub/list input while async credential work runs
	configPendingOllamaURL     string
	configZAIRegionSel         int // Z.AI (general or coding) region picker index
	configGatewayRegionSel     int // Generic gateway region picker index
	pluginRuntime              *plugin.Runtime
	spinnerVerb                string
	// Per-turn token counters shown next to the spinner (↑ input, ↓ output).
	// Reset each time the user submits a message; updated by usageUpdateMsg.
	turnInputTokens          int
	turnOutputTokens         int
	turnEstimatedOutputRunes int
	compacting               bool // stream auto-compact: spinner label = Compacting context
	manualCompacting         bool // user ran /compact: show progress panel above input
	compactBarUsed           int  // context tokens snapshot at /compact start
	compactBarWindow         int  // context window snapshot at /compact start
	compactCancel            context.CancelFunc
	// Display values lerped toward the turn targets each render frame
	// (factor 0.10). Smooths the counter animation.
	displayInTok                 float64
	displayOutTok                float64
	lastCtrlC                    time.Time
	history                      []string
	historyIdx                   int
	historyDraft                 string // unsent text before navigating history
	lastCommand                  string // most recent slash command (for context-aware tips)
	autoScroll                   bool   // whether viewport is pinned to bottom
	streamFollow                 bool   // follow streaming output (Grok-style; toggle with /follow)
	sleepCancel                  func() // cancel function to re-enable sleep (nil if not prevented)
	backgrounded                 bool   // terminal lost focus during current turn (for completion notification)
	notifiedComplete             bool   // completion notification was sent this turn (prevent duplicates)
	uiFocus                      uiFocusArea
	contentLines                 int   // total lines in scrollback content (for footer position)
	lastMouseY                   int   // last pointer row (0-based); -1 = unknown; used when Cursor reports stale wheel Y
	mouseOverride                *bool // runtime /mouse toggle; persisted via settings
	vim                          *VimState
	wal                          *session.WAL
	startedAt                    time.Time // per-turn timer (spinner + turn elapsed)
	sessionStartedAt             time.Time // whole chat session (footer duration)
	sessionBootstrapDone         bool
	toolStartTime                time.Time
	welcomeCache                 string
	welcomeSetupState            hawkconfig.SetupState
	welcomeAgentsOK              bool
	viewDirty                    bool
	layoutKey                    int    // input lines + slash menu height fingerprint
	cachedBottomBarLines         int    // memoized chatBottomBarLines; refresh via refreshInputLayoutIfNeeded
	slashSugInput                string // memoize slashSuggestions per keystroke
	slashSugCache                []string
	slashSugGen                  int             // generation counter; bumped to invalidate slashSugCache
	slashSugCachedGen            int             // generation at the time slashSugCache was computed
	contextualHelp               *ContextualHelp // rich help entries for /help <topic>
	toolResultExpanded           map[int]bool    // per-message index: expanded state for long tool results
	connStatusKey                string          // gateway+model+creds fingerprint
	connStatusVal                string
	deferredSystemContext        string
	deferredSystemContextReady   bool
	deferredSystemContextApplied bool
	partialDirty                 bool // stream text changed since last viewport paint
	lastPartialRender            time.Time
	partialRenderPending         bool
	statusLeftKey                string
	statusLeftVal                string
	statusLeftBranch             string
	statusLeftAt                 time.Time // last branch lookup; refreshed on a short TTL

	// Incremental viewport cache (see chat_viewport_render.go).
	vpStableContent string
	vpRenderedMsgs  int
	vpRenderWidth   int
	vpLastMsgLen    int

	// Cached expanded map for viewport position math (avoids repeated allocations).
	cachedExpandedMap map[int]bool
	cachedExpandedLen int

	// Streaming-partial render cache: rendered output of the completed
	// markdown blocks of m.partial (see renderStreamTail).
	streamMDPrefixRaw string
	streamMDPrefixOut string
	streamMDWidth     int

	activeSkills map[string]plugin.SmartSkill // per-session activated skills

	// Container mode (hermetic execution in Docker container)
	containerEnabled bool
	containerStatus  string // "checking docker…", "pulling image…", "starting…", "<id>", "docker not running"
	containerReady   bool
	containerErr     error
	containerSandbox *sandbox.ContainerSandbox
	// pendingSubmit holds user input entered while the container is still
	// booting. It is auto-submitted when the container becomes ready so the
	// user's message is never silently discarded.
	pendingSubmit string
	// containerRetryable is true after a container boot failure. It enables
	// the [r]etry keybinding so the user can recover without restarting the TUI.
	containerRetryable bool

	// Taste & staleness tracking
	tasteHooks        *taste.Hooks
	stalenessDetector *staleness.Detector

	// Lacy-inspired features
	termCtx        *sessioncapture.TerminalContext
	inputIndicator *InputIndicator
	ghostText      *GhostText
	modeManager    *shellmode.ModeManager
	brailleSpinner *BrailleSpinner
	// testStreamStarter overrides the async stream launcher in tests that
	// manually inject stream events and need deterministic cleanup.
	testStreamStarter func()

	// BMAD/Aeon-inspired features
	hintsLoader  *engine.HintsLoader
	sourceRoots  *engine.SourceRoots
	selfImprover *engine.SelfImprover
	codingSoul   *engine.CodingSoul

	// Loop cancellation
	loopCancel context.CancelFunc // cancels the current /loop goroutine

	// Parallel agents cancellation
	parallelCancel context.CancelFunc // cancels running /parallel agents

	// Background goroutine cancellation
	bgCancel context.CancelFunc // cancels all background goroutines on quit

	// PageRank file watcher
	watcherStop func() // stops the incremental symbol graph file watcher

	// Command palette (Ctrl+K)
	commandPalette *CommandPalette
	autonomyPicker *AutonomyPicker
	specPicker     *SpecPicker
	themePicker    *ThemePicker

	// Input history search (Ctrl+R) — overlay for searching through
	// previous inputs, similar to bash reverse-i-search.
	historySearchOpen     bool
	historySearchInput    string
	historySearchQuery    string
	historySearchFiltered []string
	historySearchSel      int

	// Session picker (Ctrl+S) — fuzzy search through saved sessions
	// with context preview for quick session switching.
	sessionPickerOpen     bool
	sessionPickerInput    string
	sessionPickerEntries  []session.Entry
	sessionPickerFiltered []session.Entry
	sessionPickerSel      int
}

const streamRenderInterval = 50 * time.Millisecond

// maxDisplayMessages bounds the number of messages kept in memory for display.
// Older messages are trimmed to prevent unbounded memory growth in long sessions.
// The session file still contains the full history for /resume.
const maxDisplayMessages = 500

// messageTrimThreshold is the point at which we start trimming old messages.
// We trim in batches to avoid frequent reallocations.
const messageTrimThreshold = 450

// maxPromptHistory bounds the in-memory prompt history ring (M18: history
// grew without bound across a long session).
const maxPromptHistory = 200

// pushHistory records a submitted prompt, capped to the most recent
// maxPromptHistory entries.
func (m *chatModel) pushHistory(text string) {
	m.history = append(m.history, text)
	if len(m.history) > maxPromptHistory {
		keep := len(m.history) - maxPromptHistory
		m.history = append(m.history[:0], m.history[keep:]...)
	}
	m.historyIdx = len(m.history)
	m.historyDraft = ""
}

// maxQueuedMessages bounds the queue of prompts entered while the agent is
// working (M18: it grew without bound during long turns). The oldest queued
// prompts are dropped first so the most recent intent is preserved.
const maxQueuedMessages = 100

// enqueueMessage queues a prompt entered while the agent is working,
// dropping the oldest entries past the cap.
func (m *chatModel) enqueueMessage(text string) {
	if len(m.messageQueue) >= maxQueuedMessages {
		m.messageQueue = append(m.messageQueue[:0], m.messageQueue[1:]...)
	}
	m.messageQueue = append(m.messageQueue, text)
}

// trimOldMessages removes old messages when the count exceeds the threshold.
// Keeps the most recent messages and shows a hint about trimmed history.
func (m *chatModel) trimOldMessages() {
	if len(m.messages) <= messageTrimThreshold {
		return
	}
	// Keep the most recent maxDisplayMessages messages.
	// Trim from the front, but keep the welcome message if present.
	trimCount := len(m.messages) - maxDisplayMessages
	if trimCount <= 0 {
		return
	}

	// Preserve a leading welcome message (index 0) if present — it is
	// re-emitted ahead of the trim hint so the header survives long sessions.
	startIdx := 0
	if m.messages[0].role == "welcome" {
		startIdx = 1
	}

	// Never trim past the end of the slice.
	if startIdx+trimCount >= len(m.messages) {
		return
	}

	trimmedHint := displayMsg{
		role:    "system",
		content: fmt.Sprintf("... %d earlier messages trimmed (use /export to save full history)", trimCount),
	}
	// Rebuild: preserved prefix (welcome) + hint + recent messages.
	kept := make([]displayMsg, 0, len(m.messages)-trimCount+1)
	kept = append(kept, m.messages[:startIdx]...)
	kept = append(kept, trimmedHint)
	kept = append(kept, m.messages[startIdx+trimCount:]...)
	m.messages = kept
	// Expansion state is keyed by message index; reindex the survivors so
	// Enter-to-expand keeps targeting the right messages and stale keys for
	// trimmed messages are pruned (M18: the map grew without bound).
	m.toolResultExpanded = reindexExpandedMap(m.toolResultExpanded, startIdx, trimCount)
	m.invalidateViewportCache()
}

// reindexExpandedMap maps tool-result expansion state across
// trimOldMessages' reindex: indices below startIdx are untouched, trimmed
// indices are pruned, and survivors above the trim shift down by
// trimCount-1 because the trim hint takes one slot.
func reindexExpandedMap(expanded map[int]bool, startIdx, trimCount int) map[int]bool {
	reindexed := make(map[int]bool, len(expanded))
	for idx, expandedState := range expanded {
		switch {
		case idx < startIdx:
			reindexed[idx] = expandedState
		case idx >= startIdx+trimCount:
			reindexed[idx-trimCount+1] = expandedState
		}
	}
	return reindexed
}

func (m *chatModel) markPartialDirty() tea.Cmd {
	m.partialDirty = true
	if time.Since(m.lastPartialRender) >= streamRenderInterval {
		m.viewDirty = true
		m.lastPartialRender = time.Now()
		m.partialDirty = false
		m.partialRenderPending = false
		return nil
	}
	if m.partialRenderPending {
		return nil
	}
	m.partialRenderPending = true
	return tea.Tick(streamRenderInterval, func(time.Time) tea.Msg { return streamRenderTickMsg{} })
}

func (m *chatModel) flushPartialDirty() {
	if m.partialDirty {
		m.viewDirty = true
		m.partialDirty = false
	}
	m.partialRenderPending = false
}

func spinnerVerbTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return spinnerVerbTickMsg{} })
}

func promptKeepAliveCmd() tea.Cmd {
	return tea.Tick(15*time.Second, func(time.Time) tea.Msg { return promptKeepAliveMsg{} })
}

func permissionPromptTimeoutCmd(seq int) tea.Cmd {
	return tea.Tick(5*time.Minute, func(time.Time) tea.Msg { return permissionPromptTimeoutMsg{seq: seq} })
}

func askUserPromptTimeoutCmd(seq int) tea.Cmd {
	return tea.Tick(5*time.Minute, func(time.Time) tea.Msg { return askUserPromptTimeoutMsg{seq: seq} })
}
