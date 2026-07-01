package cmd

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	streamChunkMsg     string
	streamDoneMsg      struct{}
	streamRetryMsg     struct{ content string }
	streamErrMsg       struct{ err error }
	blinkTickMsg       struct{}
	spinnerVerbTickMsg struct{}
	usageUpdateMsg     struct{ usage *engine.StreamUsage }
	compactStartMsg    struct{}
	compactMsg         struct {
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
	systemPromptContextReadyMsg struct {
		context string
	}
	loopTickMsg      struct{ command string }
	toolUseMsg       struct{ name, id string }
	toolResultMsg    struct{ name, content string }
	permissionAskMsg struct{ req engine.PermissionRequest }
	thinkingMsg      string
	blastRadiusMsg   struct{ message string }
	askUserMsg       struct {
		question string
		response chan string
	}
)

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
	streamCancelled            bool                      // user cancelled; suppress late streamDone side effects
	turnSawThinking            bool                      // current turn received hidden reasoning
	turnHadAssistantOutput     bool                      // current turn produced assistant text
	turnHadToolActivity        bool                      // current turn produced tool activity
	messageQueue               []string                  // queued messages while agent is working
	permReq                    *engine.PermissionRequest // pending permission prompt
	askReq                     *askUserMsg               // pending ask_user prompt
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
	configKeysPendingRemove    string              // provider awaiting delete confirmation
	configKeysRemoveStep       int                 // 1 = first prompt, 2 = final confirm
	configReplaceProvider      string              // replace-key flow target gateway
	configPostSaveKeysProvider string              // return to Gateways tab after replace save
	configSaving               bool                // blocks hub/list input while async credential work runs
	configPendingOllamaURL     string
	configXiaomiRegionSel      int // Token Plan region picker index
	configZAIRegionSel         int // Z.AI (general or coding) region picker index
	pluginRuntime              *plugin.Runtime
	spinnerVerb                string
	// Per-turn token counters shown next to the spinner (↑ input, ↓ output).
	// Reset each time the user submits a message; updated by usageUpdateMsg.
	turnInputTokens  int
	turnOutputTokens int
	compacting       bool // stream auto-compact: spinner label = Compacting context
	manualCompacting bool // user ran /compact: show progress panel above input
	compactBarUsed   int  // context tokens snapshot at /compact start
	compactBarWindow int  // context window snapshot at /compact start
	compactCancel    context.CancelFunc
	// Display values lerped toward the turn targets each render frame
	// (factor 0.10). Smooths the counter animation.
	displayInTok         float64
	displayOutTok        float64
	lastCtrlC            time.Time
	history              []string
	historyIdx           int
	historyDraft         string // unsent text before navigating history
	autoScroll           bool   // whether viewport is pinned to bottom
	streamFollow         bool   // follow streaming output (Grok-style; toggle with /follow)
	uiFocus              uiFocusArea
	contentLines         int   // total lines in scrollback content (for footer position)
	lastMouseY           int   // last pointer row (0-based); -1 = unknown; used when Cursor reports stale wheel Y
	mouseOverride        *bool // runtime /mouse toggle; persisted via settings
	vim                  *VimState
	wal                  *session.WAL
	startedAt            time.Time // per-turn timer (spinner + turn elapsed)
	sessionStartedAt     time.Time // whole chat session (footer duration)
	sessionBootstrapDone bool
	toolStartTime        time.Time
	welcomeCache         string
	viewDirty            bool
	layoutKey            int    // input lines + slash menu height fingerprint
	cachedBottomBarLines int    // memoized chatBottomBarLines; refresh via refreshInputLayoutIfNeeded
	slashSugInput        string // memoize slashSuggestions per keystroke
	slashSugCache        []string
	connStatusKey        string // gateway+model+creds fingerprint
	connStatusVal        string
	partialDirty         bool // stream text changed since last viewport paint
	lastPartialRender    time.Time
	statusLeftKey        string
	statusLeftVal        string

	// Incremental viewport cache (see chat_viewport_render.go).
	vpStableContent string
	vpRenderedMsgs  int
	vpRenderWidth   int
	vpLastMsgLen    int
	activeSkills    map[string]plugin.SmartSkill // per-session activated skills

	// Container mode (hermetic execution in sandbox)
	containerEnabled bool
	containerStatus  string // "checking docker…", "pulling image…", "starting…", "<id>", "docker not running"
	containerReady   bool
	containerErr     error
	containerSandbox *sandbox.ContainerSandbox

	// Taste & staleness tracking
	tasteHooks        *taste.Hooks
	stalenessDetector *staleness.Detector

	// Lacy-inspired features
	termCtx        *sessioncapture.TerminalContext
	inputIndicator *InputIndicator
	ghostText      *GhostText
	modeManager    *shellmode.ModeManager
	brailleSpinner *BrailleSpinner

	// BMAD/Aeon-inspired features
	hintsLoader  *engine.HintsLoader
	sourceRoots  *engine.SourceRoots
	selfImprover *engine.SelfImprover
	codingSoul   *engine.CodingSoul

	// Loop cancellation
	loopCancel context.CancelFunc // cancels the current /loop goroutine

	// PageRank file watcher
	watcherStop func() // stops the incremental symbol graph file watcher

	// Command palette (Ctrl+K)
	commandPalette *CommandPalette
}

const streamRenderInterval = 50 * time.Millisecond

func (m *chatModel) markPartialDirty() {
	m.partialDirty = true
	if time.Since(m.lastPartialRender) >= streamRenderInterval {
		m.viewDirty = true
		m.lastPartialRender = time.Now()
		m.partialDirty = false
	}
}

func (m *chatModel) flushPartialDirty() {
	if m.partialDirty {
		m.viewDirty = true
		m.partialDirty = false
	}
}

func blinkTickCmd() tea.Cmd {
	return tea.Tick(2200*time.Millisecond, func(time.Time) tea.Msg { return blinkTickMsg{} })
}

func spinnerVerbTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return spinnerVerbTickMsg{} })
}
