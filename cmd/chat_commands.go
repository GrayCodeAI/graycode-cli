package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/multiagent/parallel"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
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

func (m chatModel) inputAreaLayoutKey() int {
	if m.configOpen {
		return 0
	}
	lines := strings.Count(m.input.Value(), "\n") + 1
	if lines > 10 {
		lines = 10
	}
	key := lines<<16 | m.visibleSlashSuggestionLines()
	if m.manualCompacting {
		key |= 1 << 15
	}
	if m.inScrollbackFocus() {
		key |= 1 << 14
	}
	if m.ghostText != nil {
		if ghost := m.ghostText.Get(); ghost != "" && m.input.Value() == "" {
			key |= 1 << 13
		}
	}
	return key
}

func (m *chatModel) invalidateInputLayoutCache() {
	m.layoutKey = -1
	m.cachedBottomBarLines = 0
}

func (m *chatModel) refreshInputLayoutIfNeeded() bool {
	if m.configOpen {
		return false
	}
	key := m.inputAreaLayoutKey()
	if key == m.layoutKey && m.cachedBottomBarLines > 0 {
		return false
	}
	m.layoutKey = key
	m.cachedBottomBarLines = m.computeChatBottomBarLines()
	return true
}

func (m *chatModel) syncInputLayout() bool {
	return m.refreshInputLayoutIfNeeded()
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

	// SubcommandRegistry dispatch. Migrations live in
	// chat_subcommand_<name>.go files. Each registers itself in
	// init(); we look up by the slash name minus the leading "/".
	// If the registry has a handler, dispatch and return.
	if strings.HasPrefix(cmd, "/") {
		name := strings.TrimPrefix(cmd, "/")
		if sub, ok := subcommandRegistry.Lookup(name); ok {
			args := parts[1:]
			return sub.Handle(m, args, text)
		}
	}

	switch cmd {
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
	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Spawning %d parallel agents for %d tasks...", icons.Bolt(), workers, len(taskDescs))})

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
