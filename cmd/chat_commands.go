package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/engine/project"
	"github.com/GrayCodeAI/hawk/internal/feature/shellmode"
	"github.com/GrayCodeAI/hawk/internal/home"
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/multiagent/parallel"
	analytics "github.com/GrayCodeAI/hawk/internal/observability"
	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/recipe"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

func slashCommands() []string {
	return allSlashCommands
}

var allSlashCommands = []string{
	"/add", "/add-dir", "/agents", "/agents-init", "/audit", "/branch", "/branches", "/bughunter", "/clean", "/clear",
	"/check", "/color", "/commit", "/compact", "/compress", "/config", "/context", "/council", "/design",
	"/copy", "/cost", "/cron", "/ctx", "/diff", "/doctor", "/drop", "/effort", "/env", "/exit", "/explain",
	"/export", "/fast", "/feedback", "/files", "/focus", "/follow", "/fork", "/glm", "/help", "/history", "/home", "/hooks", "/init",
	"/integrity", "/keybindings", "/learn", "/lint", "/loop", "/mcp", "/memory", "/metrics", "/model", "/new",
	"/hunt", "/insights", "/mode", "/output-style", "/party", "/permissions", "/pin", "/plugin", "/plugins",
	"/power", "/pr-comments", "/provider-status", "/quit", "/recipe", "/recover", "/reflect", "/refresh-model-catalog", "/release-notes",
	"/image", "/reload-plugins", "/remote-env", "/rename", "/render", "/research", "/resume", "/retry", "/review", "/rewind",
	"/run", "/btw", "/brainstorm", "/checkpoint", "/dream", "/away", "/investigate", "/search", "/security-review", "/session", "/share", "/skills", "/snapshot", "/soul", "/spec", "/stale", "/stats",
	"/mouse", "/select", "/status", "/statusline", "/summary", "/tag", "/taste", "/tasks", "/test", "/theme",
	"/think", "/think-back", "/thinkback", "/thinkback-play", "/tokens", "/tools", "/ultrareview", "/undo", "/upgrade", "/usage",
	"/version", "/vibe", "/vim", "/voice", "/welcome", "/ecosystem", "/path", "/yaad",
}

func (m *chatModel) slashSuggestionsFor(input string) []string {
	if input == m.slashSugInput {
		return m.slashSugCache
	}
	m.slashSugInput = input
	m.slashSugCache = slashSuggestions(input)
	return m.slashSugCache
}

// slashMenuOpen is true while the / command picker is visible (Cursor hides the footer then).
func (m *chatModel) slashMenuOpen() bool {
	return len(m.slashSuggestionsFor(m.input.Value())) > 0
}

func (m *chatModel) visibleSlashSuggestionLines() int {
	n := len(m.slashSuggestionsFor(m.input.Value()))
	if n > 6 {
		return 6
	}
	return n
}

func (m *chatModel) syncInputLayout() bool {
	if m.configOpen {
		return false
	}
	lines := strings.Count(m.input.Value(), "\n") + 1
	if lines > 10 {
		lines = 10
	}
	visible := m.visibleSlashSuggestionLines()
	key := lines<<16 | visible
	if key == m.layoutKey {
		return false
	}
	m.layoutKey = key
	return true
}

func slashAliases() map[string]string {
	return nil
}

var slashDescriptions = map[string]string{
	"/add":             "Add files to conversation context",
	"/add-dir":         "Add a directory to context",
	"/agents":          "List active agents",
	"/agents-init":     "Generate AGENTS.md from project template",
	"/audit":           "Show tool audit summary",
	"/branch":          "Show git branch info",
	"/btw":             "Side note without triggering a response",
	"/bughunter":       "Hunt for bugs in the codebase",
	"/check":           "Review diff, find issues, auto-fix safe ones, verify before ship",
	"/design":          "Build or improve UI — use /design screenshot|system|component|regress for advanced modes",
	"/hunt":            "Diagnose root cause of errors before fixing (Waza method)",
	"/think":           "Turn rough idea into approved plan before coding (Waza method)",
	"/clean":           "Delete old sessions",
	"/clear":           "Clear conversation",
	"/color":           "Change agent color",
	"/commit":          "Auto-commit changes with AI message",
	"/compact":         "Compress conversation to save tokens",
	"/compress":        "Compress old sessions",
	"/config":          "Open settings panel",
	"/context":         "Show current context",
	"/copy":            "Copy chat or input to clipboard (/copy all|input|last|assistant)",
	"/cost":            "Show token usage and cost",
	"/council":         "Run LLM Council (multi-model consensus)",
	"/diff":            "Show git diff (preview changes)",
	"/doctor":          "Run diagnostics (build, test, lint)",
	"/drop":            "Remove file from context",
	"/effort":          "Set reasoning effort level",
	"/glm":             "Toggle GLM/Z.ai extended reasoning (on|off|default)",
	"/env":             "Show environment info",
	"/exit":            "Save and exit",
	"/explain":         "Trace code back to the commit that created it",
	"/export":          "Export session",
	"/follow":          "Toggle stream follow (auto-scroll)",
	"/home":            "Scroll to welcome screen",
	"/feedback":        "Submit feedback about hawk",
	"/fast":            "Toggle fast mode",
	"/files":           "Show modified files",
	"/focus":           "Narrow agent attention to specific files/dirs",
	"/fork":            "Fork conversation to try a different approach",
	"/branches":        "List or switch conversation branches",
	"/help":            "Show all commands",
	"/history":         "List saved sessions",
	"/hooks":           "Show configured hooks",
	"/init":            "Analyze project structure",
	"/integrity":       "Validate session integrity",
	"/lint":            "Run linter, add issues to context",
	"/loop":            "Schedule recurring command",
	"/mcp":             "Show MCP server status",
	"/memory":          "Show AGENTS.md project instructions",
	"/metrics":         "Show session metrics",
	"/model":           "Switch or view current model",
	"/new":             "Start a fresh session",
	"/permissions":     "Permission Center for tier, sandbox, mode, and rules",
	"/pin":             "Pin last N messages to protect from compaction",
	"/parallel":        "Run N agents in parallel on independent tasks",
	"/plugins":         "List installed plugins",
	"/power":           "Set power level (1-10)",
	"/quit":            "Save and exit",
	"/recover":         "Scan for interrupted sessions and resume",
	"/refactor":        "Agent-driven refactoring: dedup, dead code, lint fixes",
	"/resume":          "Resume a saved session",
	"/retry":           "Redo last message",
	"/review":          "Code review for bugs and issues",
	"/rewind":          "Undo last exchange",
	"/run":             "Run command, add output to context",
	"/search":          "Search across sessions",
	"/select":          "Pause TUI for native text selection (Ctrl+\\)",
	"/mouse":           "Toggle TUI mouse capture for native click-drag copy",
	"/snapshot":        "Manage file snapshots: list, restore <hash>, diff <hash>",
	"/stale":           "Show stale rules that may need updating or removal",
	"/security-review": "Security audit",
	"/skills":          "List skills or manage: search, install, trending, info, remove, update, feedback, publish, audit",
	"/learn":           "LLM-powered skill advisor (/learn deep for source analysis)",
	"/stats":           "Show analytics stats",
	"/status":          "Show session info",
	"/summary":         "Summarize the session",
	"/tasks":           "Show task list",
	"/test":            "Run tests, add failures to context",
	"/tokens":          "Show token estimate",
	"/tools":           "List enabled tools",
	"/undo":            "Undo the most recent file change",
	"/usage":           "Show cost summary",
	"/version":         "Show hawk version",
	"/vim":             "Toggle vim mode",
	"/welcome":         "Show welcome screen",
	"/ecosystem":       "Show eyrie, yaad, and tok integration status",
	"/path":            "Developer path readiness (setup, security, sandbox)",
	"/yaad":            "Show yaad memory (use /yaad search <query> to search)",
	"/cron":            "Show scheduled jobs",
	"/keybindings":     "Show keyboard shortcuts",
	"/output-style":    "Change output style",
	"/plugin":          "Manage plugins",
	"/pr-comments":     "Address PR comments",
	"/provider-status": "Show provider info",
	"/release-notes":   "Draft release notes",
	"/reload-plugins":  "Reload all plugins",
	"/remote-env":      "Show remote environment",
	"/rename":          "Rename current session",
	"/render":          "Export repo as CXML to clipboard",
	"/research":        "Start autonomous research loop",
	"/session":         "Show session info",
	"/share":           "Share session",
	"/statusline":      "Show status line info",
	"/tag":             "Tag current session",
	"/taste":           "Show learned taste preferences",
	"/theme":           "Change visual theme",
	"/think-back":      "Review reasoning decisions",
	"/thinkback":       "Review reasoning decisions",
	"/thinkback-play":  "Replay reasoning path",
	"/upgrade":         "Check for updates",
	"/vibe":            "Start vibe coding loop",
	"/voice":           "Toggle voice input",
	"/ctx":             "Show conversation context visualization",
	"/insights":        "Generate session patterns and improvements report",
	"/spec":            "Generate specification from context",
	"/ultrareview":     "Deep adversarial code review",
}

func slashSuggestions(input string) []string {
	v := strings.TrimSpace(input)
	if !strings.HasPrefix(v, "/") || strings.Contains(v, " ") {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, c := range allSlashCommands {
		if strings.HasPrefix(c, v) {
			seen[c] = true
			desc := slashDescriptions[c]
			if desc != "" {
				out = append(out, c+"  "+desc)
			} else {
				out = append(out, c)
			}
		}
	}
	for alias, target := range slashAliases() {
		if strings.HasPrefix(alias, v) && !seen[target] {
			seen[alias] = true
			out = append(out, alias+" → "+target)
		}
	}
	if len(out) == 1 && strings.HasPrefix(out[0], v+" ") && strings.Fields(out[0])[0] == v {
		return nil
	}
	return out
}

func applySlashSuggestion(input string) string {
	choice := strings.TrimSpace(input)
	if before, _, ok := strings.Cut(choice, " → "); ok {
		choice = before
	}
	parts := strings.Fields(choice)
	if len(parts) > 0 {
		choice = parts[0]
	}
	if target, ok := slashAliases()[choice]; ok {
		choice = target
	}
	return choice + " "
}

func (m *chatModel) handleCommand(text string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(text)
	cmd := parts[0]

	// Namespaced skill invocation: /vendor:skill-name [args...]
	if strings.Contains(cmd, ":") && strings.HasPrefix(cmd, "/") {
		return m.handleNamespacedSkill(cmd, text)
	}

	switch cmd {
	case "/quit", "/exit":
		return m.handleSessionCommand(cmd, parts, text)
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
		return m.handleSessionCommand(cmd, parts, text)
	case "/compact":
		return m.handleSessionCommand(cmd, parts, text)
	case "/diff":
		stat, _ := gitOutput("diff", "--stat")
		diff, _ := gitOutput("diff")
		if strings.TrimSpace(diff) == "" {
			stat, _ = gitOutput("diff", "--cached", "--stat")
			diff, _ = gitOutput("diff", "--cached")
		}
		if strings.TrimSpace(diff) == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No changes detected."})
			return m, nil
		}
		output := stat + "\n\n" + diff
		if len(output) > 10000 {
			output = stat + "\n\n(diff too large, showing stat only)"
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: output})
		return m, nil
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
/copy               — Copy chat or input (all|input|last|assistant)
/cost               — Token usage and cost
/cron               — List scheduled cron jobs
/diff               — Review changes
/doctor             — Run diagnostics
/dream              — Run memory consolidation
/away               — Generate session recap
/effort <level>     — Set reasoning effort (low/medium/high)
/env                — Show provider environment status
/export             — Export session to JSON
/fast               — Toggle fast mode
/feedback <msg>     — Submit feedback (saved to ~/.hawk/feedback/)
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
/permissions       — Show tier, sandbox, mode, rules, and effective behavior
/permissions tier  — Set the autonomy tier
/permissions mode  — Set the advanced permission mode
/permissions allow — Add an allow rule
/permissions deny  — Add a deny rule
/permissions save  — Persist the current permission policy
/plugins            — List installed plugins
/pr-comments        — Ask hawk to handle PR comments
/release-notes      — Draft release notes
/rename <name>      — Rename current session
/resume <id>        — Resume session
/review             — Ask hawk to review changes
/rewind             — Undo last exchange
/security-review    — Ask hawk to review security risks
/select             — Pause TUI for native text selection
/mouse              — Toggle mouse capture (off = click-drag copy)
/share              — Share session
/learn              — LLM-powered skill advisor (deep, update)
/skills             — List, search, install, remove skills
/stale              — Show stale rules that may need removal
/stats              — Session statistics
/status             — Session status
/summary            — Summarize the current session
/tag <label>        — Tag session
/taste              — Show learned coding style preferences
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
	case "/council":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /council <question>"})
			return m, nil
		}
		query := strings.TrimSpace(strings.TrimPrefix(text, "/council"))
		cfg := engine.CouncilConfig{
			Models:   engine.DefaultCouncilModels(),
			Chairman: m.session.Model(),
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("🏛 Council convened: %s (chairman: %s)", strings.Join(cfg.Models, ", "), cfg.Chairman)})
		m.messages = append(m.messages, displayMsg{role: "system", content: "Stage 1: Querying all models..."})

		result, err := engine.RunCouncil(context.Background(), query, cfg, m.session)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Council failed: " + err.Error()})
			return m, nil
		}

		for _, r := range result.Responses {
			preview := r.Response
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("  [%s] %s", r.Model, preview)})
		}

		m.messages = append(m.messages, displayMsg{role: "system", content: "Stage 2: Ranking responses..."})
		for _, r := range result.Rankings {
			preview := r.Ranking
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("  [%s] %s", r.Model, preview)})
		}

		m.messages = append(m.messages, displayMsg{role: "system", content: "Stage 3: Chairman synthesizing..."})
		m.messages = append(m.messages, displayMsg{role: "assistant", content: result.Synthesis})
		return m, nil
	case "/metrics":
		m.messages = append(m.messages, displayMsg{role: "system", content: m.session.Metrics().Format()})
		return m, nil
	case "/mode":
		if len(parts) == 1 {
			// Show current mode
			current := m.modeManager.Current()
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Mode: %s (auto | shell | agent)", current.String())})
			return m, nil
		}
		arg := strings.ToLower(parts[1])
		if arg == "toggle" {
			newMode := m.modeManager.Toggle()
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Mode → %s", newMode.String())})
			return m, nil
		}
		mode, ok := shellmode.ParseMode(arg)
		if !ok {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /mode [auto|shell|agent|toggle]"})
			return m, nil
		}
		m.modeManager.Set(mode)
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Mode → %s", mode.String())})
		return m, nil
	case "/model":
		if len(parts) == 1 {
			next, cmd := m.openConfigAtTab(configTabModels)
			*m = next
			return m, cmd
		}
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/model"))
		arg = strings.TrimSpace(strings.TrimPrefix(arg, "set"))
		if arg == "" {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /model <model-name> or /model set <model-name>"})
			return m, nil
		}
		// Validate model against known models for current provider
		known := configModelChoices(m.configModelOptions, false)
		if len(known) > 0 {
			found := false
			for i, k := range known {
				if strings.EqualFold(k, arg) || strings.EqualFold(m.configModelOptions[i].ID, arg) {
					arg = m.configModelOptions[i].ID
					found = true
					break
				}
			}
			if !found {
				// Show available models as hint
				hint := "Unknown model: " + arg + "\nAvailable models for " + m.session.Provider() + ":\n"
				max := 10
				if len(known) < max {
					max = len(known)
				}
				for _, k := range known[:max] {
					hint += "  " + k + "\n"
				}
				if len(known) > 10 {
					hint += fmt.Sprintf("  ... and %d more (use /model to browse)", len(known)-10)
				}
				m.messages = append(m.messages, displayMsg{role: "error", content: hint})
				return m, nil
			}
		}
		if hawkconfig.DeploymentRoutingEnabled(m.settings) {
			arg = hawkconfig.ResolveCanonicalModel(arg)
		}
		prevModel := m.session.Model()
		if strings.EqualFold(strings.TrimSpace(prevModel), strings.TrimSpace(arg)) {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Already using %s — no change.", prevModel)})
			return m, nil
		}
		if err := hawkconfig.SetGlobalSetting("model", arg); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		// Mid-session switch: SetModel only re-routes subsequent requests; the
		// conversation history on the session is left intact, so context carries
		// over to the new model. Count messages to confirm nothing was dropped.
		msgCount := len(m.session.RawMessages())
		m.session.SetModel(arg)
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf(
			"Model switched: %s → %s\nConversation history preserved (%d messages); new requests use the new model.\nSaved in eyrie (provider.json).",
			prevModel, m.session.Model(), msgCount,
		)})
		return m, nil
	case "/branches":
		if m.session.ConvoDAG == nil {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No conversation branches (DAG not active)."})
			return m, nil
		}
		headID := m.session.ConvoHead()
		if headID == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No conversation history."})
			return m, nil
		}
		// If /branches <id> — switch to that branch
		if len(parts) >= 2 {
			targetID := parts[1]
			if err := m.session.SwitchBranch(targetID); err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				return m, nil
			}
			m.messages = nil
			m.invalidateViewportCache()
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Switched to branch %s", targetID)})
			for _, msg := range m.session.RawMessages() {
				m.messages = append(m.messages, displayMsg{role: msg.Role, content: msg.Content})
			}
			return m, nil
		}
		// List branches
		history, err := m.session.ConvoDAG.History(context.Background(), headID)
		if err != nil || len(history) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No branches available (linear conversation).\nUse /fork to create a branch."})
			return m, nil
		}
		var branchInfo strings.Builder
		branchInfo.WriteString("Conversation branches:\n")
		found := false
		for i := len(history) - 1; i >= 0; i-- {
			branches, _ := m.session.ListBranches(history[i].ID)
			if len(branches) > 1 {
				found = true
				branchInfo.WriteString(fmt.Sprintf("\n  Fork at: %s (%s)\n", history[i].ID[:8], history[i].Role))
				for _, b := range branches {
					marker := "  "
					if b.ID == headID {
						marker = "→ "
					}
					branchInfo.WriteString(fmt.Sprintf("    %s%s (%s: %s)\n", marker, b.ID[:8], b.Role, truncate(b.Content, 40)))
				}
			}
		}
		if !found {
			branchInfo.WriteString("  No fork points yet.\n  Use /fork to create a branch.")
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: branchInfo.String()})
		return m, nil
	case "/version":
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("hawk v%s", DisplayVersion())})
		return m, nil
	case "/env":
		m.messages = append(m.messages, displayMsg{role: "system", content: envSummary(m.session.Provider(), m.session.Model())})
		return m, nil
	case "/focus":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /focus <path> [path...]"})
			return m, nil
		}
		paths := strings.TrimSpace(strings.TrimPrefix(text, "/focus"))
		m.session.AppendSystemContext("FOCUS: Only work with these files/directories: " + paths + ". Ignore files outside this scope unless explicitly asked.")
		m.messages = append(m.messages, displayMsg{role: "system", content: "Focus set: " + paths})
		return m, nil
	case "/pin":
		n := 2 // default: pin last exchange (user + assistant)
		if len(parts) >= 2 {
			if parsed, err := strconv.Atoi(parts[1]); err == nil && parsed > 0 {
				n = parsed
			}
		}
		m.session.PinnedMessages = n
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Pinned last %d messages (protected from compaction).", n)})
		return m, nil
	case "/files":
		m.messages = append(m.messages, displayMsg{role: "system", content: filesSummary()})
		return m, nil
	case "/history":
		return m.handleSessionCommand(cmd, parts, text)
	case "/render":
		renderPath := ""
		if len(parts) >= 2 {
			renderPath = strings.TrimSpace(strings.TrimPrefix(text, "/render"))
		}
		cxml, stats, err := renderCXML(renderPath)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		if err := copyToClipboard(cxml); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Failed to copy: " + err.Error()})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("📋 CXML copied to clipboard.\n%s", stats)})
		return m, nil
	case "/recover":
		return m.handleSessionCommand(cmd, parts, text)
	case "/resume":
		return m.handleSessionCommand(cmd, parts, text)
	case "/commit":
		stat, _ := gitOutput("diff", "--stat")
		if strings.TrimSpace(stat) == "" {
			stat, _ = gitOutput("diff", "--cached", "--stat")
		}
		if strings.TrimSpace(stat) != "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Changes to commit:\n" + stat})
		}
		return m.startPromptCommand("/commit", "Review the changes I've made, then create a git commit with an appropriate commit message. Use git add for specific files and git commit.")
	case "/doctor":
		return m.startPromptCommand("/doctor", "Run diagnostics on this project: check if it builds, run tests, check for lint errors. Report any issues found.")
	case "/init":
		initPrompt := "Analyze this project: read the README, check the directory structure, identify the language/framework, build system, and test runner. Report progress as you go (e.g., 'Analyzing file 5/20...'). Give me a brief summary."
		if _, err := os.Stat("AGENTS.md"); os.IsNotExist(err) {
			pt := detectAgentsProjectType()
			initPrompt += fmt.Sprintf("\n\nNote: No AGENTS.md found. I detected project type %q. After your analysis, suggest running /agents-init to generate one.", pt)
		}
		return m.startPromptCommand("/init", initPrompt)
	case "/agents-init":
		if _, err := os.Stat("AGENTS.md"); err == nil {
			m.messages = append(m.messages, displayMsg{role: "system", content: "AGENTS.md already exists. Remove it first to regenerate."})
			return m, nil
		}
		pt := detectAgentsProjectType()
		content := GenerateAgentsTemplate(pt)
		if err := os.WriteFile("AGENTS.md", []byte(content), 0o644); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Failed to write AGENTS.md: " + err.Error()})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Created AGENTS.md (detected: %s). Edit it to match your project.", pt)})
		return m, nil
	case "/review":
		return m.startPromptCommand("/review", engine.ReviewPrompt(nil))
	case "/refactor":
		return m.handleRefactorCommand(parts, text)
	case "/party":
		topic := strings.TrimSpace(strings.TrimPrefix(text, "/party"))
		if topic == "" {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /party <topic to discuss>"})
			return m, nil
		}
		ps := engine.NewPartySession(topic, nil)
		return m.startPromptCommand("/party", ps.GeneratePrompt(1))
	case "/brainstorm":
		topic := strings.TrimSpace(strings.TrimPrefix(text, "/brainstorm"))
		if topic == "" {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /brainstorm <topic>"})
			return m, nil
		}
		return m.startPromptCommand("/brainstorm", engine.BrainstormPrompt(engine.BrainstormSetup, topic, ""))
	case "/investigate":
		ctx := strings.TrimSpace(strings.TrimPrefix(text, "/investigate"))
		if ctx == "" {
			ctx = "the issue described above"
		}
		return m.startPromptCommand("/investigate", engine.InvestigatePrompt(engine.InvestigateReproduce, ctx))
	case "/checkpoint":
		return m.startPromptCommand("/checkpoint", engine.CheckpointPrompts(engine.CheckpointOrientation, nil))
	case "/dream":
		projectDir, _ := os.Getwd()
		status := memory.YaadStatus()
		if strings.Contains(status, "not initialized") || strings.Contains(status, "no memories") {
			m.messages = append(m.messages, displayMsg{role: "system", content: status + "\nRun 'yaad' to start storing memories, or use /memory to view AGENTS.md."})
			return m, nil
		}
		yaadCtx := memory.LoadYaadContext(projectDir)
		if yaadCtx == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No yaad memories found to consolidate."})
			return m, nil
		}
		dreamPrompt := `Review the yaad memories below and consolidate them into a coherent summary.
Focus on: recurring patterns, user preferences learned, project context that should persist,
and any corrections or feedback. Remove redundant or outdated entries.
Write the consolidated result as clear, organized yaad memory nodes.

` + yaadCtx
		m.messages = append(m.messages, displayMsg{role: "system", content: "Running memory consolidation...\n" + status})
		return m.startPromptCommand("/dream", dreamPrompt)
	case "/away":
		messages := m.session.RawMessages()
		if len(messages) < 4 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Not enough conversation history for a recap."})
			return m, nil
		}
		// Build recent conversation summary
		start := 0
		if len(messages) > 30 {
			start = len(messages) - 30
		}
		var summary strings.Builder
		for _, msg := range messages[start:] {
			preview := msg.Content
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			if msg.Role == "user" && preview != "" {
				summary.WriteString(fmt.Sprintf("User: %s\n", preview))
			} else if msg.Role == "assistant" && msg.Content != "" {
				summary.WriteString(fmt.Sprintf("Assistant: %s\n", preview))
			}
		}
		awayPrompt := fmt.Sprintf(`You are generating a brief "while you were away" recap for a coding session.

Rules:
- 1-3 sentences maximum
- Focus on what was accomplished and what's next
- Do NOT include status reports or tool call details
- Be concise and actionable

Recent conversation:
%s

Generate the recap:`, summary.String())
		m.messages = append(m.messages, displayMsg{role: "system", content: "Generating recap..."})
		return m.startPromptCommand("/away", awayPrompt)
	case "/reflect":
		return m.startPromptCommand("/reflect", engine.ReflectPrompt("this session so far"))
	case "/spec":
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/spec"))
		if arg == "" {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /spec <what to build>"})
			return m, nil
		}
		return m.startPromptCommand("/spec", engine.SpecGeneratePrompt(arg))
	case "/soul":
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/soul"))
		if arg == "init" {
			return m.startPromptCommand("/soul init", engine.InitSoulPrompt())
		}
		soul := engine.LoadCodingSoul()
		if soul.Style == "" && soul.Preferences == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No soul file found. Run /soul init to generate one."})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: soul.ForPrompt()})
		}
		return m, nil
	case "/recipe":
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/recipe"))
		if arg == "" || arg == "list" {
			rn := recipe.NewRunner()
			recipes := rn.List()
			if len(recipes) == 0 {
				m.messages = append(m.messages, displayMsg{role: "system", content: "No recipes found in ~/.hawk/recipes/ or .hawk/recipes/"})
			} else {
				var list string
				for _, r := range recipes {
					list += fmt.Sprintf("  • %s — %s\n", r.Title, r.Description)
				}
				m.messages = append(m.messages, displayMsg{role: "system", content: "Available recipes:\n" + list})
			}
			return m, nil
		}
		rn := recipe.NewRunner()
		for _, r := range rn.List() {
			if strings.EqualFold(r.Title, arg) || strings.Contains(strings.ToLower(r.Title), strings.ToLower(arg)) {
				prompt, err := rn.Execute(context.Background(), r, nil)
				if err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
					return m, nil
				}
				return m.startPromptCommand("/recipe "+r.Title, prompt)
			}
		}
		m.messages = append(m.messages, displayMsg{role: "error", content: "Recipe not found: " + arg})
		return m, nil
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
	case "/think":
		topic := strings.TrimSpace(strings.TrimPrefix(text, "/think"))
		if topic == "" {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /think <what to plan>"})
			return m, nil
		}
		return m.startPromptCommand("/think", buildThinkPrompt(topic))
	case "/hunt":
		symptom := strings.TrimSpace(strings.TrimPrefix(text, "/hunt"))
		if symptom == "" {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /hunt <error or symptom>"})
			return m, nil
		}
		return m.startPromptCommand("/hunt", buildHuntPrompt(symptom))
	case "/snapshot":
		return m.handleSessionCommand(cmd, parts, text)
	case "/check":
		return m.startPromptCommand("/check", buildCheckPrompt())
	case "/design":
		fields := strings.Fields(text)
		if len(fields) >= 2 {
			switch fields[1] {
			case "screenshot":
				path := strings.TrimSpace(strings.TrimPrefix(text, "/design screenshot"))
				if path == "" {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /design screenshot <path/to/screenshot.png>"})
					return m, nil
				}
				return m.startPromptCommand("/design screenshot", buildDesignScreenshotPrompt(path))
			case "system":
				dir := strings.TrimSpace(strings.TrimPrefix(text, "/design system"))
				if dir == "" {
					dir = "."
				}
				return m.startPromptCommand("/design system", buildDesignSystemPrompt(dir))
			case "component":
				rest := strings.TrimSpace(strings.TrimPrefix(text, "/design component"))
				parts2 := strings.Fields(rest)
				name := ""
				fw := ""
				if len(parts2) >= 1 {
					name = parts2[0]
					if len(parts2) >= 2 {
						fw = parts2[1]
					}
				}
				if name == "" {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /design component <name> [framework]"})
					return m, nil
				}
				return m.startPromptCommand("/design component", buildDesignComponentPrompt(name, fw))
			case "regress":
				rest := strings.TrimSpace(strings.TrimPrefix(text, "/design regress"))
				if rest == "" {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /design regress <baseline> [current]"})
					return m, nil
				}
				parts2 := strings.Fields(rest)
				baseline := parts2[0]
				current := "."
				if len(parts2) >= 2 {
					current = parts2[1]
				}
				return m.startPromptCommand("/design regress", buildDesignRegressionPrompt(baseline, current))
			default:
				topic := strings.TrimSpace(strings.TrimPrefix(text, "/design"))
				if topic == "" {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /design <what to build or improve> or /design screenshot|system|component|regress"})
					return m, nil
				}
				return m.startPromptCommand("/design", buildDesignPrompt(topic))
			}
		}
		topic := strings.TrimSpace(strings.TrimPrefix(text, "/design"))
		if topic == "" {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /design <what to build or improve> or /design screenshot|system|component|regress"})
			return m, nil
		}
		return m.startPromptCommand("/design", buildDesignPrompt(topic))
	case "/status":
		toolCount := 0
		if m.registry != nil {
			toolCount = len(m.registry.EyrieTools())
		}
		info := fmt.Sprintf("Session: %s\nModel: %s/%s\nMode: %s\nPermission mode: %s\nMessages: %d\nTools: %d\n%s",
			m.sessionID, m.session.Provider(), m.session.Model(),
			m.modeManager.Current().String(),
			permissionModeLabel(m.session), m.session.MessageCount(), toolCount, m.session.Cost.Summary())
		if len(addDirs) > 0 {
			info += "\nAdditional dirs: " + strings.Join(addDirs, ", ")
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: info})
		return m, nil
	case "/context":
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/context"))
		if arg == "init" {
			cwd, _ := os.Getwd()
			pc := project.NewProjectContext(cwd)
			return m.startPromptCommand("/context init", pc.InitPrompt())
		}
		if arg == "show" {
			cwd, _ := os.Getwd()
			pc := project.NewProjectContext(cwd)
			content := pc.Load()
			if content == "" {
				m.messages = append(m.messages, displayMsg{role: "system", content: "No project context files found. Run /context init to generate."})
			} else {
				m.messages = append(m.messages, displayMsg{role: "system", content: content})
			}
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: hawkconfig.BuildContextWithDirs(addDirs)})
		return m, nil
	case "/memory":
		md := strings.TrimSpace(hawkconfig.LoadAgentsMD())
		if md == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No AGENTS.md or .hawk/AGENTS.md project instructions found.\nUse /yaad for persistent graph memory."})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Project instructions (AGENTS.md):\n" + md})
		}
		return m, nil
	case "/yaad":
		if len(parts) >= 3 && parts[1] == "search" {
			query := strings.TrimSpace(strings.Join(parts[2:], " "))
			if query == "" {
				m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /yaad search <query>"})
				return m, nil
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: memory.FormatYaadSearch(query, 10)})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: memory.FormatYaadDetail(5)})
		return m, nil
	case "/ecosystem":
		settings, err := loadEffectiveSettings()
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		modelName, providerName := effectiveModelAndProvider(settings)
		if providerName == "" {
			providerName = "auto"
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: hawkconfig.FormatEcosystemPanel(context.Background(), providerName, modelName)})
		return m, nil
	case "/path":
		m.messages = append(m.messages, displayMsg{role: "system", content: hawkconfig.FormatDeveloperPathReport(context.Background())})
		return m, nil
	case "/config", "/con", "/conf":
		return m.handleConfigCommand(parts, text)
	case "/mcp":
		m.messages = append(m.messages, displayMsg{role: "system", content: m.mcpSummary()})
		return m, nil
	case "/power":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /power <1-10>\n" + DescribePower(5)})
			return m, nil
		}
		level, err := strconv.Atoi(parts[1])
		if err != nil || level < 1 || level > 10 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Power level must be 1-10."})
			return m, nil
		}
		ApplyPowerLevel(m.session, level)
		m.messages = append(m.messages, displayMsg{role: "system", content: DescribePower(level)})
		return m, nil
	case "/vibe":
		prompt := "Enter vibe coding mode. Auto-apply all changes, run tests after each edit, and iterate until tests pass. Start by reading the project structure."
		if len(parts) > 1 {
			prompt = strings.TrimSpace(strings.TrimPrefix(text, "/vibe"))
		}
		return m.startPromptCommand("/vibe", prompt)
	case "/research":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /research [--grep <pattern>] [--direction lower|higher] [--budget <min>] [--branch <prefix>] [--results <file>] <metric-command>\nExample: /research go test -bench .\nExample: /research --grep '^val_bpb:' --direction lower uv run train.py"})
			return m, nil
		}
		args := strings.TrimSpace(strings.TrimPrefix(text, "/research"))
		cfg := parseResearchArgs(args)
		if cfg.MetricCmd == "" {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Metric command is required."})
			return m, nil
		}
		prompt := BuildResearchPrompt(cfg)
		return m.startPromptCommand("/research", prompt)
	case "/parallel":
		return m.handleParallelCommand(parts, text)
	case "/usage":
		m.messages = append(m.messages, displayMsg{role: "system", content: m.session.Cost.Summary()})
		return m, nil
	case "/tools":
		m.messages = append(m.messages, displayMsg{role: "system", content: toolListSummary(m.registry)})
		return m, nil
	case "/skills":
		return m.handleSkillsCommand(parts, text)
	case "/learn":
		cwd, _ := os.Getwd()
		deep := len(parts) >= 2 && parts[1] == "deep"
		update := len(parts) >= 2 && parts[1] == "update"
		ctx := plugin.GatherLearnContext(cwd)
		if deep || update {
			ctx.SourceInfo = plugin.GatherDeepSourceInfo(cwd)
		}
		if update {
			summary := plugin.FormatLearnSummary(ctx, true)
			prompt := plugin.BuildLearnUpdatePrompt(ctx)
			return m.startPromptCommand(summary, prompt)
		}
		summary := plugin.FormatLearnSummary(ctx, deep)
		prompt := plugin.BuildLearnPrompt(ctx)
		return m.startPromptCommand(summary, prompt)
	case "/welcome":
		m.messages = append(m.messages, displayMsg{role: "welcome", content: m.welcomeCache})
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
	case "/agents":
		return m.startPromptCommand("/agents", "List all active agents and teammates in the current session. Show their status and assigned tasks.")
	case "/copy":
		return m.handleCopyCommand(parts)
	case "/select":
		// Pause the TUI so the user can use their terminal's native
		// text selection. Same as Ctrl+\ — see enterSelectionMode.
		return m, enterSelectionMode(m.ref, m.copyableTranscript(), m.mouseEnabled())
	case "/mouse":
		m.handleMouseCommand(parts)
		return m, nil
	case "/undo":
		restored, err := tool.UndoLatest()
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No file changes to undo"})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Restored %s", restored)})
		}
		return m, nil
	case "/theme":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /theme <dark|light|auto>"})
			return m, nil
		}
		if err := hawkconfig.SetGlobalSetting("theme", parts[1]); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Theme set to: %s (restart to apply)", parts[1])})
		}
		return m, nil
	case "/color":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /color <hex-color>"})
			return m, nil
		}
		if err := hawkconfig.SetGlobalSetting("agentColor", parts[1]); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Agent color set to: %s", parts[1])})
		}
		return m, nil
	case "/fast":
		savedModel := hawkconfig.ActiveModel(context.Background())
		if m.session.Model() == savedModel {
			norm := hawkconfig.NormalizeProviderForEngine(m.session.Provider())
			fastModel := hawkconfig.CheapestModelForProvider(norm, m.session.Model())
			if strings.TrimSpace(fastModel) == "" {
				fastModel = hawkconfig.DefaultModelForProvider(norm)
			}
			if strings.TrimSpace(fastModel) == "" {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Fast mode: no catalog model resolved for this provider"})
				return m, nil
			}
			m.session.SetModel(fastModel)
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Fast mode on → %s", fastModel)})
		} else {
			m.session.SetModel(savedModel)
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Fast mode off → %s", savedModel)})
		}
		return m, nil
	case "/effort":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /effort <low|medium|high>"})
			return m, nil
		}
		level := strings.ToLower(parts[1])
		switch level {
		case "low", "medium", "high":
			_ = hawkconfig.SetGlobalSetting("reasoningEffort", level)
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Reasoning effort → %s", level)})
		default:
			m.messages = append(m.messages, displayMsg{role: "error", content: "Valid levels: low, medium, high"})
		}
		return m, nil
	case "/glm":
		if len(parts) < 2 {
			cur, _ := hawkconfig.SettingValue(hawkconfig.LoadSettings(), "glmthinking")
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /glm <on|off|default> — toggle GLM/Z.ai extended reasoning\nCurrent: " + cur})
			return m, nil
		}
		switch strings.ToLower(parts[1]) {
		case "on":
			_ = hawkconfig.SetGlobalSetting("glmthinking", "true")
			enabled := true
			m.session.GLMThinkingEnabled = &enabled
			m.messages = append(m.messages, displayMsg{role: "system", content: "GLM thinking → enabled"})
		case "off":
			_ = hawkconfig.SetGlobalSetting("glmthinking", "false")
			disabled := false
			m.session.GLMThinkingEnabled = &disabled
			m.messages = append(m.messages, displayMsg{role: "system", content: "GLM thinking → disabled"})
		case "default":
			_ = hawkconfig.SetGlobalSetting("glmthinking", "default")
			m.session.GLMThinkingEnabled = nil
			m.messages = append(m.messages, displayMsg{role: "system", content: "GLM thinking → default (model decides)"})
		default:
			m.messages = append(m.messages, displayMsg{role: "error", content: "Valid options: on, off, default"})
		}
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
	case "/explain":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /explain <file>:<line>  — trace code back to the commit that created it"})
			return m, nil
		}
		arg := parts[1]
		path := arg
		line := 1
		if idx := strings.LastIndex(arg, ":"); idx > 0 {
			path = arg[:idx]
			if n, err := strconv.Atoi(arg[idx+1:]); err == nil {
				line = n
			}
		}
		result, err := explainCode(path, line)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.messages = append(m.messages, displayMsg{role: "assistant", content: result})
		}
		return m, nil

	case "/export":
		return m.handleSessionCommand(cmd, parts, text)
	case "/feedback":
		body := strings.TrimSpace(strings.TrimPrefix(text, "/feedback"))
		if body == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /feedback <message>\nCaptures session context and saves feedback to ~/.hawk/feedback/"})
			return m, nil
		}
		home := home.Dir()
		feedDir := filepath.Join(home, ".hawk", "feedback")
		_ = os.MkdirAll(feedDir, 0o755)
		report := fmt.Sprintf(`{"timestamp":%q,"version":%q,"model":%q,"provider":%q,"category":"session","body":%q,"session_id":%q}`,
			time.Now().Format(time.RFC3339), version, m.session.Model(), m.session.Provider(), body, m.sessionID)
		fname := fmt.Sprintf("feedback-%s.json", time.Now().Format("20060102-150405"))
		fpath := filepath.Join(feedDir, fname)
		if err := os.WriteFile(fpath, []byte(report), 0o644); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Failed to save feedback: " + err.Error()})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Feedback saved to %s", fpath)})
		}
		return m, nil
	case "/stale":
		if m.stalenessDetector == nil {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No staleness data available yet. Rules will be tracked as they are used."})
			return m, nil
		}
		threshold := 7 * 24 * time.Hour // 7 days default
		if len(parts) > 1 {
			if d, err := time.ParseDuration(parts[1]); err == nil {
				threshold = d
			}
		}
		staleRules := m.stalenessDetector.CheckStaleness(threshold)
		if len(staleRules) == 0 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No stale rules detected. All rules have been used within the threshold."})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: stalenessFormatReport(staleRules)})
		}
		return m, nil
	case "/taste":
		store, err := tasteStoreForSession()
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Taste store error: " + err.Error()})
			return m, nil
		}
		cwd, _ := os.Getwd()
		projectID := filepath.Base(cwd)
		profile, err := store.Load(projectID)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Load taste profile: " + err.Error()})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: profile.Summary()})
		return m, nil
	case "/rename":
		return m.handleSessionCommand(cmd, parts, text)
	case "/tag":
		return m.handleSessionCommand(cmd, parts, text)
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
	case "/image":
		return m.handleImageCommand(parts, text)
	case "/voice":
		out, err := exec.CommandContext(context.Background(), "which", "whisper").CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Voice requires whisper.cpp. Install with: brew install whisper-cpp"})
		} else {
			// Record audio and transcribe
			m.messages = append(m.messages, displayMsg{role: "system", content: "Recording audio... (press Enter to stop)"})
			go func() {
				// Create temp file for recording
				tmpFile := filepath.Join(os.TempDir(), "hawk_voice_input.wav")

				// Record using sox or ffmpeg if available
				var recordCmd *exec.Cmd
				if _, err := exec.LookPath("sox"); err == nil {
					recordCmd = exec.Command("sox", "-d", tmpFile, "trim", "0", "10")
				} else if _, err := exec.LookPath("ffmpeg"); err == nil {
					recordCmd = exec.Command("ffmpeg", "-y", "-f", "avfoundation", "-i", ":0", "-t", "10", tmpFile)
				} else {
					// Fallback: tell user to record manually
					m.messages = append(m.messages, displayMsg{role: "system", content: "No audio recorder found. Install sox (brew install sox) or use: whisper --model base -f recording.wav"})
					return
				}

				if err := recordCmd.Run(); err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Recording failed: %v", err)})
					return
				}

				// Transcribe with whisper
				transcribeCmd := exec.Command("whisper", "--model", "base", "--output_format", "txt", "--output_dir", os.TempDir(), tmpFile)
				if err := transcribeCmd.Run(); err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Transcription failed: %v", err)})
					return
				}

				// Read transcription
				txtFile := strings.TrimSuffix(tmpFile, ".wav") + ".txt"
				transcription, err := os.ReadFile(txtFile)
				if err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Could not read transcription"})
					return
				}

				transcript := strings.TrimSpace(string(transcription))
				if transcript != "" {
					m.input.SetValue(transcript)
					m.input.CursorEnd()
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Voice input: %s", transcript)})
				}
			}()
		}
		return m, nil
	case "/share":
		return m.handleSessionCommand(cmd, parts, text)
	case "/permissions":
		next, cmd := m.handlePermissionsCommand(parts)
		return next, cmd
	case "/upgrade":
		return m.startPromptCommand("/upgrade", "Check for hawk updates and show the latest available version.")
	case "/keybindings":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Keybindings:\n  Enter           — Submit\n  Ctrl+C          — Cancel/Exit\n  Ctrl+Shift+C    — Copy (input draft or chat)\n  Ctrl+\\          — Native text selection\n  Ctrl+L          — Clear\n  Up/Down         — History\n  Tab             — Complete\n  /mouse off      — Enable click-drag copy"})
		return m, nil
	case "/output-style":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /output-style <concise|normal|detailed>"})
			return m, nil
		}
		style := strings.ToLower(parts[1])
		switch style {
		case "concise", "normal", "detailed":
			_ = hawkconfig.SetGlobalSetting("outputStyle", style)
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Output style → %s", style)})
		default:
			m.messages = append(m.messages, displayMsg{role: "error", content: "Valid styles: concise, normal, detailed"})
		}
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
		report, err := hawkconfig.DeploymentStatusReport(context.Background(), m.session.Model())
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Provider status failed: %v", err)})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: report})
		return m, nil
	case "/session":
		return m.handleSessionCommand(cmd, parts, text)
	case "/statusline":
		m.messages = append(m.messages, displayMsg{role: "system", content: statusLineSummary(m)})
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
		summary, err := hawkconfig.RefreshModelCatalogV1(context.Background())
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Model catalog refresh failed: %v", err)})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: summary})
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
	case "/ctx", "/ctx-viz":
		m.messages = append(m.messages, displayMsg{role: "system", content: formatSessionContextUsage(m)})
		m.viewDirty = true
		return m, nil
	case "/home":
		m.goHome()
		m.updateViewportContent()
		return m, nil
	case "/follow":
		m.streamFollow = !m.streamFollow
		if m.streamFollow {
			m.autoScroll = true
			m.viewport.GotoBottom()
		}
		state := "off"
		if m.streamFollow {
			state = "on"
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: "Stream follow: " + state + " (scroll up or Tab→scrollback freezes view during replies)"})
		m.viewDirty = true
		return m, nil
	case "/rewind":
		return m.handleSessionCommand(cmd, parts, text)
	case "/loop":
		if len(parts) < 3 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /loop <interval> <command> (e.g., /loop 5m /doctor)"})
			return m, nil
		}
		interval, err := time.ParseDuration(parts[1])
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Invalid interval %q: %v", parts[1], err)})
			return m, nil
		}
		loopCmd := strings.Join(parts[2:], " ")
		// Cancel any previous loop before starting a new one.
		if m.loopCancel != nil {
			m.loopCancel()
		}
		loopCtx, loopCancel := context.WithCancel(context.Background())
		m.loopCancel = loopCancel
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Loop started: %s every %s (stop with /clear)", loopCmd, interval)})
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-loopCtx.Done():
					return
				case <-ticker.C:
					m.ref.Send(loopTickMsg{command: loopCmd})
				}
			}
		}()
		return m, nil
	case "/fork":
		return m.handleSessionCommand(cmd, parts, text)
	case "/search":
		return m.handleSessionCommand(cmd, parts, text)
	case "/clean":
		return m.handleSessionCommand(cmd, parts, text)
	case "/audit":
		m.messages = append(m.messages, displayMsg{role: "system", content: tool.FormatAuditSummary()})
		return m, nil
	case "/compress":
		return m.handleSessionCommand(cmd, parts, text)
	case "/integrity":
		return m.handleSessionCommand(cmd, parts, text)
	case "/retry":
		return m.handleSessionCommand(cmd, parts, text)
	case "/add":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /add <file-path> [file-path...]"})
			return m, nil
		}
		var added []string
		for _, f := range parts[1:] {
			content, err := os.ReadFile(f)
			if err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Cannot read %s: %v", f, err)})
				continue
			}
			m.session.AddUser(fmt.Sprintf("[File: %s]\n```\n%s\n```", f, string(content)))
			added = append(added, f)
		}
		if len(added) > 0 {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Added to context: %s", strings.Join(added, ", "))})
		}
		return m, nil

	case "/drop":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /drop <file-path>"})
			return m, nil
		}
		file := parts[1]
		m.session.AddUser(fmt.Sprintf("[System: The file %s has been removed from context. Disregard any previous content from this file.]", file))
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Dropped %s from context.", file)})
		return m, nil

	case "/run":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /run <command>"})
			return m, nil
		}
		cmdStr := strings.TrimSpace(strings.TrimPrefix(text, "/run"))
		if tool.IsDestructiveCommand(cmdStr) || tool.IsSuspicious(cmdStr) {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Blocked: command fails safety check"})
			return m, nil
		}
		out, err := exec.CommandContext(context.Background(), "sh", "-c", cmdStr).CombinedOutput()
		result := strings.TrimSpace(string(out))
		if err != nil {
			result += "\n" + err.Error()
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("$ %s\n%s", cmdStr, result)})
		m.session.AddUser(fmt.Sprintf("[Command output: %s]\n```\n%s\n```", cmdStr, result))
		return m, nil

	case "/test":
		cmdStr := "go test ./..."
		if len(parts) >= 2 {
			cmdStr = strings.TrimSpace(strings.TrimPrefix(text, "/test"))
		}
		if tool.IsDestructiveCommand(cmdStr) || tool.IsSuspicious(cmdStr) {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Blocked: command fails safety check"})
			return m, nil
		}
		out, err := exec.CommandContext(context.Background(), "sh", "-c", cmdStr).CombinedOutput()
		result := strings.TrimSpace(string(out))
		if err != nil {
			result += "\n" + err.Error()
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Tests failed:\n%s", result)})
			m.session.AddUser(fmt.Sprintf("[Test failures]\n```\n%s\n```\nPlease fix these test failures.", result))
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: "All tests passed."})
		}
		return m, nil

	case "/lint":
		cmdStr := "golangci-lint run ./..."
		if len(parts) >= 2 {
			cmdStr = strings.TrimSpace(strings.TrimPrefix(text, "/lint"))
		}
		if tool.IsDestructiveCommand(cmdStr) || tool.IsSuspicious(cmdStr) {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Blocked: command fails safety check"})
			return m, nil
		}
		out, _ := exec.CommandContext(context.Background(), "sh", "-c", cmdStr).CombinedOutput()
		result := strings.TrimSpace(string(out))
		if result == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No lint issues."})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Lint issues:\n%s", result)})
			m.session.AddUser(fmt.Sprintf("[Lint output]\n```\n%s\n```\nPlease fix these lint issues.", result))
		}
		return m, nil

	case "/tokens":
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Messages: %d\nEstimated tokens: ~%d", m.session.MessageCount(), m.session.MessageCount()*200)})
		return m, nil

	case "/new":
		return m.handleSessionCommand(cmd, parts, text)
	case "/btw":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /btw <message>"})
			return m, nil
		}
		note := strings.TrimSpace(strings.TrimPrefix(text, "/btw"))
		m.session.AddUser(fmt.Sprintf("[Background note — do not respond to this directly, just acknowledge and keep it in mind]\n%s", note))
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Noted: %s", note)})
		return m, nil

	default:
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

// handleParallelCommand spawns multiple agents in parallel on independent tasks.
// Usage: /parallel <N> <task1> | <task2> | ...
func (m *chatModel) handleParallelCommand(parts []string, text string) (tea.Model, tea.Cmd) {
	if len(parts) < 3 {
		m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /parallel <N> <task1> | <task2> | ...\nExample: /parallel 3 Fix auth bug | Add logging | Update tests"})
		return m, nil
	}

	// Parse worker count
	var workers int
	if _, err := fmt.Sscanf(parts[1], "%d", &workers); err != nil || workers < 1 || workers > 8 {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Worker count must be 1-8"})
		return m, nil
	}

	// Parse tasks (separated by |)
	taskStr := strings.Join(parts[2:], " ")
	taskDescs := strings.Split(taskStr, "|")
	for i := range taskDescs {
		taskDescs[i] = strings.TrimSpace(taskDescs[i])
	}
	if len(taskDescs) < 2 {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Need at least 2 tasks separated by |"})
		return m, nil
	}

	// Get repo root for worktree pool
	cwd, _ := os.Getwd()

	// Create grid UI
	grid := NewAgentGrid(taskDescs, m.width, m.height-10)
	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("🚀 Spawning %d parallel agents for %d tasks...", workers, len(taskDescs))})

	// Run parallel agents in background
	go func() {
		pool := parallel.NewPool(cwd, "main", workers)
		for _, desc := range taskDescs {
			pool.AddTask(desc)
		}

		// Update grid as agents run
		taskIdx := 0
		err := pool.Run(context.Background(), func(ctx context.Context, worktreePath string, task *parallel.Task) (string, error) {
			pane := grid.GetPane(fmt.Sprintf("%d", taskIdx+1))
			if pane != nil {
				pane.SetState(AgentRunning)
				pane.Append(fmt.Sprintf("Starting in worktree: %s", worktreePath))
				m.ref.Send(streamChunkMsg(grid.Render()))
			}
			taskIdx++

			// Create a new session for this agent with same provider/model
			agentSession := engine.NewSession(
				m.session.Provider(),
				m.session.Model(),
				"You are a coding agent working in an isolated git worktree. Complete the assigned task.",
				m.registry,
			)
			agentSession.AddUser(fmt.Sprintf("Working in isolated worktree: %s\nTask: %s", worktreePath, task.Description))

			// Stream the agent's work
			ch, err := agentSession.Stream(ctx)
			if err != nil {
				if pane != nil {
					pane.SetState(AgentFailed)
					pane.Append(fmt.Sprintf("Error: %v", err))
				}
				return "", err
			}

			var result strings.Builder
			for ev := range ch {
				switch ev.Type {
				case "content":
					result.WriteString(ev.Content)
					if pane != nil {
						pane.Append(ev.Content)
					}
				case "done":
					if pane != nil {
						pane.SetState(AgentDone)
						pane.Append("Task completed")
					}
					return result.String(), nil
				case "error":
					if pane != nil {
						pane.SetState(AgentFailed)
						pane.Append(fmt.Sprintf("Error: %s", ev.Content))
					}
					return result.String(), fmt.Errorf("%s", ev.Content)
				}
			}
			return result.String(), nil
		})

		// Send final grid state
		m.ref.Send(streamChunkMsg(grid.Render()))

		if err != nil {
			m.ref.Send(streamErrMsg{err: err})
		} else {
			m.ref.Send(streamDoneMsg{})
		}
	}()

	return m, nil
}

// handleRefactorCommand runs agent-driven refactoring on the codebase.
func (m *chatModel) handleRefactorCommand(parts []string, text string) (tea.Model, tea.Cmd) {
	// Default refactoring scope
	scope := "."
	if len(parts) > 1 {
		scope = parts[1]
	}

	prompt := fmt.Sprintf(`You are in REFACTOR mode. Perform agent-driven refactoring on the codebase.

## Scope
%s

## Tasks (execute in order)
1. **Dead code removal**: Find and remove unused functions, variables, imports
2. **Deduplication**: Identify and consolidate duplicate code patterns
3. **Lint fixes**: Run linter and fix all issues
4. **Import cleanup**: Remove unused imports, organize import groups
5. **Test coverage**: Add tests for untested critical paths

## Rules
- Make minimal, safe changes
- Run tests after each change to verify no regressions
- If a change is risky, skip it and note why
- Commit each category of change separately

## Output
After completing, provide a summary of:
- Files modified
- Lines removed/added
- Issues fixed
- Any skipped changes and why`, scope)

	return m.startPromptCommand("/refactor", prompt)
}
