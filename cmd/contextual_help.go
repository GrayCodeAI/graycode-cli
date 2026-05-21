package cmd

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// HelpEntry represents a single help topic with its documentation.
type HelpEntry struct {
	Topic    string
	Summary  string
	Detail   string
	Examples []string
	Related  []string
	Category string
}

// ContextualHelp provides context-aware help and documentation lookup.
type ContextualHelp struct {
	Entries map[string]*HelpEntry
	mu      sync.RWMutex
}

// NewContextualHelp creates a new ContextualHelp instance populated with all built-in help entries.
func NewContextualHelp() *ContextualHelp {
	ch := &ContextualHelp{
		Entries: make(map[string]*HelpEntry),
	}
	ch.registerAllEntries()
	return ch
}

func (ch *ContextualHelp) registerAllEntries() {
	entries := []*HelpEntry{
		// ─── Slash Commands ───────────────────────────────────────────
		{
			Topic:    "/commit",
			Summary:  "Auto-commit with AI message",
			Detail:   "Creates a git commit with an AI-generated message based on staged changes.",
			Examples: []string{"/commit              — commit all staged changes", "/commit --amend      — amend the last commit", `/commit "msg"        — use custom message`},
			Related:  []string{"/diff", "/branch", "/undo"},
			Category: "slash-commands",
		},
		{
			Topic:    "/diff",
			Summary:  "Show current changes with AI summary",
			Detail:   "Displays a diff of uncommitted changes along with an AI-generated summary explaining what changed and why.",
			Examples: []string{"/diff                — show all uncommitted changes", "/diff --staged       — show only staged changes", "/diff file.go        — diff a specific file"},
			Related:  []string{"/commit", "/status", "/branch"},
			Category: "slash-commands",
		},
		{
			Topic:    "/test",
			Summary:  "Run tests with AI analysis",
			Detail:   "Runs your project test suite and provides AI-powered analysis of failures, including suggested fixes.",
			Examples: []string{"/test                — run all tests", "/test ./pkg/...      — run tests in a package", "/test -v             — verbose output"},
			Related:  []string{"/lint", "/bugfind", "/fix"},
			Category: "slash-commands",
		},
		{
			Topic:    "/lint",
			Summary:  "Lint code and suggest fixes",
			Detail:   "Runs configured linters on your codebase and provides AI-powered fix suggestions for any issues found.",
			Examples: []string{"/lint                — lint entire project", "/lint ./cmd/...      — lint specific package", "/lint --fix          — auto-fix where possible"},
			Related:  []string{"/test", "/fix", "/format"},
			Category: "slash-commands",
		},
		{
			Topic:    "/branch",
			Summary:  "Create or switch branches",
			Detail:   "Manages git branches with AI-suggested naming based on your current work context.",
			Examples: []string{"/branch              — list branches", "/branch feat/login   — create and switch to branch", "/branch -d old-feat  — delete a branch"},
			Related:  []string{"/commit", "/diff", "/merge"},
			Category: "slash-commands",
		},
		{
			Topic:    "/undo",
			Summary:  "Undo last action safely",
			Detail:   "Reverts the last hawk action (commit, file edit, etc.) using safe snapshot-based rollback.",
			Examples: []string{"/undo                — undo last action", "/undo --hard         — undo and discard changes", "/undo 3              — undo last 3 actions"},
			Related:  []string{"/commit", "/snapshot", "/history"},
			Category: "slash-commands",
		},
		{
			Topic:    "/status",
			Summary:  "Show project and session status",
			Detail:   "Displays current git status, session info, active tasks, and recent activity in a unified view.",
			Examples: []string{"/status              — full status overview", "/status --short      — abbreviated status", "/status --git        — git status only"},
			Related:  []string{"/diff", "/history", "/session"},
			Category: "slash-commands",
		},
		{
			Topic:    "/fix",
			Summary:  "Auto-fix errors and issues",
			Detail:   "Analyzes errors from tests, linting, or compilation and applies AI-generated fixes.",
			Examples: []string{"/fix                 — fix last error", "/fix test            — fix failing tests", "/fix lint            — fix lint issues"},
			Related:  []string{"/test", "/lint", "/bugfind"},
			Category: "slash-commands",
		},
		{
			Topic:    "/config",
			Summary:  "Open configuration panel",
			Detail:   "Opens the interactive configuration panel for hawk settings, model selection, and preferences.",
			Examples: []string{"/config              — open config panel", "/config model        — change model", "/config key remove   — remove stored API key", "/config keys         — show key status"},
			Related:  []string{"/session", "/profile", "/rules"},
			Category: "slash-commands",
		},
		{
			Topic:    "/chat",
			Summary:  "Start or continue a conversation",
			Detail:   "Opens the AI chat interface for freeform conversation about your code, design decisions, or debugging.",
			Examples: []string{"/chat                — start new conversation", "/chat \"explain this\" — ask a question directly", "/chat --continue     — continue last conversation"},
			Related:  []string{"/explain", "/ask", "/session"},
			Category: "slash-commands",
		},
		{
			Topic:    "/explain",
			Summary:  "Explain code or concepts",
			Detail:   "Provides AI-generated explanations of selected code, functions, or architectural patterns.",
			Examples: []string{"/explain main.go     — explain a file", "/explain func Init   — explain a function", "/explain --deep      — detailed explanation"},
			Related:  []string{"/chat", "/docs", "/repomap"},
			Category: "slash-commands",
		},
		{
			Topic:    "/search",
			Summary:  "Semantic code search",
			Detail:   "Searches your codebase using semantic understanding, not just text matching.",
			Examples: []string{"/search \"auth logic\"  — find authentication code", "/search --type func  — search functions only", "/search --file *.go  — limit to Go files"},
			Related:  []string{"/explain", "/repomap", "/find"},
			Category: "slash-commands",
		},
		{
			Topic:    "/format",
			Summary:  "Format code files",
			Detail:   "Formats source files using the configured formatter for each language.",
			Examples: []string{"/format              — format all changed files", "/format main.go      — format specific file", "/format --check      — check without modifying"},
			Related:  []string{"/lint", "/fix", "/commit"},
			Category: "slash-commands",
		},
		{
			Topic:    "/merge",
			Summary:  "Merge branches with conflict resolution",
			Detail:   "Merges branches and provides AI-assisted conflict resolution when conflicts arise.",
			Examples: []string{"/merge main          — merge main into current", "/merge --squash feat — squash merge a feature", "/merge --abort       — abort in-progress merge"},
			Related:  []string{"/branch", "/diff", "/commit"},
			Category: "slash-commands",
		},
		{
			Topic:    "/snapshot",
			Summary:  "Save or restore project snapshots",
			Detail:   "Creates point-in-time snapshots of your project state that can be restored later.",
			Examples: []string{"/snapshot            — create a snapshot", "/snapshot list       — list all snapshots", "/snapshot restore 3  — restore snapshot #3"},
			Related:  []string{"/undo", "/history", "/branch"},
			Category: "slash-commands",
		},
		{
			Topic:    "/history",
			Summary:  "View session and action history",
			Detail:   "Displays a timeline of actions taken during the current or past sessions.",
			Examples: []string{"/history             — show recent history", "/history --all       — show full history", "/history --session 5 — history for session 5"},
			Related:  []string{"/undo", "/snapshot", "/status"},
			Category: "slash-commands",
		},
		{
			Topic:    "/session",
			Summary:  "Manage hawk sessions",
			Detail:   "Start, stop, resume, or list hawk working sessions with full context preservation.",
			Examples: []string{"/session             — show current session", "/session new         — start new session", "/session resume 3    — resume session #3"},
			Related:  []string{"/status", "/history", "/config"},
			Category: "slash-commands",
		},
		{
			Topic:    "/bugfind",
			Summary:  "AI-powered bug detection",
			Detail:   "Scans code for potential bugs, race conditions, and logic errors using AI analysis.",
			Examples: []string{"/bugfind             — scan all changed files", "/bugfind ./pkg/...   — scan specific package", "/bugfind --deep      — thorough analysis"},
			Related:  []string{"/test", "/lint", "/fix"},
			Category: "slash-commands",
		},
		{
			Topic:    "/profile",
			Summary:  "Manage user profiles",
			Detail:   "Switch between user profiles with different settings, API keys, and preferences.",
			Examples: []string{"/profile             — show current profile", "/profile work        — switch to work profile", "/profile create home — create new profile"},
			Related:  []string{"/config", "/session", "/rules"},
			Category: "slash-commands",
		},
		{
			Topic:    "/rules",
			Summary:  "Manage project rules",
			Detail:   "View and edit project-specific rules that guide AI behavior and code generation.",
			Examples: []string{"/rules               — list active rules", "/rules add \"no tabs\" — add a new rule", "/rules edit 3        — edit rule #3"},
			Related:  []string{"/config", "/profile", "/docs"},
			Category: "slash-commands",
		},
		{
			Topic:    "/docs",
			Summary:  "Generate or view documentation",
			Detail:   "Generates documentation for functions, packages, or entire projects using AI.",
			Examples: []string{"/docs                — generate for changed files", "/docs pkg/auth       — document a package", "/docs --format md    — output as markdown"},
			Related:  []string{"/explain", "/repomap", "/chat"},
			Category: "slash-commands",
		},
		{
			Topic:    "/repomap",
			Summary:  "Generate repository map",
			Detail:   "Creates an AI-generated map of your repository structure, showing key files and their relationships.",
			Examples: []string{"/repomap             — full repo map", "/repomap --compact   — abbreviated map", "/repomap ./cmd       — map a subdirectory"},
			Related:  []string{"/explain", "/search", "/docs"},
			Category: "slash-commands",
		},
		{
			Topic:    "/find",
			Summary:  "Find files and symbols",
			Detail:   "Locates files, functions, types, and other symbols in your project.",
			Examples: []string{"/find Handler        — find symbol by name", "/find --type struct  — find all structs", "/find --file *.test  — find test files"},
			Related:  []string{"/search", "/repomap", "/explain"},
			Category: "slash-commands",
		},
		{
			Topic:    "/ask",
			Summary:  "Ask a question about the codebase",
			Detail:   "Asks the AI a question with full codebase context, without making any changes.",
			Examples: []string{"/ask \"how does auth work?\"", "/ask \"what calls this function?\"", "/ask \"is this thread-safe?\""},
			Related:  []string{"/chat", "/explain", "/search"},
			Category: "slash-commands",
		},
		// ─── Common Tasks ────────────────────────────────────────────
		{
			Topic:    "how to fix tests",
			Summary:  "Fixing failing tests with hawk",
			Detail:   "Use /test to run tests and identify failures, then /fix test to apply AI-generated fixes. For complex failures, use /chat to discuss the issue.",
			Examples: []string{"/test                — identify failures", "/fix test            — auto-fix test failures", "/chat \"why is TestX failing?\""},
			Related:  []string{"/test", "/fix", "/bugfind"},
			Category: "common-tasks",
		},
		{
			Topic:    "how to commit",
			Summary:  "Making commits with hawk",
			Detail:   "Stage your changes with git add, then use /commit to create a commit with an AI-generated message. Use /diff first to review what you are committing.",
			Examples: []string{"/diff --staged        — review staged changes", "/commit              — commit with AI message", `/commit "my message" — commit with custom message`},
			Related:  []string{"/commit", "/diff", "/branch"},
			Category: "common-tasks",
		},
		{
			Topic:    "how to debug",
			Summary:  "Debugging with hawk AI assistance",
			Detail:   "Describe the bug in /chat or use /bugfind for automated detection. For test failures, /test with /fix provides targeted debugging.",
			Examples: []string{"/bugfind             — automated bug detection", "/chat \"I see panic at line 42\"", "/test -v ./pkg/...   — verbose test output"},
			Related:  []string{"/bugfind", "/test", "/fix", "/chat"},
			Category: "common-tasks",
		},
		{
			Topic:    "how to review code",
			Summary:  "Code review assistance",
			Detail:   "Use /diff to see changes with AI summary, /bugfind to scan for issues, and /explain for understanding complex sections.",
			Examples: []string{"/diff                — see changes with AI summary", "/bugfind             — scan for potential issues", "/explain func Handle — understand a function"},
			Related:  []string{"/diff", "/bugfind", "/explain"},
			Category: "common-tasks",
		},
		{
			Topic:    "how to refactor",
			Summary:  "Refactoring code with AI assistance",
			Detail:   "Use /chat to discuss refactoring strategies, then hawk can apply changes. Use /snapshot before large refactors for safety.",
			Examples: []string{"/snapshot            — save state before refactor", "/chat \"refactor auth to use interfaces\"", "/test                — verify nothing broke"},
			Related:  []string{"/chat", "/snapshot", "/test", "/undo"},
			Category: "common-tasks",
		},
		{
			Topic:    "how to start a session",
			Summary:  "Starting a new hawk session",
			Detail:   "Run hawk in your project directory to start a session. Your context is preserved across the session. Use /session to manage sessions.",
			Examples: []string{"hawk                 — start hawk in current dir", "/session new         — start fresh session", "/session resume      — resume last session"},
			Related:  []string{"/session", "/config", "/status"},
			Category: "common-tasks",
		},
		// ─── Error Explanations ──────────────────────────────────────
		{
			Topic:    "error: api key invalid",
			Summary:  "API key is missing or invalid",
			Detail:   "Your API key is not configured or has expired. Save a new key via /config (paste in the panel). Keys are stored in the OS secret store (macOS Keychain / Linux keyring).",
			Examples: []string{"/config              — paste API key in the config panel", "hawk credentials status — verify stored keys"},
			Related:  []string{"/config", "error: rate limit", "error: network"},
			Category: "errors",
		},
		{
			Topic:    "error: rate limit",
			Summary:  "API rate limit exceeded",
			Detail:   "You have exceeded the API rate limit. Hawk will automatically retry with exponential backoff. Consider upgrading your plan for higher limits.",
			Examples: []string{"/config model        — switch to a lower-tier model", "/status              — check rate limit status"},
			Related:  []string{"error: api key invalid", "/config", "error: timeout"},
			Category: "errors",
		},
		{
			Topic:    "error: network",
			Summary:  "Network connection failed",
			Detail:   "Unable to reach the API server. Check your internet connection and proxy settings.",
			Examples: []string{"/config proxy        — configure proxy", "export HTTPS_PROXY=http://proxy:8080"},
			Related:  []string{"error: timeout", "error: api key invalid", "/config"},
			Category: "errors",
		},
		{
			Topic:    "error: timeout",
			Summary:  "Request timed out",
			Detail:   "The API request took too long and was cancelled. This may be due to network issues or a complex request. Try again or simplify your request.",
			Examples: []string{"/config timeout 60   — increase timeout to 60s", "/chat --short        — request shorter response"},
			Related:  []string{"error: network", "error: rate limit", "/config"},
			Category: "errors",
		},
		{
			Topic:    "error: git conflict",
			Summary:  "Git merge conflict detected",
			Detail:   "A merge conflict was encountered. Hawk can assist with conflict resolution using AI to understand both sides.",
			Examples: []string{"/merge --resolve     — AI-assisted resolution", "/diff --conflicts    — show conflict details", "/undo                — abort and go back"},
			Related:  []string{"/merge", "/diff", "/undo"},
			Category: "errors",
		},
		{
			Topic:    "error: no git repo",
			Summary:  "Not in a git repository",
			Detail:   "Hawk requires a git repository. Initialize one with git init or navigate to an existing repo.",
			Examples: []string{"git init             — initialize new repo", "cd /path/to/repo     — navigate to a repo"},
			Related:  []string{"/status", "/branch", "/commit"},
			Category: "errors",
		},
		{
			Topic:    "error: model not available",
			Summary:  "Selected model is not available",
			Detail:   "The configured model is not available for your account. Switch to a different model or check your plan.",
			Examples: []string{"/config model        — change model", "/config model sonnet — use a specific model"},
			Related:  []string{"/config", "error: api key invalid"},
			Category: "errors",
		},
		{
			Topic:    "error: file too large",
			Summary:  "File exceeds size limit",
			Detail:   "The file is too large to process in a single request. Try working with smaller sections or use streaming mode.",
			Examples: []string{"/explain func X      — explain specific function", "/chat \"summarize main.go lines 1-100\""},
			Related:  []string{"/explain", "/search", "/config"},
			Category: "errors",
		},
		// ─── Configuration Options ───────────────────────────────────
		{
			Topic:    "config: model",
			Summary:  "Configure the AI model",
			Detail:   "Choose which AI model to use. Options include various sizes with different speed/quality tradeoffs.",
			Examples: []string{"/config model opus   — use most capable model", "/config model sonnet — balanced model", "/config model haiku  — fastest model"},
			Related:  []string{"/config", "config: api-key", "config: temperature"},
			Category: "configuration",
		},
		{
			Topic:    "config: api-key",
			Summary:  "Set the API key",
			Detail:   "API keys are stored in the OS secret store. Use /config to paste a key, or /config key remove to delete one.",
			Examples: []string{"/config              — paste API key in the config panel", "/config key remove   — remove a stored key", "hawk credentials status — list configured providers"},
			Related:  []string{"/config", "config: model", "error: api key invalid"},
			Category: "configuration",
		},
		{
			Topic:    "config: temperature",
			Summary:  "Set model temperature",
			Detail:   "Controls randomness in AI responses. Lower values (0.0) are more deterministic, higher values (1.0) are more creative.",
			Examples: []string{"/config temperature 0.0 — deterministic", "/config temperature 0.7 — balanced", "/config temperature 1.0 — creative"},
			Related:  []string{"/config", "config: model", "config: max-tokens"},
			Category: "configuration",
		},
		{
			Topic:    "config: max-tokens",
			Summary:  "Set maximum response length",
			Detail:   "Limits the maximum number of tokens in AI responses. Higher values allow longer responses but cost more.",
			Examples: []string{"/config max-tokens 1024  — short responses", "/config max-tokens 4096  — medium responses", "/config max-tokens 16384 — long responses"},
			Related:  []string{"/config", "config: temperature", "config: model"},
			Category: "configuration",
		},
		{
			Topic:    "config: editor",
			Summary:  "Set preferred editor",
			Detail:   "Configure which editor hawk uses when opening files for editing.",
			Examples: []string{"/config editor vim    — use vim", "/config editor code   — use VS Code", "/config editor nano   — use nano"},
			Related:  []string{"/config", "config: theme", "config: shell"},
			Category: "configuration",
		},
		{
			Topic:    "config: theme",
			Summary:  "Set color theme",
			Detail:   "Choose the color theme for hawk terminal output. Supports light and dark terminal backgrounds.",
			Examples: []string{"/config theme dark    — dark background theme", "/config theme light   — light background theme", "/config theme none    — disable colors"},
			Related:  []string{"/config", "config: editor", "config: shell"},
			Category: "configuration",
		},
		{
			Topic:    "config: shell",
			Summary:  "Set shell for command execution",
			Detail:   "Configure which shell hawk uses when running commands.",
			Examples: []string{"/config shell bash   — use bash", "/config shell zsh    — use zsh", "/config shell fish   — use fish"},
			Related:  []string{"/config", "config: editor", "config: theme"},
			Category: "configuration",
		},
		{
			Topic:    "config: proxy",
			Summary:  "Configure HTTP proxy",
			Detail:   "Set a proxy server for API requests. Useful in corporate environments.",
			Examples: []string{"/config proxy http://proxy:8080", "/config proxy socks5://proxy:1080", "/config proxy none   — disable proxy"},
			Related:  []string{"/config", "error: network", "config: timeout"},
			Category: "configuration",
		},
		{
			Topic:    "config: timeout",
			Summary:  "Set request timeout",
			Detail:   "Configure how long to wait for API responses before timing out. Default is 30 seconds.",
			Examples: []string{"/config timeout 30   — 30 second timeout", "/config timeout 60   — 60 second timeout", "/config timeout 120  — 2 minute timeout"},
			Related:  []string{"/config", "error: timeout", "config: proxy"},
			Category: "configuration",
		},
		// ─── Tool Descriptions ───────────────────────────────────────
		{
			Topic:    "tool: repomap",
			Summary:  "Repository mapping engine",
			Detail:   "Analyzes your repository structure to build a semantic map of files, symbols, and their relationships. Used internally to provide context to the AI.",
			Examples: []string{"/repomap             — view the generated map", "/repomap --refresh   — force rebuild"},
			Related:  []string{"/search", "/explain", "tool: embeddings"},
			Category: "tools",
		},
		{
			Topic:    "tool: embeddings",
			Summary:  "Code embedding engine",
			Detail:   "Generates vector embeddings of your code for semantic search and similarity matching.",
			Examples: []string{"/search \"auth logic\"  — uses embeddings internally"},
			Related:  []string{"/search", "tool: repomap", "tool: memory"},
			Category: "tools",
		},
		{
			Topic:    "tool: memory",
			Summary:  "Session memory system",
			Detail:   "Stores context across sessions including conversation history, decisions made, and project understanding.",
			Examples: []string{"/session resume      — restore memory from session", "/history             — view remembered actions"},
			Related:  []string{"/session", "/history", "tool: embeddings"},
			Category: "tools",
		},
		{
			Topic:    "tool: snapshot",
			Summary:  "Project snapshot engine",
			Detail:   "Creates and manages point-in-time snapshots of your project for safe experimentation and undo capability.",
			Examples: []string{"/snapshot            — create a snapshot", "/snapshot restore 2  — restore a snapshot", "/undo                — uses snapshots internally"},
			Related:  []string{"/snapshot", "/undo", "tool: memory"},
			Category: "tools",
		},
		{
			Topic:    "tool: parallel",
			Summary:  "Parallel execution engine",
			Detail:   "Runs multiple AI tasks in parallel for faster results. Used for multi-file analysis and batch operations.",
			Examples: []string{"/bugfind             — scans files in parallel", "/test                — analyzes results in parallel"},
			Related:  []string{"/bugfind", "/test", "tool: repomap"},
			Category: "tools",
		},
		{
			Topic:    "tool: sandbox",
			Summary:  "Safe execution sandbox",
			Detail:   "Provides an isolated environment for running code and commands safely without affecting your project.",
			Examples: []string{"/test                — runs in sandbox", "/fix                 — validates fixes in sandbox"},
			Related:  []string{"/test", "/fix", "tool: snapshot"},
			Category: "tools",
		},
		{
			Topic:    "tool: daemon",
			Summary:  "Background daemon service",
			Detail:   "Runs in the background to provide file watching, indexing, and real-time analysis of your project.",
			Examples: []string{"hawk daemon start    — start the daemon", "hawk daemon status   — check daemon status", "hawk daemon stop     — stop the daemon"},
			Related:  []string{"/status", "tool: repomap", "tool: memory"},
			Category: "tools",
		},
	}

	for _, entry := range entries {
		ch.Entries[entry.Topic] = entry
	}
}

// GetHelp retrieves a specific help entry by topic.
func (ch *ContextualHelp) GetHelp(topic string) *HelpEntry {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if entry, ok := ch.Entries[topic]; ok {
		return entry
	}

	// Try case-insensitive lookup
	lower := strings.ToLower(topic)
	for key, entry := range ch.Entries {
		if strings.ToLower(key) == lower {
			return entry
		}
	}
	return nil
}

// SearchHelp performs fuzzy search across topics, summaries, and details.
func (ch *ContextualHelp) SearchHelp(query string) []*HelpEntry {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.searchHelpLocked(query)
}

// searchHelpLocked performs the search without acquiring the lock (caller must hold it).
func (ch *ContextualHelp) searchHelpLocked(query string) []*HelpEntry {
	if query == "" {
		return nil
	}

	query = strings.ToLower(query)
	terms := strings.Fields(query)

	type scored struct {
		entry *HelpEntry
		score int
	}

	var results []scored

	for _, entry := range ch.Entries {
		score := 0
		topicLower := strings.ToLower(entry.Topic)
		summaryLower := strings.ToLower(entry.Summary)
		detailLower := strings.ToLower(entry.Detail)

		for _, term := range terms {
			// Exact topic match gets highest score
			if topicLower == term {
				score += 100
			}
			// Topic contains term
			if strings.Contains(topicLower, term) {
				score += 50
			}
			// Summary contains term
			if strings.Contains(summaryLower, term) {
				score += 30
			}
			// Detail contains term
			if strings.Contains(detailLower, term) {
				score += 10
			}
			// Check examples
			for _, ex := range entry.Examples {
				if strings.Contains(strings.ToLower(ex), term) {
					score += 5
				}
			}
			// Category match
			if strings.Contains(strings.ToLower(entry.Category), term) {
				score += 15
			}
		}

		// Fuzzy matching: check if any term is a substring of any word in the entry
		if score == 0 {
			for _, term := range terms {
				if len(term) >= 3 {
					allText := topicLower + " " + summaryLower + " " + detailLower
					words := strings.Fields(allText)
					for _, word := range words {
						if fuzzyMatch(term, word) {
							score += 5
						}
					}
				}
			}
		}

		if score > 0 {
			results = append(results, scored{entry: entry, score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	entries := make([]*HelpEntry, 0, len(results))
	for _, r := range results {
		entries = append(entries, r.entry)
	}
	return entries
}

// fuzzyMatch checks if two strings are similar enough to be considered a match.
func fuzzyMatch(query, target string) bool {
	if strings.Contains(target, query) || strings.Contains(query, target) {
		return true
	}

	// Simple Levenshtein-based threshold: if strings are close in length
	// and share a common prefix or suffix
	if len(query) < 3 || len(target) < 3 {
		return false
	}

	// Common prefix match (at least 3 chars)
	prefixLen := 0
	minLen := len(query)
	if len(target) < minLen {
		minLen = len(target)
	}
	for i := 0; i < minLen; i++ {
		if query[i] == target[i] {
			prefixLen++
		} else {
			break
		}
	}
	if prefixLen >= 3 {
		return true
	}

	// Common suffix match (at least 3 chars)
	suffixLen := 0
	for i := 0; i < minLen; i++ {
		if query[len(query)-1-i] == target[len(target)-1-i] {
			suffixLen++
		} else {
			break
		}
	}
	if suffixLen >= 3 {
		return true
	}

	return false
}

// SuggestHelp returns relevant help entries based on the current context.
func (ch *ContextualHelp) SuggestHelp(context string) []*HelpEntry {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if context == "" {
		return nil
	}

	contextLower := strings.ToLower(context)
	var suggestions []*HelpEntry

	// After error -> suggest error help and related fixes
	if strings.Contains(contextLower, "error") || strings.Contains(contextLower, "failed") || strings.Contains(contextLower, "panic") {
		for _, entry := range ch.Entries {
			if entry.Category == "errors" {
				// Check if this specific error is mentioned
				if strings.Contains(contextLower, strings.ToLower(strings.TrimPrefix(entry.Topic, "error: "))) {
					suggestions = append(suggestions, entry)
				}
			}
		}
		// Also suggest fix-related commands
		if fix := ch.Entries["/fix"]; fix != nil {
			suggestions = append(suggestions, fix)
		}
		if bugfind := ch.Entries["/bugfind"]; bugfind != nil {
			suggestions = append(suggestions, bugfind)
		}

		if len(suggestions) > 0 {
			return suggestions
		}
		// Generic error suggestions
		for _, entry := range ch.Entries {
			if entry.Category == "errors" {
				suggestions = append(suggestions, entry)
				if len(suggestions) >= 5 {
					break
				}
			}
		}
		return suggestions
	}

	// First session / onboarding context
	if strings.Contains(contextLower, "first session") || strings.Contains(contextLower, "onboarding") || strings.Contains(contextLower, "getting started") || strings.Contains(contextLower, "new user") {
		onboardingTopics := []string{"/status", "/chat", "/config", "how to commit", "how to start a session"}
		for _, topic := range onboardingTopics {
			if entry, ok := ch.Entries[topic]; ok {
				suggestions = append(suggestions, entry)
			}
		}
		return suggestions
	}

	// After /config -> configuration reference
	if strings.Contains(contextLower, "/config") || strings.Contains(contextLower, "configuration") || strings.Contains(contextLower, "settings") {
		for _, entry := range ch.Entries {
			if entry.Category == "configuration" {
				suggestions = append(suggestions, entry)
			}
		}
		return suggestions
	}

	// Testing context
	if strings.Contains(contextLower, "test") {
		testTopics := []string{"/test", "/fix", "how to fix tests", "/bugfind"}
		for _, topic := range testTopics {
			if entry, ok := ch.Entries[topic]; ok {
				suggestions = append(suggestions, entry)
			}
		}
		return suggestions
	}

	// Git / commit context
	if strings.Contains(contextLower, "commit") || strings.Contains(contextLower, "git") || strings.Contains(contextLower, "branch") {
		gitTopics := []string{"/commit", "/diff", "/branch", "/merge", "how to commit"}
		for _, topic := range gitTopics {
			if entry, ok := ch.Entries[topic]; ok {
				suggestions = append(suggestions, entry)
			}
		}
		return suggestions
	}

	// Fallback: search for relevant entries
	return ch.searchHelpLocked(context)
}

// FormatHelp formats a single help entry for display.
func (ch *ContextualHelp) FormatHelp(entry *HelpEntry) string {
	if entry == nil {
		return ""
	}

	var b strings.Builder

	// Header line: topic — summary
	header := fmt.Sprintf("%s — %s", entry.Topic, entry.Summary)
	b.WriteString(header)
	b.WriteString("\n")

	// Separator line
	sep := strings.Repeat("─", len(header))
	b.WriteString(sep)
	b.WriteString("\n")

	// Detail
	b.WriteString(entry.Detail)
	b.WriteString("\n")

	// Examples
	if len(entry.Examples) > 0 {
		b.WriteString("\nExamples:\n")
		for _, ex := range entry.Examples {
			b.WriteString("  ")
			b.WriteString(ex)
			b.WriteString("\n")
		}
	}

	// Related
	if len(entry.Related) > 0 {
		b.WriteString("\nRelated: ")
		b.WriteString(strings.Join(entry.Related, ", "))
		b.WriteString("\n")
	}

	return b.String()
}

// FormatSearchResults formats multiple help entries as a search result list.
func (ch *ContextualHelp) FormatSearchResults(entries []*HelpEntry) string {
	if len(entries) == 0 {
		return "No results found."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d result(s):\n\n", len(entries)))

	for i, entry := range entries {
		b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, entry.Topic))
		b.WriteString(fmt.Sprintf("     %s\n", entry.Summary))
		if i < len(entries)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// GetCategories returns all unique categories sorted alphabetically.
func (ch *ContextualHelp) GetCategories() []string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	seen := make(map[string]bool)
	for _, entry := range ch.Entries {
		seen[entry.Category] = true
	}

	categories := make([]string, 0, len(seen))
	for cat := range seen {
		categories = append(categories, cat)
	}
	sort.Strings(categories)
	return categories
}

// ListByCategory returns all help entries in a given category.
func (ch *ContextualHelp) ListByCategory(category string) []*HelpEntry {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	var entries []*HelpEntry
	categoryLower := strings.ToLower(category)

	for _, entry := range ch.Entries {
		if strings.ToLower(entry.Category) == categoryLower {
			entries = append(entries, entry)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Topic < entries[j].Topic
	})

	return entries
}
