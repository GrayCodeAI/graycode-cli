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

var (
	tealColor    = lipgloss.Color("#4ECDC4")
	dimColor     = lipgloss.Color("#666666")
	errorColor   = lipgloss.Color("#e05555")
	toolColor    = lipgloss.Color("#FFD700")
	dimStyle     = lipgloss.NewStyle().Foreground(dimColor)
	errorStyle   = lipgloss.NewStyle().Foreground(errorColor)
	toolStyle    = lipgloss.NewStyle().Foreground(toolColor).Bold(true)
	toolDimStyle = lipgloss.NewStyle().Foreground(dimColor)

	slashCmdStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#73767E"))
	slashDescStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#73767E"))
	slashSelCmdStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	slashSelDescStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E"))
	inputBorderStyle  = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false, true, false).BorderForeground(lipgloss.Color("#555555"))
	ghostHintStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Italic(true)
	containerErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
)

// Hawk spinner frames: dot-by-dot build then reverse (like Droid)
var hawkSpinnerFrames = []string{"◐", "◓", "◑", "◒"}

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
	streamChunkMsg string
	streamDoneMsg  struct{}
	streamErrMsg   struct{ err error }
	blinkTickMsg   struct{}
)

type (
	glimmerTickMsg   struct{} // unused; kept for tea.Msg compatibility
	modelsFetchedMsg struct {
		options  []configModelOption
		provider string
		err      error
	}
	loopTickMsg      struct{ command string }
	toolUseMsg       struct{ name, id string }
	toolResultMsg    struct{ name, content string }
	permissionAskMsg struct{ req engine.PermissionRequest }
	thinkingMsg      string
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
	input                  textarea.Model
	configInput            textinput.Model // secondary input for config panel password entry
	useConfigInput         bool            // true when config panel needs textinput (e.g. password)
	spinner                spinner.Model
	viewport               viewport.Model
	session                *engine.Session
	registry               *tool.Registry
	settings               hawkconfig.Settings
	ref                    *progRef
	cancel                 context.CancelFunc // cancel current stream
	sessionID              string
	messages               []displayMsg
	partial                *strings.Builder
	waiting                bool
	permReq                *engine.PermissionRequest // pending permission prompt
	askReq                 *askUserMsg               // pending ask_user prompt
	width                  int
	height                 int
	quitting               bool
	blinkClosed            bool
	slashSel               int
	configOpen             bool
	configTab              int    // configTabKeys, configTabGateways, configTabModels
	configMenu             string // configMenuNone, configMenuProviders
	configSel              int
	configScroll           int // scroll offset for long lists
	configNotice           string
	configEntry            string              // configEntryNone, configEntryAPIKeyPaste, configEntryOllamaURL
	configProvider         string              // e.g. configProviderOllama while entry overlay is open
	configModelOptions     []configModelOption // labels + ids from eyrie catalog
	configModelProvider    string              // filter models after API key paste
	configGuideAfterKey    bool                // open model picker when discover finishes
	configGatewayFocus     int                 // last highlighted gateway row (for refresh action)
	configPendingKey       string
	configProviderOptions  []hawkconfig.CredentialProviderOption
	configSaving           bool // blocks hub/list input while async credential work runs
	configPendingOllamaURL string
	pluginRuntime          *plugin.Runtime
	spinnerVerb            string
	glimmerPos             int
	lastCtrlC              time.Time
	history                []string
	historyIdx             int
	historyDraft           string // unsent text before navigating history
	autoScroll             bool   // whether to auto-scroll viewport to bottom
	vim                    *VimState
	contextViz             *ContextVisualization
	wal                    *session.WAL
	startedAt              time.Time
	toolStartTime          time.Time
	welcomeCache           string
	viewDirty              bool
	layoutKey              int    // input lines + slash menu height fingerprint
	slashSugInput          string // memoize slashSuggestions per keystroke
	slashSugCache          []string
	connStatusKey          string // gateway+model+creds fingerprint
	connStatusVal          string
	partialDirty           bool // stream text changed since last viewport paint
	lastPartialRender      time.Time
	activeSkills           map[string]plugin.SmartSkill // per-session activated skills

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

func glimmerTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return glimmerTickMsg{} })
}
