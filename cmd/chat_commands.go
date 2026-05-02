package cmd

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/hawk/analytics"
	hawkconfig "github.com/GrayCodeAI/hawk/config"
	"github.com/GrayCodeAI/hawk/engine"
	"github.com/GrayCodeAI/hawk/plugin"
	"github.com/GrayCodeAI/hawk/session"
	"github.com/GrayCodeAI/hawk/tool"
)

func slashCommands() []string {
	return []string{
		"/add", "/add-dir", "/agents", "/agents-init", "/audit", "/branch", "/bughunter", "/clean", "/clear",
		"/color", "/commit", "/compact", "/compress", "/config", "/context", "/council",
		"/copy", "/cost", "/cron", "/diff", "/doctor", "/drop", "/effort", "/env", "/exit", "/explain",
		"/export", "/fast", "/files", "/focus", "/fork", "/help", "/history", "/hooks", "/init",
		"/integrity", "/keybindings", "/learn", "/lint", "/loop", "/mcp", "/memory", "/metrics", "/model", "/new",
		"/output-style", "/permissions", "/pin", "/plan", "/plugin", "/plugins",
		"/power", "/pr-comments", "/provider-status", "/quit", "/refresh-model-catalog", "/release-notes",
		"/reload-plugins", "/remote-env", "/rename", "/render", "/research", "/resume", "/retry", "/review", "/rewind",
		"/run", "/btw", "/sandbox", "/search", "/security-review", "/session", "/share", "/skills", "/stats",
		"/status", "/statusline", "/summary", "/tag", "/tasks", "/test", "/theme",
		"/think-back", "/thinkback", "/thinkback-play", "/tokens", "/tools", "/undo", "/upgrade", "/usage",
		"/version", "/vibe", "/vim", "/voice", "/welcome", "/yolo",
	}
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
	"/clean":           "Delete old sessions",
	"/clear":           "Clear conversation",
	"/color":           "Change agent color",
	"/commit":          "Auto-commit changes with AI message",
	"/compact":         "Compress conversation to save tokens",
	"/compress":        "Compress old sessions",
	"/config":          "Open settings panel",
	"/context":         "Show current context",
	"/copy":            "Copy last response to clipboard",
	"/cost":            "Show token usage and cost",
	"/council":         "Run LLM Council (multi-model consensus)",
	"/diff":            "Show git diff (preview changes)",
	"/doctor":          "Run diagnostics (build, test, lint)",
	"/drop":            "Remove file from context",
	"/effort":          "Set reasoning effort level",
	"/env":             "Show environment info",
	"/exit":            "Save and exit",
	"/explain":         "Trace code back to the commit that created it",
	"/export":          "Export session",
	"/fast":            "Toggle fast mode",
	"/files":           "Show modified files",
	"/focus":           "Narrow agent attention to specific files/dirs",
	"/fork":            "Fork session at a point",
	"/help":            "Show all commands",
	"/history":         "List saved sessions",
	"/hooks":           "Show configured hooks",
	"/init":            "Analyze project structure",
	"/integrity":       "Validate session integrity",
	"/lint":            "Run linter, add issues to context",
	"/loop":            "Schedule recurring command",
	"/mcp":             "Show MCP server status",
	"/memory":          "Show project instructions",
	"/metrics":         "Show session metrics",
	"/model":           "Switch or view current model",
	"/new":             "Start a fresh session",
	"/permissions":     "Manage permission rules",
	"/pin":             "Pin last N messages to protect from compaction",
	"/plan":            "Enter plan mode (read-only)",
	"/plugins":         "List installed plugins",
	"/power":           "Set power level (1-10)",
	"/quit":            "Save and exit",
	"/resume":          "Resume a saved session",
	"/retry":           "Redo last message",
	"/review":          "Code review for bugs and issues",
	"/rewind":          "Undo last exchange",
	"/run":             "Run command, add output to context",
	"/sandbox":         "Toggle sandbox mode",
	"/search":          "Search across sessions",
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
	"/yolo":            "Toggle auto-approve mode",
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
	"/theme":           "Change visual theme",
	"/think-back":      "Review reasoning decisions",
	"/thinkback":       "Review reasoning decisions",
	"/thinkback-play":  "Replay reasoning path",
	"/upgrade":         "Check for updates",
	"/vibe":            "Start vibe coding loop",
	"/voice":           "Toggle voice input",
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
	if md := hawkconfig.LoadAgentsMDFrom(abs); md != "" {
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
/learn              — LLM-powered skill advisor (deep, update)
/skills             — List, search, install, remove skills
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
	case "/model":
		if len(parts) == 1 {
			m.configModels = nil
			m.configOpen = true
			m.configMenu = "model"
			m.configSel = 0
			m.configScroll = 0
			m.configNotice = ""
			provider := m.session.Provider()
			cmd := func() tea.Msg {
				models, _ := hawkconfig.FetchModelsForProvider(provider)
				return modelsFetchedMsg(extractModelIDs(models))
			}
			return m, cmd
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
	case "/version":
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("hawk %s", version)})
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
		if err := os.WriteFile("AGENTS.md", []byte(content), 0644); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Failed to write AGENTS.md: " + err.Error()})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Created AGENTS.md (detected: %s). Edit it to match your project.", pt)})
		return m, nil
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
		md := strings.TrimSpace(hawkconfig.LoadAgentsMD())
		if md == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No AGENTS.md or .hawk/AGENTS.md project instructions found."})
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
		if len(parts) >= 2 {
			switch parts[1] {
			case "install":
				if len(parts) < 3 {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills install <owner/repo> [skill-name]"})
					return m, nil
				}
				repo := parts[2]
				if !strings.Contains(repo, "/") {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills install <owner/repo> [skill-name]"})
					return m, nil
				}
				skillName := ""
				if len(parts) >= 4 {
					skillName = parts[3]
				}
				rc := plugin.NewRegistryClient()
				msg, err := rc.Install(repo, skillName, "user")
				if err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				} else {
					m.messages = append(m.messages, displayMsg{role: "system", content: msg})
				}
				return m, nil

			case "search":
				query := ""
				category := ""
				for i := 2; i < len(parts); i++ {
					if parts[i] == "--category" && i+1 < len(parts) {
						category = parts[i+1]
						i++
					} else {
						if query != "" {
							query += " "
						}
						query += parts[i]
					}
				}
				rc := plugin.NewRegistryClient()
				results, err := rc.Search(query, category)
				if err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
					return m, nil
				}
				if len(results) == 0 {
					m.messages = append(m.messages, displayMsg{role: "system", content: "No skills found."})
					return m, nil
				}
				var b strings.Builder
				b.WriteString(fmt.Sprintf("Found %d skill(s):\n\n", len(results)))
				limit := 20
				if len(results) < limit {
					limit = len(results)
				}
				for _, e := range results[:limit] {
					b.WriteString(plugin.FormatSkillEntry(e))
				}
				if len(results) > 20 {
					fmt.Fprintf(&b, "\n  ... and %d more. Refine your search.\n", len(results)-20)
				}
				m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
				return m, nil

			case "trending":
				limit := 10
				if len(parts) >= 3 {
					if n, err := strconv.Atoi(parts[2]); err == nil && n > 0 {
						limit = n
					}
				}
				rc := plugin.NewRegistryClient()
				results, err := rc.Trending(limit)
				if err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
					return m, nil
				}
				if len(results) == 0 {
					m.messages = append(m.messages, displayMsg{role: "system", content: "No trending skills found."})
					return m, nil
				}
				var b strings.Builder
				b.WriteString("Trending skills:\n\n")
				for i, e := range results {
					fmt.Fprintf(&b, "  %d. ", i+1)
					b.WriteString(strings.TrimLeft(plugin.FormatSkillEntry(e), " "))
				}
				m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
				return m, nil

			case "info":
				if len(parts) < 3 {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills info <name>"})
					return m, nil
				}
				name := parts[2]
				// Check local first.
				if skill, path, ok := plugin.InstalledSkillInfo(name); ok {
					m.messages = append(m.messages, displayMsg{role: "system", content: plugin.FormatSkillInfo(skill, path)})
					return m, nil
				}
				// Fall back to registry.
				rc := plugin.NewRegistryClient()
				entry, err := rc.Info(name)
				if err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
					return m, nil
				}
				var b strings.Builder
				fmt.Fprintf(&b, "Skill: %s (not installed)\n", entry.Name)
				if entry.Version != "" {
					fmt.Fprintf(&b, "Version: %s\n", entry.Version)
				}
				if entry.Author != "" {
					fmt.Fprintf(&b, "Author: %s\n", entry.Author)
				}
				if entry.Description != "" {
					fmt.Fprintf(&b, "Description: %s\n", entry.Description)
				}
				if entry.Repo != "" {
					fmt.Fprintf(&b, "Repo: %s\n", entry.Repo)
				}
				fmt.Fprintf(&b, "Installs: %d\n", entry.Installs)
				fmt.Fprintf(&b, "\nInstall with: /skills install %s %s\n", entry.Repo, entry.Name)
				m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
				return m, nil

			case "remove":
				if len(parts) < 3 {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills remove <name>"})
					return m, nil
				}
				if err := plugin.Remove(parts[2]); err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				} else {
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Removed skill %q.", parts[2])})
				}
				return m, nil

			case "update":
				name := ""
				if len(parts) >= 3 {
					name = parts[2]
				}
				// Find installed skills with source metadata and re-install.
				updated := 0
				skills := plugin.LoadSmartSkills(plugin.DefaultSkillDirs())
				for _, s := range skills {
					if s.Source.Repo == "" {
						continue
					}
					if name != "" && !strings.EqualFold(s.Name, name) {
						continue
					}
					rc := plugin.NewRegistryClient()
					if _, err := rc.Install(s.Source.Repo, s.Name, "user"); err == nil {
						updated++
					}
				}
				if updated == 0 {
					m.messages = append(m.messages, displayMsg{role: "system", content: "No skills to update (only skills with source tracking can be updated)."})
				} else {
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Updated %d skill(s).", updated)})
				}
				return m, nil

			case "publish":
				if len(parts) < 3 {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills publish <skill-dir>\nValidates the skill and shows the command to submit it."})
					return m, nil
				}
				skillDir := parts[2]
				skillFile := filepath.Join(skillDir, "SKILL.md")
				if _, err := os.Stat(skillFile); err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("No SKILL.md found in %s", skillDir)})
					return m, nil
				}
				findings, _ := plugin.AuditSkillFile(skillFile)
				for _, f := range findings {
					if f.Severity == plugin.SeverityCritical {
						m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Publish blocked: %s has CRITICAL security findings. Run /skills audit first.", skillFile)})
						return m, nil
					}
				}
				data, _ := os.ReadFile(skillFile)
				skill := plugin.ParseSmartSkillPublic(string(data))
				var issues []string
				if skill.Name == "" {
					issues = append(issues, "missing 'name' in frontmatter")
				}
				if skill.Description == "" {
					issues = append(issues, "missing 'description' in frontmatter")
				}
				if len(issues) > 0 {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Validation failed:\n  - " + strings.Join(issues, "\n  - ")})
					return m, nil
				}
				var b strings.Builder
				b.WriteString("✓ Skill validated successfully.\n\n")
				fmt.Fprintf(&b, "  Name: %s\n", skill.Name)
				fmt.Fprintf(&b, "  Description: %s\n", skill.Description)
				if skill.Version != "" {
					fmt.Fprintf(&b, "  Version: %s\n", skill.Version)
				}
				b.WriteString("\nTo publish:\n")
				b.WriteString("  1. Push your skill to a GitHub repo with skills/<name>/SKILL.md\n")
				b.WriteString("  2. Submit a PR to github.com/GrayCodeAI/hawk-skills to add your repo\n")
				b.WriteString("  3. Or install directly: /skills install <your-org>/<your-repo>\n")
				m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
				return m, nil

			case "audit":
				if len(parts) >= 3 {
					target := parts[2]
					if info, err := os.Stat(target); err == nil && !info.IsDir() {
						findings, err := plugin.AuditSkillFile(target)
						if err != nil {
							m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
							return m, nil
						}
						r := plugin.AuditResult{Findings: findings, Files: 1}
						m.messages = append(m.messages, displayMsg{role: "system", content: plugin.FormatAuditResult(r)})
						return m, nil
					}
					if _, path, ok := plugin.InstalledSkillInfo(target); ok {
						findings, _ := plugin.AuditSkillFile(path)
						r := plugin.AuditResult{Findings: findings, Files: 1}
						m.messages = append(m.messages, displayMsg{role: "system", content: plugin.FormatAuditResult(r)})
						return m, nil
					}
					m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Skill or file %q not found.", target)})
					return m, nil
				}
				result := plugin.AuditAllSkills()
				m.messages = append(m.messages, displayMsg{role: "system", content: plugin.FormatAuditResult(result)})
				return m, nil

			case "feedback":
				if len(parts) < 4 {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills feedback <name> <1-5> [comment]"})
					return m, nil
				}
				name := parts[2]
				rating, err := strconv.Atoi(parts[3])
				if err != nil || rating < 1 || rating > 5 {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Rating must be 1-5."})
					return m, nil
				}
				comment := ""
				if len(parts) > 4 {
					comment = strings.Join(parts[4:], " ")
				}
				fs := plugin.NewFeedbackStore()
				if err := fs.Rate(name, rating, comment); err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				} else {
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Rated %s %s", name, plugin.FormatRating(rating))})
				}
				return m, nil

			case "use":
				if len(parts) < 3 {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills use <name>"})
					return m, nil
				}
				name := parts[2]
				skills := plugin.LoadSmartSkills(plugin.DefaultSkillDirs())
				for _, s := range skills {
					if strings.EqualFold(s.Name, name) {
						if m.activeSkills == nil {
							m.activeSkills = make(map[string]plugin.SmartSkill)
						}
						m.activeSkills[s.Name] = s
						m.session.AddUser(fmt.Sprintf("[Skill activated: %s]\n\n%s", s.Name, s.Content))
						m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Activated skill: %s", s.Name)})
						return m, nil
					}
				}
				m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Skill %q not found. Run /skills to see available skills.", name)})
				return m, nil

			case "deactivate":
				if len(parts) < 3 {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills deactivate <name>"})
					return m, nil
				}
				name := parts[2]
				if m.activeSkills != nil {
					if _, ok := m.activeSkills[name]; ok {
						delete(m.activeSkills, name)
						m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Deactivated skill: %s", name)})
						return m, nil
					}
				}
				m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Skill %q is not active.", name)})
				return m, nil

			case "new":
				desc := "a useful coding skill for this project"
				if len(parts) >= 3 {
					desc = strings.Join(parts[2:], " ")
				}
				prompt := plugin.BuildNewSkillPrompt(desc)
				return m.startPromptCommand("/skills new "+desc, prompt)
			}
		}
		// Default: list local skills.
		out, err := (tool.SkillTool{}).Execute(context.Background(), nil)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: out})
		}
		return m, nil
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
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].role == "assistant" {
				if err := copyToClipboard(m.messages[i].content); err != nil {
					m.messages = append(m.messages, displayMsg{role: "error", content: "Failed to copy: " + err.Error()})
				} else {
					m.messages = append(m.messages, displayMsg{role: "system", content: "Copied to clipboard."})
				}
				return m, nil
			}
		}
		m.messages = append(m.messages, displayMsg{role: "error", content: "No assistant response to copy."})
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
		if m.session.Model() == m.settings.Model {
			fastModel := "claude-haiku-4-5-20251001"
			m.session.SetModel(fastModel)
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Fast mode on → %s", fastModel)})
		} else {
			m.session.SetModel(m.settings.Model)
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Fast mode off → %s", m.settings.Model)})
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
		home, _ := os.UserHomeDir()
		exportDir := filepath.Join(home, ".hawk", "exports")
		os.MkdirAll(exportDir, 0755)
		exportPath := filepath.Join(exportDir, m.sessionID+".md")
		var md strings.Builder
		md.WriteString(fmt.Sprintf("# Session %s\n\n", m.sessionID))
		for _, msg := range m.messages {
			switch msg.role {
			case "user":
				md.WriteString("## User\n" + msg.content + "\n\n")
			case "assistant":
				md.WriteString("## Assistant\n" + msg.content + "\n\n")
			case "system":
				md.WriteString("_" + msg.content + "_\n\n")
			}
		}
		if err := os.WriteFile(exportPath, []byte(md.String()), 0644); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Exported to: %s", exportPath)})
		}
		return m, nil
	case "/rename":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /rename <new-session-name>"})
			return m, nil
		}
		newName := parts[1]
		home, _ := os.UserHomeDir()
		sessDir := filepath.Join(home, ".hawk", "sessions")
		oldPath := filepath.Join(sessDir, m.sessionID+".jsonl")
		newPath := filepath.Join(sessDir, newName+".jsonl")
		if err := os.Rename(oldPath, newPath); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.sessionID = newName
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Session renamed to: %s", newName)})
		}
		return m, nil
	case "/tag":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /tag <label>"})
			return m, nil
		}
		home, _ := os.UserHomeDir()
		tagFile := filepath.Join(home, ".hawk", "sessions", m.sessionID+".tags")
		f, err := os.OpenFile(tagFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			f.WriteString(parts[1] + "\n")
			f.Close()
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Tagged: %s", parts[1])})
		}
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
		out, err := exec.Command("which", "whisper").CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Voice requires whisper.cpp. Install with: brew install whisper-cpp"})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Recording... (press Enter when done, Ctrl+C to cancel)\nNote: voice input requires a separate terminal. Use: whisper --model base -f recording.wav | hawk"})
		}
		return m, nil
	case "/share":
		home, _ := os.UserHomeDir()
		exportDir := filepath.Join(home, ".hawk", "exports")
		os.MkdirAll(exportDir, 0755)
		exportPath := filepath.Join(exportDir, m.sessionID+".md")
		var md strings.Builder
		md.WriteString(fmt.Sprintf("# Hawk Session %s\n\n", m.sessionID))
		md.WriteString(fmt.Sprintf("Model: %s/%s\n\n---\n\n", m.session.Provider(), m.session.Model()))
		for _, msg := range m.messages {
			switch msg.role {
			case "user":
				md.WriteString("**User:** " + msg.content + "\n\n")
			case "assistant":
				md.WriteString("**Hawk:** " + msg.content + "\n\n")
			}
		}
		if err := os.WriteFile(exportPath, []byte(md.String()), 0644); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Session saved to: %s\nShare this file or paste its contents.", exportPath)})
		}
		return m, nil
	case "/upgrade":
		return m.startPromptCommand("/upgrade", "Check for hawk updates and show the latest available version.")
	case "/keybindings":
		m.messages = append(m.messages, displayMsg{role: "system", content: "Keybindings:\n  Enter       — Submit\n  Ctrl+C      — Cancel/Exit\n  Ctrl+L      — Clear\n  Up/Down     — History\n  Tab         — Complete"})
		return m, nil
	case "/sandbox":
		if string(m.session.Mode) == "acceptEdits" {
			m.session.SetPermissionMode("default")
			m.messages = append(m.messages, displayMsg{role: "system", content: "Sandbox ON — all actions require approval."})
		} else {
			m.session.SetPermissionMode("acceptEdits")
			m.messages = append(m.messages, displayMsg{role: "system", content: "Sandbox OFF — file edits auto-approved, other actions require approval."})
		}
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
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Loop started: %s every %s (stop with /clear)", loopCmd, interval)})
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for range ticker.C {
				m.ref.Send(loopTickMsg{command: loopCmd})
			}
		}()
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
	case "/retry":
		if len(m.history) > 0 {
			last := m.history[len(m.history)-1]
			if m.session.MessageCount() > 2 {
				m.session.RemoveLastExchange()
				if len(m.messages) >= 2 {
					m.messages = m.messages[:len(m.messages)-2]
				}
			}
			m.messages = append(m.messages, displayMsg{role: "user", content: last})
			m.session.AddUser(last)
			m.waiting = true
			m.autoScroll = true
			m.spinnerVerb = spinnerVerbs[rand.Intn(len(spinnerVerbs))]
			m.startStream()
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "error", content: "No previous message to retry."})
		return m, nil

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
		out, err := exec.Command("sh", "-c", cmdStr).CombinedOutput()
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
		out, err := exec.Command("sh", "-c", cmdStr).CombinedOutput()
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
		out, _ := exec.Command("sh", "-c", cmdStr).CombinedOutput()
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

	case "/yolo":
		if string(m.session.Mode) == "bypassPermissions" {
			m.session.SetPermissionMode("default")
			m.messages = append(m.messages, displayMsg{role: "system", content: "Yolo mode OFF — all actions require approval."})
		} else {
			m.session.SetPermissionMode("bypassPermissions")
			m.messages = append(m.messages, displayMsg{role: "system", content: "⚠ Yolo mode ON — all tool calls auto-approved."})
		}
		return m, nil

	case "/new":
		m.saveSession()
		m.messages = []displayMsg{{role: "welcome", content: m.welcomeCache}}
		m.session.LoadMessages(nil)
		sid := genID()
		m.sessionID = sid
		if wal, err := session.NewWAL(sid); err == nil {
			m.wal = wal
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: "New session started."})
		return m, nil

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

// explainCode traces a file/line back to the git commit and session that created it.
func explainCode(path string, line int) (string, error) {
	// Step 1: git blame to find the commit
	args := []string{"blame", "-L", fmt.Sprintf("%d,%d", line, line), "--porcelain", path}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git blame failed: %w", err)
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("no blame output")
	}
	commitHash := strings.Fields(lines[0])[0]
	if commitHash == "0000000000000000000000000000000000000000" {
		return "This line is uncommitted (not yet in git history).", nil
	}

	// Step 2: get commit info
	info, err := exec.Command("git", "log", "-1", "--format=%h %s (%an, %ar)", commitHash).Output()
	if err != nil {
		return fmt.Sprintf("Commit: %s (details unavailable)", commitHash[:7]), nil
	}

	// Step 3: get the diff for context
	diff, _ := exec.Command("git", "log", "-1", "--format=", "-p", "--", path, commitHash).Output()
	diffStr := string(diff)
	if len(diffStr) > 2000 {
		diffStr = diffStr[:2000] + "\n... (truncated)"
	}

	result := fmt.Sprintf("**Origin:** %s\n", strings.TrimSpace(string(info)))
	if diffStr != "" {
		result += fmt.Sprintf("\n**Changes in that commit:**\n```diff\n%s\n```", diffStr)
	}
	return result, nil
}
