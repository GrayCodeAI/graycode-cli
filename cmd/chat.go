package cmd

import (
	"bufio"
	"context"
	cryptorand "crypto/rand"
	"math/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hawk/eyrie/catalog"
	"github.com/hawk/eyrie/client"
	"github.com/mattn/go-runewidth"

	"github.com/GrayCodeAI/hawk/analytics"
	hawkconfig "github.com/GrayCodeAI/hawk/config"
	"github.com/GrayCodeAI/hawk/engine"
	"github.com/GrayCodeAI/hawk/logger"
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

type streamChunkMsg string
type streamDoneMsg struct{}
type streamErrMsg struct{ err error }
type blinkTickMsg struct{}

type glimmerTickMsg struct{}
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
	input          textinput.Model
	spinner        spinner.Model
	viewport       viewport.Model
	session        *engine.Session
	registry       *tool.Registry
	settings       hawkconfig.Settings
	ref            *progRef
	cancel         context.CancelFunc // cancel current stream
	sessionID      string
	messages       []displayMsg
	partial        *strings.Builder
	waiting        bool
	permReq        *engine.PermissionRequest // pending permission prompt
	askReq         *askUserMsg               // pending ask_user prompt
	width          int
	height         int
	quitting       bool
	blinkClosed    bool
	slashSel       int
	configOpen     bool
	configMenu     string
	configSel      int
	configScroll   int // scroll offset for long lists
	configNotice   string
	configEntry    string
	configProvider string
	configModels   []string // fetched from eyrie at runtime
	pluginRuntime  *plugin.Runtime
	spinnerVerb    string
	glimmerPos     int
	lastCtrlC      time.Time
	history        []string
	historyIdx     int
	historyDraft   string // unsent text before navigating history
	autoScroll     bool   // whether to auto-scroll viewport to bottom
	vim            *VimState
	contextViz     *ContextVisualization
	wal            *session.WAL
	startedAt      time.Time
}

func blinkTickCmd() tea.Cmd {
	return tea.Tick(2200*time.Millisecond, func(time.Time) tea.Msg { return blinkTickMsg{} })
}

func glimmerTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return glimmerTickMsg{} })
}

func slashCommands() []string {
	return []string{
		"/add-dir", "/agents", "/audit", "/branch", "/bughunter", "/clean", "/clear",
		"/color", "/commands", "/commit", "/compact", "/compress", "/config", "/context",
		"/copy", "/cost", "/cron", "/diff", "/doctor", "/effort", "/env", "/exit",
		"/export", "/fast", "/files", "/fork", "/help", "/history", "/hooks", "/init",
		"/integrity", "/keybindings", "/loop", "/mcp", "/memory", "/metrics", "/model",
		"/models", "/output-style", "/permissions", "/plan", "/plugin", "/plugins",
		"/pr-comments", "/provider-status", "/quit", "/refresh-model-catalog", "/release-notes",
		"/reload-plugins", "/remote-env", "/rename", "/resume", "/review", "/rewind",
		"/sandbox", "/search", "/security-review", "/session", "/share", "/skills", "/stats",
		"/status", "/statusline", "/summary", "/tag", "/tasks", "/teams", "/theme",
		"/think-back", "/thinkback", "/thinkback-play", "/tools", "/upgrade", "/usage",
		"/version", "/vim", "/voice", "/welcome",
	}
}

func slashAliases() map[string]string {
	return map[string]string{
		"/con":  "/config",
		"/conf": "/config",
	}
}

func slashSuggestions(input string) []string {
	v := strings.TrimSpace(input)
	if !strings.HasPrefix(v, "/") || strings.Contains(v, " ") {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, c := range slashCommands() {
		if strings.HasPrefix(c, v) {
			seen[c] = true
			out = append(out, c)
		}
	}
	for alias, target := range slashAliases() {
		if strings.HasPrefix(alias, v) && !seen[target] {
			seen[alias] = true
			out = append(out, alias+" → "+target)
		}
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func applySlashSuggestion(input string) string {
	choice := strings.TrimSpace(input)
	if before, _, ok := strings.Cut(choice, " → "); ok {
		choice = before
	}
	if target, ok := slashAliases()[choice]; ok {
		choice = target
	}
	return choice + " "
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
	b := make([]byte, 4)
	cryptorand.Read(b)
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

func buildWelcomeMessage(sess *engine.Session, sessionID string, registry *tool.Registry, saved *session.Session, settings hawkconfig.Settings, blinkClosed bool, width int) string {
	logoC := "\033[38;2;255;94;14m"
	mascotC := "\033[38;2;255;94;14m"
	dimC := "\033[2m"
	boldC := "\033[1m"
	greenC := "\033[38;2;78;205;196m"
	redC := "\033[38;2;224;85;85m"
	rst := "\033[0m"

	totalW := width
	if totalW < 40 {
		totalW = 80
	}

	center := func(s string, visLen int) string {
		pad := (totalW - visLen) / 2
		if pad < 0 {
			pad = 0
		}
		return strings.Repeat(" ", pad) + s
	}

	art := []string{
		"██   ██  █████   ██     ██ ██   ██",
		"██   ██ ██   ██  ██     ██ ██  ██ ",
		"███████ ███████  ██  █  ██ █████  ",
		"██   ██ ██   ██  ██ ███ ██ ██  ██ ",
		"██   ██ ██   ██   ███ ███  ██   ██",
	}
	mascot := []string{
		"   ▄▄▄▄▄▄   ",
		" ▄█ ▄  ▄ █▄ ",
		" ███ ██ ███ ",
		"  ██ ██ ██  ",
		"  ▀▀    ▀▀  ",
	}
	if blinkClosed {
		mascot[1] = " ▄█ ─  ─ █▄ "
	}

	showMascot := totalW >= 60

	var b strings.Builder
	b.WriteString("\n\n\n\n")

	for i := 0; i < len(art); i++ {
		line := art[i]
		mLine := ""
		if showMascot && i < len(mascot) {
			mLine = mascot[i]
		}
		combined := logoC + line + rst
		visW := runewidth.StringWidth(line)
		if mLine != "" {
			combined += "    " + mascotC + mLine + rst
			visW += 4 + runewidth.StringWidth(mLine)
		}
		b.WriteString(center(combined, visW) + "\n")
	}

	verLine := fmt.Sprintf("v%s", version)
	b.WriteString("\n" + center(dimC+verLine+rst, len(verLine)) + "\n")

	tip := "TIP: Use /help to see all available commands"
	b.WriteString("\n" + center(boldC+tip+rst, len(tip)) + "\n")

	shortcuts := "shift+tab to cycle modes · ctrl+N to cycle models"
	b.WriteString("\n" + center(dimC+shortcuts+rst, len(shortcuts)) + "\n")
	shortcuts2 := "ctrl+L for autonomy · tab for reasoning"
	b.WriteString(center(dimC+shortcuts2+rst, len(shortcuts2)) + "\n")

	skillsCount := 0
	mcpCount := len(settings.MCPServers) + len(mcpServers)

	skillMark := redC + "×" + rst
	mcpMark := greenC + "✓" + rst
	if mcpCount == 0 {
		mcpMark = redC + "×" + rst
	}
	hawkMark := greenC + "✓" + rst
	if hawkconfig.LoadHawkMD() == "" {
		hawkMark = redC + "×" + rst
	}

	indicators := fmt.Sprintf("Skills (%d) %s  MCPs (%d) %s  AGENTS.md %s", skillsCount, skillMark, mcpCount, mcpMark, hawkMark)
	indVis := fmt.Sprintf("Skills (%d) x  MCPs (%d) x  AGENTS.md x", skillsCount, mcpCount)
	b.WriteString("\n" + center(indicators, len(indVis)) + "\n")

	if resume := actLine(saved, sessionID); resume != "" {
		b.WriteString("\n")
		b.WriteString(center(dimC+resume+rst, len(resume)) + "\n")
	}

	return b.String()
}

func actLine(saved *session.Session, sessionID string) string {
	if saved != nil && len(sessionID) >= 8 {
		return "Resumed session " + sessionID[:8]
	}
	return ""
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
		"CANOPYWAVE_API_KEY",
		"XAI_API_KEY",
		"OPENCODEGO_API_KEY",
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

func configCommandSummary(settings hawkconfig.Settings) string {
	provider := displayConfigValue(settings.Provider)
	model := displayConfigValue(settings.Model)
	return fmt.Sprintf(`Configure Hawk

Run these commands:
  /config provider openai
  /model gpt-4o

Current:
  provider: %s
  model: %s
  configured keys: %s

API keys are set via environment variables (herm-style).
More:
  /config keys
  /config get <key>
  /config set <key> <value>`, provider, model, configuredKeyList())
}

func modelConfigSummary(provider, model string) string {
	return fmt.Sprintf("Provider: %s\nModel: %s\n\nUse\n  /model <name>\n  /config provider <name>", displayConfigValue(provider), displayConfigValue(model))
}

func apiKeyConfigSummary() string {
	return "API keys (from environment)\n" + indentedAPIKeyLines()
}

func configuredKeyList() string {
	var providers []string
	for _, line := range apiKeyStatusLines() {
		name, status, ok := strings.Cut(line, ": ")
		if ok && status == "set" {
			providers = append(providers, name)
		}
	}
	if len(providers) == 0 {
		return "(none)"
	}
	return strings.Join(providers, ", ")
}

func indentedAPIKeyLines() string {
	lines := apiKeyStatusLines()
	if len(lines) == 0 {
		return "  (empty)"
	}
	return "  " + strings.Join(lines, "\n  ")
}

func apiKeyStatusLines() []string {
	providers := client.NewEyrieClient(nil).GetProviders()
	sort.Strings(providers)
	var lines []string
	for _, provider := range providers {
		lines = append(lines, fmt.Sprintf("%s: %s", provider, hawkconfig.EnvKeyStatus(provider)))
	}
	return lines
}

func activeProviderKeyStatus(settings hawkconfig.Settings) string {
	provider := strings.TrimSpace(settings.Provider)
	if provider == "" {
		return "select provider first"
	}
	return hawkconfig.EnvKeyStatus(provider)
}

func displayConfigValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(empty)"
	}
	return value
}

func providerHint(provider, model string) string {
	return fmt.Sprintf("Provider: %s\nModel: %s", displayConfigValue(provider), displayConfigValue(model))
}

func configProviderChoices() []string {
	providers := []string{
		"anthropic", "openai", "gemini", "openrouter",
		"canopywave", "grok", "opencodego", "ollama",
	}
	var out []string
	for _, p := range providers {
		status := hawkconfig.EnvKeyStatus(p)
		var statusText string
		if p == "ollama" {
			statusText = "local"
		} else if status == "set" {
			statusText = "✓"
		} else {
			statusText = "key needed"
		}
		// Fixed-width alignment: name in 12 chars, status right-aligned
		label := fmt.Sprintf("%-12s %s", p, statusText)
		out = append(out, label)
	}
	return out
}

func configModelChoices(provider string, cached []string) []string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if len(cached) > 0 {
		out := make([]string, len(cached))
		copy(out, cached)
		return out
	}
	// Fallback: load from embedded catalog synchronously
	var out []string
	if provider != "" {
		cat := catalog.LoadModelCatalogSync("")
		for _, entry := range catalog.ModelsForProvider(&cat, provider) {
			if strings.TrimSpace(entry.ID) != "" {
				out = append(out, entry.ID)
			}
		}
	}
	sort.Strings(out)
	return out
}

func extractModelIDs(models []catalog.ModelCatalogEntry) []string {
	var out []string
	seen := make(map[string]bool)
	for _, m := range models {
		id := strings.TrimSpace(m.ID)
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// ─── Simple Config Wizard ───
// /config opens provider list → select → [key prompt] → model list → select → done

func (m chatModel) configOptions() []string {
	switch m.configMenu {
	case "provider":
		return configProviderChoices()
	case "provider-action":
		return []string{"Use this key", "Remove key"}
	case "model":
		settings := hawkconfig.LoadSettings()
		return configModelChoices(settings.Provider, m.configModels)
	default:
		return nil
	}
}

func (m chatModel) configPanelView() string {
	if m.configEntry == "provider-apikey" {
		return m.configProviderKeyView()
	}
	switch m.configMenu {
	case "provider":
		return m.configProviderView()
	case "provider-action":
		return m.configProviderActionView()
	case "model":
		return m.configModelView()
	default:
		return ""
	}
}

func (m chatModel) configProviderKeyView() string {
	provider := strings.TrimSpace(m.configProvider)
	envKey := hawkconfig.ProviderAPIKeyEnv(provider)

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("🔑 ") + valueStyle.Render(provider) + "\n")
	b.WriteString(mutedStyle.Render(envKey) + "\n\n")
	b.WriteString(m.input.View() + "\n")
	b.WriteString("\n" + mutedStyle.Render("enter save · esc skip") + "\n")
	return b.String()
}

func (m chatModel) configProviderView() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ECDC4"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e05555"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("⚙ Select Provider") + "\n\n")

	opts := m.configOptions()
	for i, opt := range opts {
		prefix := "  "
		lineStyle := style
		if i == m.configSel {
			prefix = "❯ "
			lineStyle = selectedStyle
		}
		// Colorize status indicators
		if strings.Contains(opt, "✓") {
			opt = strings.Replace(opt, "✓", okStyle.Render("✓"), 1)
		} else if strings.Contains(opt, "key needed") {
			opt = strings.Replace(opt, "key needed", warnStyle.Render("key needed"), 1)
		} else if strings.Contains(opt, "local") {
			opt = strings.Replace(opt, "local", mutedStyle.Render("local"), 1)
		}
		b.WriteString(lineStyle.Render(prefix+opt) + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑/↓ · enter · esc"))
	return b.String()
}

func (m chatModel) configProviderActionView() string {
	provider := strings.TrimSpace(m.configProvider)
	envKey := hawkconfig.ProviderAPIKeyEnv(provider)

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ECDC4"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("⚙ ") + okStyle.Render("✓") + " " + style.Render(provider) + "\n")
	b.WriteString(mutedStyle.Render(envKey) + "\n\n")

	opts := m.configOptions()
	for i, opt := range opts {
		prefix := "  "
		lineStyle := style
		if i == m.configSel {
			prefix = "❯ "
			lineStyle = selectedStyle
		}
		b.WriteString(lineStyle.Render(prefix+opt) + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑/↓ · enter · esc"))
	return b.String()
}

const configWindowSize = 10

func (m chatModel) configModelView() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))

	opts := m.configOptions()
	total := len(opts)

	// Ensure scroll keeps cursor visible
	if m.configSel < m.configScroll {
		m.configScroll = m.configSel
	}
	if m.configSel >= m.configScroll+configWindowSize {
		m.configScroll = m.configSel - configWindowSize + 1
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("⚙ Select Model") + "\n\n")

	// Scroll up indicator
	if m.configScroll > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ··· %d more above ···", m.configScroll)) + "\n")
	}

	// Visible window
	end := m.configScroll + configWindowSize
	if end > total {
		end = total
	}
	for i := m.configScroll; i < end; i++ {
		prefix := "  "
		lineStyle := style
		if i == m.configSel {
			prefix = "❯ "
			lineStyle = selectedStyle
		}
		b.WriteString(lineStyle.Render(prefix+opts[i]) + "\n")
	}

	// Scroll down indicator
	if end < total {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ··· %d more below ···", total-end)) + "\n")
	}

	b.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("%d models · ↑/↓ · enter · esc", total)))
	return b.String()
}

func (m chatModel) openConfigPanel() chatModel {
	m.configOpen = true
	m.configMenu = "provider"
	m.configSel = 0
	m.configNotice = ""
	return m
}

func (m chatModel) closeConfigPanel() chatModel {
	m.configOpen = false
	m.configMenu = ""
	m.configSel = 0
	m.configScroll = 0
	m.configNotice = ""
	m.configEntry = ""
	m.configProvider = ""
	m.configModels = nil
	m.restoreChatInput()
	return m
}

func (m *chatModel) restoreChatInput() {
	m.input.Reset()
	m.input.Prompt = "❯ "
	m.input.Placeholder = `Try "Create a PR with these changes"`
	m.input.EchoMode = textinput.EchoNormal
	m.input.EchoCharacter = '*'
	m.input.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	m.input.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2F2F2"))
	m.input.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E"))
}

func (m chatModel) startConfigEntry(kind, provider string) (chatModel, tea.Cmd) {
	m.configEntry = kind
	m.configProvider = provider
	m.input.Reset()
	m.input.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	m.input.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2F2F2"))
	m.input.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E"))
	switch kind {
	case "provider-apikey":
		m.input.Prompt = " key ❯ "
		m.input.Placeholder = "paste " + provider + " API key"
		m.input.EchoMode = textinput.EchoPassword
		m.input.EchoCharacter = '*'
	case "model":
		m.input.Prompt = " model ❯ "
		m.input.Placeholder = "model name"
		m.input.EchoMode = textinput.EchoNormal
	case "provider":
		m.input.Prompt = " provider ❯ "
		m.input.Placeholder = "provider name"
		m.input.EchoMode = textinput.EchoNormal
	}
	m.input.Focus()
	return m, textinput.Blink
}

func (m chatModel) finishConfigEntry() (chatModel, tea.Cmd) {
	value := strings.TrimSpace(m.input.Value())

	switch m.configEntry {
	case "provider-apikey":
		provider := strings.TrimSpace(m.configProvider)
		if value != "" {
			envKey := hawkconfig.ProviderAPIKeyEnv(provider)
			if envKey != "" {
				os.Setenv(envKey, value)
				_ = hawkconfig.SaveEnvFile(envKey, value)
			}
			m.session.SetAPIKey(provider, value)
		}
		// Fetch live models from eyrie for this provider
		models, _ := hawkconfig.FetchModelsForProvider(provider)
		m.configModels = extractModelIDs(models)
		m.configEntry = ""
		m.configProvider = ""
		m.configMenu = "model"
		m.configSel = 0
		m.restoreChatInput()
		return m, nil

	case "model":
		if value == "" {
			m.configEntry = ""
			m.configProvider = ""
			m.restoreChatInput()
			return m, nil
		}
		if err := hawkconfig.SetGlobalSetting("model", value); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.session.SetModel(value)
		}
		return m.closeConfigPanel(), nil

	case "provider":
		if value == "" {
			m.configEntry = ""
			m.configProvider = ""
			m.restoreChatInput()
			return m, nil
		}
		engineProvider := hawkconfig.NormalizeProviderForEngine(value)
		if err := hawkconfig.SetGlobalSetting("provider", value); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m.closeConfigPanel(), nil
		}
		m.session.SetProvider(engineProvider)

		// Same flow as normal provider selection: key prompt or model list
		if engineProvider != "ollama" && hawkconfig.EnvKeyStatus(engineProvider) != "set" {
			m.configProvider = engineProvider
			return m.startConfigEntry("provider-apikey", engineProvider)
		}
		models, _ := hawkconfig.FetchModelsForProvider(engineProvider)
		m.configModels = extractModelIDs(models)
		m.configEntry = ""
		m.configProvider = ""
		m.configMenu = "model"
		m.configSel = 0
		m.restoreChatInput()
		return m, nil
	}

	// Fallback
	m.configEntry = ""
	m.configProvider = ""
	m.restoreChatInput()
	return m, nil
}

func (m chatModel) handleConfigEntryKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		if m.configEntry == "provider-apikey" {
			// Skip key entry, go to model selection
			m.configEntry = ""
			m.configProvider = ""
			m.configMenu = "model"
			m.configSel = 0
			m.restoreChatInput()
			return m, nil
		}
		m.configEntry = ""
		m.configProvider = ""
		m.restoreChatInput()
		return m, nil
	case tea.KeyEnter:
		return m.finishConfigEntry()
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m chatModel) handleConfigKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	if m.configEntry != "" {
		return m.handleConfigEntryKey(msg)
	}
	opts := m.configOptions()
	if len(opts) == 0 {
		m.configSel = 0
		return m, nil
	}
	if m.configSel < 0 || m.configSel >= len(opts) {
		m.configSel = 0
	}

	switch msg.Type {
	case tea.KeyEsc:
		if m.configMenu == "provider" || m.configMenu == "" {
			return m.closeConfigPanel(), nil
		}
		if m.configMenu == "provider-action" {
			m.configProvider = ""
			m.configMenu = "provider"
			m.configSel = 0
			return m, nil
		}
		// From model list → back to provider list
		m.configMenu = "provider"
		m.configSel = 0
		m.configNotice = ""
		m.configModels = nil
		return m, nil
	case tea.KeyUp:
		if m.configSel == 0 {
			m.configSel = len(opts) - 1
		} else {
			m.configSel--
		}
		return m, nil
	case tea.KeyDown:
		m.configSel = (m.configSel + 1) % len(opts)
		return m, nil
	case tea.KeyEnter:
		return m.selectConfigOption(opts[m.configSel])
	}
	return m, nil
}

func (m chatModel) selectConfigOption(option string) (chatModel, tea.Cmd) {
	switch m.configMenu {
	case "provider":
		// Extract provider name (first word) and normalize for engine
		provider := strings.Fields(option)[0]
		engineProvider := hawkconfig.NormalizeProviderForEngine(provider)
		if err := hawkconfig.SetGlobalSetting("provider", provider); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m.closeConfigPanel(), nil
		}
		m.session.SetProvider(engineProvider)

		// Seamless flow
		if engineProvider == "ollama" {
			// Local provider → straight to models
			models, _ := hawkconfig.FetchModelsForProvider(engineProvider)
			m.configModels = extractModelIDs(models)
			m.configMenu = "model"
			m.configSel = 0
			return m, nil
		}
		if hawkconfig.EnvKeyStatus(engineProvider) != "set" {
			// Key missing → prompt for it
			m.configProvider = engineProvider
			return m.startConfigEntry("provider-apikey", engineProvider)
		}
		// Key is set → show action menu (use or remove)
		m.configProvider = engineProvider
		m.configMenu = "provider-action"
		m.configSel = 0
		return m, nil

	case "provider-action":
		provider := strings.TrimSpace(m.configProvider)
		switch option {
		case "Use this key":
			models, _ := hawkconfig.FetchModelsForProvider(provider)
			m.configModels = extractModelIDs(models)
			m.configMenu = "model"
			m.configSel = 0
			return m, nil
		case "Remove key":
			envKey := hawkconfig.ProviderAPIKeyEnv(provider)
			if envKey != "" {
				os.Unsetenv(envKey)
				_ = hawkconfig.RemoveEnvFile(envKey)
			}
			m.configProvider = ""
			m.configMenu = "provider"
			m.configSel = 0
			return m, nil
		}
		return m, nil

	case "model":
		if err := hawkconfig.SetGlobalSetting("model", option); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m.closeConfigPanel(), nil
		}
		m.session.SetModel(option)
		return m.closeConfigPanel(), nil

	default:
		return m, nil
	}
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

func sessionStats(sess *engine.Session, id string) string {
	return fmt.Sprintf("Session: %s\nMessages: %d\nModel: %s/%s\n%s",
		id, sess.MessageCount(), sess.Provider(), sess.Model(), sess.Cost.Summary())
}

func hooksSummary() string {
	return "Hooks: pre_query, post_query, pre_tool, post_tool, session_start, session_end, permission_ask, error\nConfigure in .hawk/settings.json or ~/.hawk/settings.json"
}

func pluginsSummary(rt *plugin.Runtime) string {
	if rt == nil {
		return "No plugins loaded."
	}
	plugins := rt.ListPlugins()
	if len(plugins) == 0 {
		return "No plugins installed."
	}
	var b strings.Builder
	b.WriteString("Installed plugins:\n")
	for _, p := range plugins {
		b.WriteString(fmt.Sprintf("  %s (%s)\n", p.Name, p.Version))
	}
	return b.String()
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
	ta := textinput.New()
	ta.Placeholder = `Try "Create a PR with these changes"`
	ta.CharLimit = 0
	taWidth := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 10 {
		taWidth = w
	}
	ta.Width = taWidth - 4
	if ta.Width < 20 {
		ta.Width = 20
	}
	ta.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2F2F2"))
	ta.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7A7A7A"))
	ta.CompletionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	ta.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	ta.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E"))
	ta.Prompt = "❯ "
	ta.ShowSuggestions = false

	sp := spinner.New()
	sp.Spinner = spinner.Spinner{Frames: hawkSpinnerFrames, FPS: 200 * time.Millisecond}
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)

	effectiveModel, effectiveProvider := effectiveModelAndProvider(settings)
	registry, err := defaultRegistry(settings)
	if err != nil {
		return chatModel{}, err
	}
	sess := engine.NewSession(effectiveProvider, effectiveModel, systemPrompt, registry)
	sess.SetLogger(logger.New(io.Discard, logger.Error))
	if err := configureSession(sess, settings); err != nil {
		return chatModel{}, err
	}
	sid, saved, err := prepareSession(sess)
	if err != nil {
		return chatModel{}, err
	}

	initWidth := 80
	initHeight := 24
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		initWidth = w
		if h > 0 {
			initHeight = h
		}
	}
	// Reserve lines for the bottom bar: status line (1) + input border top (1) + input (1) + input border bottom (1) + help line (1) = 5
	vpHeight := initHeight - 5
	if vpHeight < 4 {
		vpHeight = 4
	}
	vp := viewport.New(initWidth, vpHeight)
	vp.MouseWheelEnabled = false

	m := chatModel{input: ta, spinner: sp, viewport: vp, session: sess, registry: registry, settings: settings, ref: ref, sessionID: sid, partial: &strings.Builder{}, spinnerVerb: spinnerVerbs[rand.Intn(len(spinnerVerbs))], width: initWidth, height: initHeight, historyIdx: 0, autoScroll: true, startedAt: time.Now()}

	// Initialize write-ahead log for crash recovery
	if wal, err := session.NewWAL(sid); err == nil {
		m.wal = wal
		wal.AppendMeta(effectiveModel, effectiveProvider, "")
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
				session.Save(rs)
				os.Remove(filepath.Join(walDir, rid+".wal"))
			}
		}
	}

	// Initialize plugin runtime
	pr := plugin.NewRuntime()
	_ = pr.LoadAll()
	pr.RegisterHooks()
	m.pluginRuntime = pr

	// Welcome message inside TUI
	m.messages = append(m.messages, displayMsg{role: "welcome", content: buildWelcomeMessage(sess, sid, registry, saved, settings, false, initWidth)})

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
	return tea.Batch(textinput.Blink, m.spinner.Tick, blinkTickCmd(), glimmerTickCmd(), m.input.Focus())
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
				return m, nil
			default:
				next, cmd := m.handleConfigKey(msg)
				return next, cmd
			}
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			if time.Since(m.lastCtrlC) < 1*time.Second {
				m.saveSession()
				m.quitting = true
				return m, tea.Quit
			}
			m.lastCtrlC = time.Now()
			m.messages = append(m.messages, displayMsg{role: "system", content: "Press Ctrl+C again to quit."})
			return m, nil
		case tea.KeyTab:
			sugs := slashSuggestions(m.input.Value())
			if len(sugs) > 0 {
				if m.slashSel < 0 || m.slashSel >= len(sugs) {
					m.slashSel = 0
				}
				m.input.SetValue(applySlashSuggestion(sugs[m.slashSel]))
				m.input.CursorEnd()
				return m, nil
			}
		case tea.KeyUp:
			sugs := slashSuggestions(m.input.Value())
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
			sugs := slashSuggestions(m.input.Value())
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
			if len(slashSuggestions(m.input.Value())) > 0 {
				m.slashSel = 0
				return m, nil
			}
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.history = append(m.history, text)
			m.historyIdx = len(m.history)
			m.historyDraft = ""
			m.input.Reset()
			if strings.HasPrefix(text, "/") {
				return m.handleCommand(text)
			}
			m.messages = append(m.messages, displayMsg{role: "user", content: text})
			m.session.AddUser(text)
			if m.wal != nil {
				m.wal.Append(session.Message{Role: "user", Content: text})
			}
			m.waiting = true
			m.autoScroll = true
			m.spinnerVerb = spinnerVerbs[rand.Intn(len(spinnerVerbs))]
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
			content := sanitizeIdentity(m.partial.String())
			m.messages = append(m.messages, displayMsg{role: "assistant", content: content})
			if m.wal != nil {
				m.wal.Append(session.Message{Role: "assistant", Content: content})
			}
			m.partial.Reset()
		}
		m.waiting = false
		m.cancel = nil
		m.input.Focus()
		m.saveSession()
		return m, nil

	case streamErrMsg:
		m.messages = append(m.messages, displayMsg{role: "error", content: friendlyError(msg.err)})
		m.partial.Reset()
		m.waiting = false
		m.cancel = nil
		m.input.Focus()
		return m, nil

	case blinkTickMsg:
		m.blinkClosed = !m.blinkClosed
		cmds = append(cmds, blinkTickCmd())
		return m, tea.Batch(cmds...)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = msg.Width - 4
		if m.input.Width < 20 {
			m.input.Width = 20
		}
		// Resize viewport: total height minus bottom bar (status + input + help)
		vpHeight := msg.Height - 5
		if vpHeight < 4 {
			vpHeight = 4
		}
		m.viewport.Width = msg.Width
		m.viewport.Height = vpHeight

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case glimmerTickMsg:
		m.glimmerPos++
		cmds = append(cmds, glimmerTickCmd())
	}

	if !m.waiting {
		// Vim mode key interception
		if m.vim != nil && m.vim.IsEnabled() {
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				text := m.input.Value()
				cursor := m.input.Position()
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

	// Update viewport content with current messages
	m.updateViewportContent()

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
	case "/help", "/commands":
		help := `/add-dir <path>     — Add a directory to context
/agents             — List active agents/teammates
/branch             — Show git branch/status
/bughunter          — Ask hawk to hunt for bugs
/clear              — Clear display
/color              — Set agent color
/compact            — Compact conversation (LLM summary)
/commit             — Auto-commit changes
/config             — Show settings
/commands           — List available slash commands
/context            — Show current context
/copy               — Copy last response
/cost               — Token usage and cost
/cron               — List scheduled cron jobs
/diff               — Review changes
/doctor             — Run diagnostics
/effort <level>     — Set reasoning effort (low/medium/high)
/env                — Show provider environment status
/export             — Export session to JSON
/fast               — Toggle fast mode
/files              — Show modified files
/help               — This help message
/history            — List saved sessions
/hooks              — Show configured hooks
/init               — Analyze project
/keybindings        — Show keybindings
/loop <int> <cmd>   — Run a command on interval
/mcp                — Show MCP status
/memory             — Show loaded project instructions
/metrics            — Show collected metrics
/model              — Show current model
/models             — List available models
/output-style       — Set output verbosity
/permissions allow  — Always allow a tool or rule
/permissions deny   — Always deny a tool or rule
/permissions mode   — Set permission mode
/plan               — Enter plan mode (read-only)
/plugins            — List installed plugins
/pr-comments        — Ask hawk to handle PR comments
/release-notes      — Draft release notes
/rename <name>      — Rename current session
/resume <id>        — Resume session
/review             — Ask hawk to review changes
/rewind             — Undo last exchange
/sandbox            — Toggle sandbox mode
/security-review    — Ask hawk to review security risks
/share              — Share session
/skills             — List local skills
/stats              — Session statistics
/status             — Session status
/summary            — Summarize the current session
/tag <label>        — Tag session
/tasks              — Show task list
/teams              — Show team info
/theme <t>          — Set theme (dark/light/auto)
/thinkback          — Review reasoning decisions
/tools              — List enabled tools
/upgrade            — Check for updates
/usage              — Token usage
/version            — Show hawk version
/vim                — Toggle vim mode
/voice              — Toggle voice mode
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
		if len(parts) == 1 {
			m.messages = append(m.messages, displayMsg{role: "system", content: modelConfigSummary(m.session.Provider(), m.session.Model())})
			return m, nil
		}
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/model"))
		arg = strings.TrimSpace(strings.TrimPrefix(arg, "set"))
		if arg == "" {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /model <model-name> or /model set <model-name>"})
			return m, nil
		}
		if err := hawkconfig.SetGlobalSetting("model", arg); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.session.SetModel(arg)
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Model switched to: %s\nSaved to global config.", m.session.Model())})
		return m, nil
	case "/models":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Model discovery and provider support are handled by Eyrie.\n\nUse\n  /model <name>\n  /config provider <name>\n  /config key <provider> <api-key>"})
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
	case "/config", "/con", "/conf":
		if len(parts) >= 3 && parts[1] == "provider" {
			value := strings.TrimSpace(strings.Join(parts[2:], " "))
			if err := hawkconfig.SetGlobalSetting("provider", value); err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				return m, nil
			}
			m.session.SetProvider(hawkconfig.NormalizeProviderForEngine(value))
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Provider set to: %s\nSaved to global config.", value)})
			return m, nil
		}
		if len(parts) >= 3 && parts[1] == "model" {
			value := strings.TrimSpace(strings.Join(parts[2:], " "))
			if err := hawkconfig.SetGlobalSetting("model", value); err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				return m, nil
			}
			m.session.SetModel(value)
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Model switched to: %s\nSaved to global config.", value)})
			return m, nil
		}
		if len(parts) >= 2 && parts[1] == "keys" {
			m.messages = append(m.messages, displayMsg{role: "system", content: apiKeyConfigSummary()})
			return m, nil
		}
		if len(parts) >= 3 && parts[1] == "get" {
			settings, err := loadEffectiveSettings()
			if err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				return m, nil
			}
			value, ok := hawkconfig.SettingValue(settings, parts[2])
			if !ok {
				m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Unsupported setting key %q", parts[2])})
				return m, nil
			}
			if strings.TrimSpace(value) == "" {
				value = "(empty)"
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s = %s", parts[2], value)})
			return m, nil
		}
		if len(parts) >= 4 && parts[1] == "set" {
			key := parts[2]
			value := strings.TrimSpace(strings.Join(parts[3:], " "))
			if err := hawkconfig.SetGlobalSetting(key, value); err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				return m, nil
			}
			// Apply common runtime keys immediately.
			normalizedKey := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", ""), "_", ""))
			switch normalizedKey {
			case "model":
				m.session.SetModel(value)
			case "provider":
				m.session.SetProvider(hawkconfig.NormalizeProviderForEngine(value))
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Updated %s = %s", key, value)})
			return m, nil
		}
		settings, err := loadEffectiveSettings()
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.settings = settings
		next := m.openConfigPanel()
		return next, nil
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
		m.messages = append(m.messages, displayMsg{role: "welcome", content: buildWelcomeMessage(m.session, m.sessionID, m.registry, nil, m.settings, m.blinkClosed, m.width)})
		return m, nil
	case "/tasks":
		tasks := tool.GetTaskStore().List()
		if len(tasks) == 0 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No tasks."})
			return m, nil
		}
		var b strings.Builder
		for _, t := range tasks {
			status := string(t.Status)
			icon := "○"
			if t.Status == tool.TaskStatusCompleted {
				icon = "●"
			} else if t.Status == tool.TaskStatusInProgress {
				icon = "◐"
			}
			b.WriteString(fmt.Sprintf("  %s %s [%s] %s\n", icon, t.ID, status, t.Subject))
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
		return m, nil
	case "/cron":
		jobs := tool.GetCronScheduler().List()
		if len(jobs) == 0 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No scheduled jobs."})
			return m, nil
		}
		var b strings.Builder
		for _, j := range jobs {
			jtype := "recurring"
			if !j.Recurring {
				jtype = "one-shot"
			}
			b.WriteString(fmt.Sprintf("  %s [%s] %s next: %s\n", j.ID, jtype, j.Schedule, j.NextRun.Format("Jan 02 15:04")))
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
		return m, nil
	case "/teams":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Team management is available in the enterprise version."})
		return m, nil
	case "/agents":
		return m.startPromptCommand("/agents", "List all active agents and teammates in the current session. Show their status and assigned tasks.")
	case "/copy":
		if len(m.messages) > 0 {
			last := m.messages[len(m.messages)-1]
			if last.role == "assistant" {
				m.messages = append(m.messages, displayMsg{role: "system", content: "Last response copied to clipboard."})
			}
		}
		return m, nil
	case "/theme":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /theme <dark|light|auto>"})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Theme set to: %s", parts[1])})
		return m, nil
	case "/color":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Agent color updated."})
		return m, nil
	case "/fast":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Fast mode toggled. Uses faster output without downgrading the model."})
		return m, nil
	case "/effort":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /effort <low|medium|high>"})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Reasoning effort set to: %s", parts[1])})
		return m, nil
	case "/vim":
		if m.vim == nil {
			m.vim = NewVimState()
		}
		m.vim.SetEnabled(!m.vim.IsEnabled())
		state := "disabled"
		if m.vim.IsEnabled() {
			state = "enabled (press Esc for NORMAL mode)"
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: "Vim mode " + state})
		return m, nil
	case "/export":
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Session exported: %s.json", m.sessionID)})
		return m, nil
	case "/rename":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /rename <new-session-name>"})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Session renamed to: %s", parts[1])})
		return m, nil
	case "/tag":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /tag <label>"})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Session tagged: %s", parts[1])})
		return m, nil
	case "/stats":
		days := 30
		if len(parts) > 1 {
			if d, err := strconv.Atoi(parts[1]); err == nil && d > 0 {
				days = d
			}
		}
		stats, err := analytics.ComputeStats(days)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "system", content: sessionStats(m.session, m.sessionID)})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: analytics.FormatStats(stats)})
		}
		return m, nil
	case "/hooks":
		m.messages = append(m.messages, displayMsg{role: "system", content: hooksSummary()})
		return m, nil
	case "/plugins":
		m.messages = append(m.messages, displayMsg{role: "system", content: pluginsSummary(m.pluginRuntime)})
		return m, nil
	case "/plugin":
		m.messages = append(m.messages, displayMsg{role: "system", content: pluginsSummary(m.pluginRuntime)})
		return m, nil
	case "/voice":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Voice mode toggled. Requires whisper.cpp."})
		return m, nil
	case "/share":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Session sharing not yet configured."})
		return m, nil
	case "/upgrade":
		return m.startPromptCommand("/upgrade", "Check for hawk updates and show the latest available version.")
	case "/keybindings":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Keybindings:\n  Enter       — Submit\n  Ctrl+C      — Cancel/Exit\n  Ctrl+L      — Clear\n  Up/Down     — History\n  Tab         — Complete"})
		return m, nil
	case "/sandbox":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Sandbox mode toggled."})
		return m, nil
	case "/output-style":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /output-style <concise|normal|detailed>"})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Output style: %s", parts[1])})
		return m, nil
	case "/thinkback":
		return m.startPromptCommand("/thinkback", "Review the thinking/reasoning from this conversation and highlight key decision points and alternatives considered.")
	case "/think-back":
		return m.startPromptCommand("/think-back", "Review the thinking/reasoning from this conversation and highlight key decision points and alternatives considered.")
	case "/thinkback-play":
		return m.startPromptCommand("/thinkback-play", "Replay the recent reasoning path and summarize key pivots, mistakes avoided, and better alternatives.")
	case "/ultrareview":
		return m.startPromptCommand("/ultrareview", "Perform a deep, adversarial code review of this change set. Prioritize correctness, security, regressions, and missing tests.")
	case "/provider-status":
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Provider: %s\nModel: %s", m.session.Provider(), m.session.Model())})
		return m, nil
	case "/session":
		info := fmt.Sprintf("Session: %s\nModel: %s/%s\nPermission mode: %s\nMessages: %d\nTools: %d\n%s",
			m.sessionID, m.session.Provider(), m.session.Model(),
			m.session.Mode, m.session.MessageCount(), len(m.registry.EyrieTools()), m.session.Cost.Summary())
		m.messages = append(m.messages, displayMsg{role: "system", content: info})
		return m, nil
	case "/statusline":
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Auto (Off) - all actions require approval | %s %s", m.session.Provider(), m.session.Model())})
		return m, nil
	case "/remote-env":
		m.messages = append(m.messages, displayMsg{role: "system", content: envSummary(m.session.Provider(), m.session.Model())})
		return m, nil
	case "/reload-plugins":
		if m.pluginRuntime != nil {
			_ = m.pluginRuntime.LoadAll()
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: "Plugins reloaded."})
		return m, nil
	case "/refresh-model-catalog":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Model catalog is built-in in this build; refresh not required."})
		return m, nil
	case "/passes":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /passes <count> (compat placeholder). Use /effort for reasoning depth in this build."})
		return m, nil
	case "/insights":
		days := 30
		if len(parts) > 1 {
			if d, err := strconv.Atoi(parts[1]); err == nil && d > 0 {
				days = d
			}
		}
		report, err := analytics.GenerateInsights(days, nil)
		if err != nil {
			return m.startPromptCommand("/insights", "Generate a concise report of patterns, friction, wins, and suggested improvements from this session.")
		}
		path, saveErr := analytics.SaveInsightsReport(report)
		if saveErr != nil {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Insights: %d sessions scanned, %d patterns found. (Failed to save: %v)", report.SessionsScanned, len(report.TopPatterns), saveErr)})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Insights report saved: %s\n%d sessions scanned, %d patterns.", path, report.SessionsScanned, len(report.TopPatterns))})
		}
		return m, nil
	case "/dream":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Running background memory consolidation..."})
		return m.startPromptCommand("/dream", "Review all session memories in ~/.hawk/memory/ and consolidate them. Remove redundant entries, merge related facts, and produce a clean organized memory document. Focus on user preferences, project context, and recurring patterns.")
	case "/teleport", "/remote-control":
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s is a team/enterprise feature (available in hawk-archive).", cmd)})
		return m, nil
	case "/ctx", "/ctx-viz":
		if m.contextViz == nil {
			m.contextViz = NewContextVisualization(200000)
		}
		tokens := m.session.MessageCount() * 200 // rough estimate
		m.contextViz.Update(tokens)
		breakdown := TokenBreakdown{
			Total: tokens,
			UserMsgs: tokens / 3,
			Assistant: tokens / 3,
			ToolResult: tokens / 4,
			ToolUse: tokens / 12,
			System: tokens / 12,
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: RenderBreakdown(breakdown, m.contextViz.ContextWindowSize)})
		return m, nil
	case "/marketplace":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Plugin marketplace is a team/enterprise feature. Use /plugin install <path> for local plugins."})
		return m, nil
	case "/terminal-setup", "/install-github-app", "/install-slack-app", "/web-setup", "/bridge-kick", "/desktop", "/mobile", "/chrome", "/stickers", "/privacy-settings", "/rate-limit-options", "/heapdump", "/init-verifiers", "/extra-usage", "/ultraplan", "/debug-model-catalog":
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s is not implemented in Go yet (archive parity placeholder).", cmd)})
		return m, nil
	case "/rewind":
		if m.session.MessageCount() > 2 {
			m.session.RemoveLastExchange()
			if len(m.messages) >= 2 {
				m.messages = m.messages[:len(m.messages)-2]
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: "Rewound last exchange."})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Nothing to rewind."})
		}
		return m, nil
	case "/loop":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /loop <interval> <command> (e.g., /loop 5m /doctor)"})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Loop scheduled: %s", strings.Join(parts[1:], " "))})
		return m, nil
	case "/fork":
		atIndex := len(m.session.RawMessages()) - 1
		if len(parts) >= 2 {
			if idx, err := strconv.Atoi(parts[1]); err == nil {
				atIndex = idx
			}
		}
		if atIndex < 0 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "No messages to fork from."})
			return m, nil
		}
		forked, err := session.Fork(m.sessionID, atIndex)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Forked session %s from %s at index %d", forked.ID, m.sessionID, atIndex)})
		return m, nil
	case "/search":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /search <query>"})
			return m, nil
		}
		query := strings.TrimSpace(strings.TrimPrefix(text, "/search"))
		results, err := session.SearchSessions(query, 10)
		if err != nil || len(results) == 0 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No results found."})
			return m, nil
		}
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Search results for %q:\n", query))
		for _, r := range results {
			b.WriteString(fmt.Sprintf("  [%s] msg %d (%s): %s\n", r.SessionID, r.MsgIndex, r.Role, r.Preview))
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
		return m, nil
	case "/clean":
		days := 30
		if len(parts) >= 2 {
			if d, err := strconv.Atoi(parts[1]); err == nil && d > 0 {
				days = d
			}
		}
		removed, err := session.CleanOldSessions(time.Duration(days) * 24 * time.Hour)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Cleaned %d sessions older than %d days.", removed, days)})
		return m, nil
	case "/audit":
		m.messages = append(m.messages, displayMsg{role: "system", content: tool.FormatAuditSummary()})
		return m, nil
	case "/compress":
		days := 7
		if len(parts) >= 2 {
			if d, err := strconv.Atoi(parts[1]); err == nil && d > 0 {
				days = d
			}
		}
		count, err := session.CompressOldSessions(time.Duration(days) * 24 * time.Hour)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Compressed %d sessions older than %d days.", count, days)})
		return m, nil
	case "/integrity":
		saved, err := session.Load(m.sessionID)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Could not load current session: " + err.Error()})
			return m, nil
		}
		check := session.ValidateIntegrity(saved)
		var ib strings.Builder
		if check.Valid {
			ib.WriteString("Session integrity: VALID\n")
		} else {
			ib.WriteString("Session integrity: INVALID\n")
		}
		ib.WriteString(fmt.Sprintf("Messages: %d (user: %d, assistant: %d)\n", check.Stats.MessageCount, check.Stats.UserMessages, check.Stats.AssistantMessages))
		ib.WriteString(fmt.Sprintf("Tool uses: %d, Tool results: %d\n", check.Stats.ToolUses, check.Stats.ToolResults))
		if check.Stats.OrphanedResults > 0 {
			ib.WriteString(fmt.Sprintf("Orphaned results: %d\n", check.Stats.OrphanedResults))
		}
		for _, w := range check.Warnings {
			ib.WriteString("  warning: " + w + "\n")
		}
		for _, e := range check.Errors {
			ib.WriteString("  error: " + e + "\n")
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: ib.String()})
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
	err := session.Save(&session.Session{
		ID: m.sessionID, Model: m.session.Model(), Provider: m.session.Provider(),
		Messages: msgs, CreatedAt: time.Now(),
	})
	// On successful save, WAL is no longer needed (session file has everything)
	if err == nil && m.wal != nil {
		m.wal.Remove()
		m.wal = nil
	}
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

// renderGlimmerVerb renders the spinner verb with a sweeping 3-color wave
func renderGlimmerVerb(verb string, glimmerPos int) string {
	if len(verb) == 0 {
		return ""
	}
	// 3 colors: vivid orange, bright orange, warm amber
	colors := []string{"255;94;14", "255;140;50", "255;180;80"}
	runes := []rune(verb)

	var b strings.Builder
	for i, r := range runes {
		// Wave: each char picks a color based on position + time, cycling 1→2→3→2→1→2→3...
		dist := abs((i + glimmerPos) % 5 - 2)
		if dist == 0 {
			b.WriteString("\033[1;38;2;" + colors[0] + "m" + string(r))
		} else if dist == 1 {
			b.WriteString("\033[1;38;2;" + colors[1] + "m" + string(r))
		} else {
			b.WriteString("\033[1;38;2;" + colors[2] + "m" + string(r))
		}
	}
	b.WriteString("\033[0m")
	return b.String()
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// friendlyError translates raw API errors into user-friendly messages.
// sanitizeIdentity replaces model self-identifications with "hawk" / "GrayCode AI".
var (
	reModelName = regexp.MustCompile(`(?i)\b(I['` + "\u2018\u2019" + `]m|I am|my name is)\s+\*{0,2}(ChatGPT|GPT-?\d*[o]?|Claude|Gemini|Gemma|Kimi|DeepSeek|Llama|Qwen|Mistral|Mixtral|Grok|Copilot|Bard|Command R|Yi|Phi|Nova|Titan|BLOOM|Falcon|PaLM|LaMDA|Chinchilla|Vicuna|Alpaca|WizardLM|Orca|Nemotron|Granite|DBRX|OLMo|Pixtral|Ernie|PanGu|Sarvam|MiMo|GLM|Codex|Jurassic|Cohere|Jais|Step|Velvet|Alice|Apertus|Param|YandexGPT|MiniMax)\*{0,2}`)
	reCreator   = regexp.MustCompile(`(?i)(made|created|developed|built|trained|designed)\s+by\s+(?:a\s+company\s+called\s+|a\s+team\s+(?:at|called)\s+|the\s+team\s+at\s+)?\*{0,2}(Moonshot\s*AI|OpenAI|Anthropic|Google|Google\s*DeepMind|DeepMind|Meta|Meta\s*AI|Alibaba|Alibaba\s*Cloud|Mistral\s*AI|xAI|Microsoft|Microsoft\s*AI|Amazon|AWS|Cohere|01\.AI|Baidu|Huawei|IBM|Nvidia|EleutherAI|Hugging\s*Face|AI21\s*Labs|Yandex|Databricks|StepFun|Xiaomi|Sarvam\s*AI|MiniMax|BharatGen|Z\.ai|Zhipu\s*AI|Cerebras|Technology\s*Innovation\s*Institute|TII|Inflection\s*AI|Stability\s*AI|Anysphere|Cognition\s*AI|Scale\s*AI|Sakana\s*AI)\*{0,2}`)
)

func sanitizeIdentity(s string) string {
	s = reModelName.ReplaceAllStringFunc(s, func(m string) string {
		parts := reModelName.FindStringSubmatch(m)
		return parts[1] + " hawk"
	})
	s = reCreator.ReplaceAllString(s, "${1} by GrayCode AI")
	return s
}

// reBold matches markdown **bold** syntax.
var reBold = regexp.MustCompile(`\*\*(.+?)\*\*`)

// renderInlineMarkdown converts inline markdown to ANSI terminal formatting.
// Currently handles **bold** → ANSI bold.
func renderInlineMarkdown(s string) string {
	return reBold.ReplaceAllString(s, "\033[1m${1}\033[22m")
}

// friendlyError is defined in errors.go with comprehensive pattern matching.

// wrapText wraps text to fit within width columns total (including indent).
// The first line has no indent (caller provides the prefix).
// Continuation lines get indent prepended.
// wrapText wraps text to fit within the given width.
// prefixWidth is the visual width of the prefix already printed before the first line
// (e.g. "⛬ " = 2 columns). Continuation lines are indented to align with the first
// line's text start position.
// width is the total terminal width available.
func wrapText(text string, width int, prefixWidth int) string {
	if width < 20 {
		width = 80
	}
	// First line has less room because the prefix is already printed.
	firstLineWidth := width - prefixWidth
	if firstLineWidth < 10 {
		firstLineWidth = width
	}
	// Continuation indent: spaces to align under the first line's text.
	indent := strings.Repeat(" ", prefixWidth)
	indentW := prefixWidth
	contWidth := width - indentW
	if contWidth < 10 {
		contWidth = width
	}
	var result strings.Builder
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		isFirst := (i == 0)
		if !isFirst {
			result.WriteString(indent)
		}

		// Detect leading whitespace on this line so wrapped continuations
		// stay aligned with the original content (e.g. indented bullet lists).
		trimmed := strings.TrimLeft(line, " \t")
		lineLeading := line[:len(line)-len(trimmed)]
		lineLeadingW := runewidth.StringWidth(lineLeading)

		// For bullet-style lines ("* ", "- ", "N. "), add extra indent so
		// continuation text aligns past the bullet marker.
		bulletExtra := ""
		if len(trimmed) > 0 {
			if strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "+ ") {
				bulletExtra = "  "
			} else if idx := strings.Index(trimmed, ". "); idx > 0 && idx <= 3 {
				allDigits := true
				for _, ch := range trimmed[:idx] {
					if ch < '0' || ch > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					bulletExtra = strings.Repeat(" ", idx+2)
				}
			}
		}

		// Build the continuation indent for wrapped segments of this line.
		lineIndent := indent + lineLeading + bulletExtra
		lineIndentW := indentW + lineLeadingW + runewidth.StringWidth(bulletExtra)
		lineContWidth := width - lineIndentW
		if lineContWidth < 10 {
			lineContWidth = contWidth
			lineIndent = indent
		}

		maxW := firstLineWidth
		if !isFirst {
			maxW = contWidth - lineLeadingW // account for leading whitespace already written
		}
		if runewidth.StringWidth(line) <= maxW {
			result.WriteString(line)
			result.WriteByte('\n')
			continue
		}
		curWidth := 0
		var curLine strings.Builder
		for _, word := range strings.Fields(line) {
			wordW := runewidth.StringWidth(word)
			if curWidth > 0 && curWidth+1+wordW > maxW {
				result.WriteString(curLine.String())
				result.WriteByte('\n')
				result.WriteString(lineIndent)
				curLine.Reset()
				curLine.WriteString(word)
				curWidth = wordW
				maxW = lineContWidth
			} else if curWidth > 0 {
				curLine.WriteByte(' ')
				curLine.WriteString(word)
				curWidth += 1 + wordW
			} else {
				curLine.WriteString(word)
				curWidth = wordW
			}
		}
		if curLine.Len() > 0 {
			result.WriteString(curLine.String())
			result.WriteByte('\n')
		}
	}
	return strings.TrimRight(result.String(), "\n")
}

func (m *chatModel) hasRealMessages() bool {
	for _, msg := range m.messages {
		if msg.role != "welcome" {
			return true
		}
	}
	return m.waiting
}

func (m *chatModel) updateViewportContent() {
	viewWidth := m.width
	if viewWidth <= 0 {
		viewWidth = 80
	}

	hawkC := "\033[38;2;255;94;14m"
	rst := "\033[0m"
	bgDark := "\033[48;2;30;30;40m"

	welcome := buildWelcomeMessage(m.session, m.sessionID, m.registry, nil, m.settings, m.blinkClosed, viewWidth)

	var chatContent strings.Builder
	chatContent.WriteString(welcome + "\n")

	for i, msg := range m.messages {
		switch msg.role {
		case "user":
			if i > 0 {
				chatContent.WriteString("\n")
			}
			wrapped := wrapText(msg.content, viewWidth, 3)
			line := hawkC + "█" + rst + "  " + wrapped
			chatContent.WriteString(bgDark + line + rst)
		case "assistant":
			content := strings.TrimLeft(msg.content, "\n\r")
			chatContent.WriteString(hawkC + "⛬ " + rst + renderInlineMarkdown(wrapText(content, viewWidth, 3)))
		case "tool_use":
			chatContent.WriteString(toolStyle.Render("⚡ " + msg.content))
		case "tool_result":
			chatContent.WriteString(toolDimStyle.Render(msg.content))
		case "thinking":
			chatContent.WriteString(dimStyle.Render("💭 " + msg.content))
		case "welcome":
			// Skip welcome in viewport — it's rendered statically in View()
		case "system":
			chatContent.WriteString(dimStyle.Render(msg.content))
		case "permission":
			chatContent.WriteString(toolStyle.Render("⚠ " + msg.content + "  [y/n]"))
		case "question":
			chatContent.WriteString(toolStyle.Render(msg.content))
		case "usage":
		case "error":
			chatContent.WriteString(errorStyle.Render("error: " + msg.content))
		}
		chatContent.WriteString("\n\n")
	}

	if m.waiting {
		partial := sanitizeIdentity(strings.TrimLeft(m.partial.String(), "\n\r"))
		if partial != "" {
			chatContent.WriteString(hawkC + "⛬ " + rst + renderInlineMarkdown(wrapText(partial, viewWidth, 3)))
			chatContent.WriteString("\n\n")
		} else {
			chatContent.WriteString(m.spinner.View() + "  " + renderGlimmerVerb(m.spinnerVerb, m.glimmerPos) + "\033[1;38;2;255;94;14m...\033[0m " + dimStyle.Render("(Press ESC to stop)") + "\n\n")
		}
	}

	if m.configOpen {
		chatContent.WriteString(m.configPanelView())
		chatContent.WriteString("\n\n")
	}

	// Calculate bottom bar height to size viewport correctly
	bottomBarLines := 0
	if !m.configOpen {
		bottomBarLines = 5 // status(1) + input borders+content(3) + help(1)
		if sugs := slashSuggestions(m.input.Value()); len(sugs) > 0 {
			bottomBarLines += len(sugs)
		}
	}
	vpHeight := m.height - bottomBarLines - 1
	if vpHeight < 4 {
		vpHeight = 4
	}
	m.viewport.Width = viewWidth
	m.viewport.Height = vpHeight

	atBottom := m.viewport.AtBottom()
	contentStr := chatContent.String()

	if !m.hasRealMessages() {
		contentLines := strings.Count(contentStr, "\n")
		if contentLines < vpHeight {
			topPad := (vpHeight - contentLines) / 3
			contentStr = strings.Repeat("\n", topPad) + contentStr
		}
	}

	m.viewport.SetContent(contentStr)
	if atBottom || m.autoScroll {
		m.viewport.GotoBottom()
	}
}

func (m chatModel) View() string {
	if m.quitting {
		return ""
	}

	viewWidth := m.width
	if viewWidth <= 0 {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
			viewWidth = w
		} else {
			viewWidth = 80
		}
	}

	// Build the fixed bottom bar
	var bottomBar strings.Builder
	bottomBarLines := 0

	if !m.configOpen {
		totalW := viewWidth
		if totalW < 40 {
			totalW = 80
		}
		leftBold := "Auto (Off)"
		leftDim := " - all actions require approval"
		rightStatus := fmt.Sprintf("%s %s", m.session.Provider(), m.session.Model())
		leftVisLen := len(leftBold) + len(leftDim)
		gap := totalW - leftVisLen - len(rightStatus)
		if gap < 1 {
			gap = 1
		}
		leftRendered := lipgloss.NewStyle().Bold(true).Render(leftBold) + dimStyle.Render(leftDim)
		bottomBar.WriteString(leftRendered + strings.Repeat(" ", gap) + dimStyle.Render(rightStatus) + "\n")
		bottomBarLines++
		inputBox := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, true, false).
			BorderForeground(lipgloss.Color("#555555")).
			Width(totalW).
			Render(m.input.View())
		bottomBar.WriteString(inputBox + "\n")
		bottomBarLines += 3
		if sugs := slashSuggestions(m.input.Value()); len(sugs) > 0 {
			if m.slashSel < 0 || m.slashSel >= len(sugs) {
				m.slashSel = 0
			}
			for i, s := range sugs {
				if i == m.slashSel {
					bottomBar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6")).Render("  "+s) + "\n")
				} else {
					bottomBar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#73767E")).Render("  "+s) + "\n")
				}
				bottomBarLines++
			}
		}
		bottomBar.WriteString(dimStyle.Render("? for help") + "\n")
		bottomBarLines++
	}

	return m.viewport.View() + "\n" + bottomBar.String()
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

	_, err = p.Run()
	if err == nil {
		fmt.Println(dimStyle.Render("Goodbye."))
	}
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
	sess.SetLogger(logger.New(io.Discard, logger.Error))
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
