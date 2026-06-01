# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- **Version re-baselined to `0.2.0`** across `main.go`, `api/server.go`, `flake.nix`,
  `.github/workflows/release.yml`, and the `update`/`api` test suites, aligning hawk
  with the rest of the GrayCodeAI ecosystem (`eyrie`, `tok`, `yaad`, `sight`, `inspect`).

### Added
- REPL magic commands (%reset, %undo, %tokens, %history, %copy, %save, %compact, %model, %clear)
- Prompt cache keep-alive pings
- Unified Finding type in shared/types for cross-tool interoperability

### Added — Round 2 of rtk + caveman porting (2026-06-01)
- **Cavecrew personas** (`internal/multiagent/agents`): three new
  built-in personas ported from JuliusBrussee/caveman
  (`cavecrew-investigator`, `cavecrew-builder`, `cavecrew-reviewer`).
  Each enforces a strict output format so downstream agents can parse
  outputs mechanically:
  - `cavecrew-investigator`: `path:line — symbol — note`, max 6 words per note
  - `cavecrew-builder`: hard-refuses tasks touching 3+ files
  - `cavecrew-reviewer`: severity emoji (🔴/🟡/🔵/❓) at the start of every line
  Exposed via `CavecrewPersonas()` helper and `EnsureCavecrew()`
  registry method; `BuiltinPersonas()` returns 21 (was 18).
- **`internal/safewrite`** package: hardened atomic file-write utility
  ported from caveman's `safeWriteFlag`. Refuses symlinks at destination
  and parent, refuses paths that escape via `..`, opens with
  `O_NOFOLLOW` via `golang.org/x/sys/unix`, writes to a temp file
  with mode 0600, syncs to disk, then atomically renames. `ErrSymlinkTarget`
  and `ErrPathEscape` sentinel errors.
- **`internal/jsonc`** package: JSON-with-Comments parser and
  `ValidateClaudeSettings` validator, ported from caveman's settings
  parser and `validateHookFields`. Accepts `//` and `/* */` comments
  plus trailing commas in objects and arrays. Validates Claude Code
  `settings.json` fields (model, permissions, hooks, mcpServers,
  env) with type checks and value validation.
- **`internal/permissions/verdict.go`**: unified `PermissionVerdict`
  type with Risk levels (`RiskLow`, `RiskMedium`, `RiskHigh`,
  `RiskBlocked`). Helpers: `Allow`, `Deny`, `RequireApproval`. Additive
  change — existing `GuardianDecision` is unchanged.
- **`internal/providers`** package: PROVIDERS matrix (34 entries)
  ported from caveman's `bin/install.js` (PROVIDERS array). Each
  entry describes an AI coding agent (Claude Code, Cursor, Codex,
  Aider, etc.) with install mechanism and detection probes. Probe
  kinds: `command` (PATH), `dir` (filesystem), `vscode-ext`,
  `cursor-ext`, `macapp`, `jetbrains-plugin`. API: `Get(id)`,
  `All()`, `Hard()`, `Detect()`. `Soft: true` means detection is
  best-effort; soft providers are excluded from auto-detect.
- **`internal/session.GainTracker`**: per-session gain event recording
  in a new `gains` SQLite table inside the session store. Each event
  captures original/compressed byte + token counts, mode/tier/model,
  and a command label. API: `Record`, `AggregateForSession`,
  `ListForSession`, `PruneForSession`. Scoped to a single session so
  callers can selectively compact their own history. Companion to
  tok's `internal/tracking.Tracker` (tok tracks globally, hawk tracks
  per-session).

### Added — Production Hardening (top-50 OSS parity)
- **Stricter linting**: `.golangci.yml` v2 config enabling `errcheck`, `staticcheck`,
  `gocritic` (diagnostic + performance), `unused`, `ineffassign`, `misspell`, `noctx`,
  `bodyclose`, `unconvert`, `whitespace`, with `govet enable-all` (minus `fieldalignment`).
- **CI parity**: race-detector tests with coverage upload, golangci-lint v2 action,
  `govulncheck` and `gosec` security scans, multi-platform build matrix
  (linux/darwin/windows × amd64/arm64), benchmark job on PRs.
- **Makefile** with standard targets: `build`, `test`, `test-coverage`, `test-10x`,
  `lint`, `fmt`, `vet`, `security`, `bench`, `clean`, `install`, `release`, `help`.
- **Container**: Dockerfile uses `tini` init, embeds tzdata, verifies deps, runs as
  non-root.
- **Repository hygiene**: `.editorconfig` for cross-editor consistency,
  `.github/dependabot.yml` for automated dep updates, `CONTRIBUTING.md` with the
  full contributor workflow.

### Fixed — Correctness
- 240+ unchecked error returns hardened across `session`, `engine`, `tool`, `config`,
  `auth`, `cmd`, `daemon`, `diffsandbox`, `analytics`, `cmdhistory`, `container`,
  `eval`, `fingerprint`, `update` packages.
- Dead-code removal: 13 unused declarations removed (caught by the `unused` linter).
- Real bugs fixed where `append` results were silently discarded
  (`mcp/server.go`, `repomap/depgraph.go`, flagged by `staticcheck SA4010`).
- `session` package is now fully `errcheck`-clean, protecting persistence integrity.

### Tests
- `auth` package coverage raised from ~18% to ~71% with table-driven tests covering
  every code path.
- `update` package coverage raised from ~22% to ~92% with full HTTP mocking, including
  error paths (server failure, invalid JSON, unreachable host) and `Summary()` rendering.

## [0.4.0] — 2026-05-05

### Added
- **Exec Subcommand**: `hawk exec "prompt"` — full engine non-interactive mode with `--output-format json`, `--auto` autonomy levels, `--worktree` isolation, `--agent` personas, `--session-id` resume, stdin piping
- **Daemon Server**: `hawk daemon start/stop/status` — background HTTP server with JSON + SSE streaming on `/v1/chat`, `/v1/health`, `/v1/sessions`
- **Mission Mode**: `hawk mission "prompt"` — multi-agent orchestration decomposing work into parallel features executed in isolated git worktrees. `--dry-run` for planning only
- **Session Search**: `hawk search "query"` — full-text search across all saved sessions with `--json` output
- **Custom Agents**: `hawk agent list/create/show/remove` — markdown persona definitions in `~/.hawk/agents/` with YAML frontmatter (name, description, model)
- **Snapshot System**: Shadow git tracking of every file change. `hawk snapshot list/restore/diff` + `/snapshot` slash command. Auto-snapshots on every Write/Edit tool call
- **Waza Workflows**: `/think` (plan before code), `/hunt` (root-cause diagnosis), `/check` (pre-ship review with auto-fix), `/design` (screenshot-driven UI iteration)
- **Structured Compaction**: Summary template with Goal/Constraints/Progress/Files/Decisions/Errors/Instructions/Next sections for better intent preservation
- **Doom Loop Detection**: Lowered threshold to 3 (from 4). Two-tier escalation: first detection injects redirect prompt, doom loop hard-stops with "ask user for help"
- **Session Persistence for exec**: All exec runs saved to `~/.hawk/sessions/` and searchable via `hawk search`

### Packages Added
- `mission/` — Multi-agent orchestration with worktree-based parallel workers
- `daemon/` — HTTP server with session factory, SSE streaming, PID file management
- `agents/` — Markdown persona loader with frontmatter parsing
- `snapshot/` — Shadow git repository for granular file-level undo

### Inspired By
- Factory Droid v0.117.0 (exec mode, daemon, mission orchestration, custom agents)
- OpenCode (structured compaction, snapshot system, doom loop escalation)
- Waza by tw93 (engineering-habit workflows: think, hunt, check, design)

## [0.3.0] — 2026-05-03

### Added
- **Model Cascade Router**: Cost-aware routing that classifies prompts and selects optimal model tier (simple→Haiku, debug→Sonnet, generation→Opus). Supports frugal mode for aggressive cost savings. Tracks routing decisions for analytics.
- **Dynamic max_tokens**: Adaptive output budgets based on task type and recent tool-call patterns. Reduces output token costs 15-25% by not over-allocating.
- **Cheap Compaction Model**: Conversation summaries now use the cheapest available model (Haiku/gpt-4o-mini) instead of the primary model. Saves $0.10-0.50 per compaction.
- **Context Budget Allocator**: Formal token allocation across system prompt, tool defs, repo map, memory, workspace, pre-loaded files, and conversation. Adaptive: shrinks file budget as conversation grows. Triggers compaction at threshold.
- **LLM Reflection Engine**: Verbal self-reflection after failed attempts (Reflexion pattern). Asks "what failed, why, what to do differently" instead of mechanical summaries. Accumulates episodic memory buffer.
- **Self-Review Before Write**: Rubber duck debugging step between code generation and file write. Model explains its code and checks for bugs/regressions before applying.
- **Session Lifecycle (Self-Improvement Loop)**: Closed loop wiring OnSessionStart (retrieve guidelines + skills) and OnSessionEnd (learn guidelines, distill skills, record cost).
- **Import/Dependency Graph**: Parses import statements for Go, Python, TypeScript. Builds forward/reverse edges. DependenciesOf, DependentsOf, ImpactSet with BFS depth control.
- **Change-Set Aware Context**: Loads only code relevant to current `git diff`. 70-90% context reduction for focused tasks. FormatContext with token budgeting.
- **Landlock Sandbox (Linux)**: Zero-dependency, zero-overhead, unprivileged filesystem isolation. Restricts agent to project dir + /tmp. Default Linux sandbox.
- **seccomp-bpf Syscall Filtering (Linux)**: Blocks 21 dangerous syscalls (mount, ptrace, reboot, kexec_load, init_module, bpf, etc.). Applied via SysProcAttr.

### Changed
- `generateSummary()` now uses cheapest available model per provider instead of primary model
- Ecosystem roadmap added: `ECOSYSTEM-ROADMAP.md` with 30-feature prioritized implementation plan

## [0.2.0] — 2026-05-01

### Added
- **Bash Security**: zsh bypass protection, process substitution blocking, IFS injection detection, carriage return prevention, ANSI-C quoting detection, git commit safety
- **Hook System**: 8 event types with priority-based execution (pre_query, post_query, pre_tool, post_tool, session_start, session_end, permission_ask, error)
- **Plugin System**: manifest validation, install/list/uninstall, hook registration, command execution
- **Advanced Permissions**: auto-mode learning, command classifier, bypass killswitch, shadowed rule detection
- **Model Catalog**: 25+ models across 7 providers with pricing and context sizes
- **Session Memory**: extraction, search, and consolidation
- **Analytics**: event logging, session traces, cost tracking
- **Auth System**: secure token storage with OS keychain integration
- **Auto-Update**: GitHub release checking
- **LSP Integration**: JSON-RPC client and server manager
- **Voice Mode**: Whisper.cpp integration
- **Magic Docs**: Go AST parsing and automatic markdown generation
- **Worktree Tools**: EnterWorktree/ExitWorktree with validation
- **Retry Package**: exponential backoff for API resilience
- **Circuit Breaker**: three-state circuit breaker for fault tolerance
- **Rate Limiter**: token bucket algorithm
- **Logger**: structured logging with levels
- **Health Checks**: registry with status aggregation
- **Metrics**: counters, gauges, timers with atomic operations
- **Graceful Shutdown**: signal-based shutdown with hooks
- **Profiling**: CPU, memory, goroutine profiling
- **Tracing**: distributed tracing spans
- **Config Validation**: field-level validation errors
- **Benchmarks & Fuzz Tests**: bash security parsing
- **Shell Completion**: bash, zsh, fish, powershell
- **Docker**: multi-stage build with non-root user
- **Nix Flake**: reproducible builds
- **GitHub Actions**: CI with test, build, lint, coverage; release with GoReleaser

### Changed
- Improved error messages with context wrapping
- JSONL session storage with legacy JSON fallback
- Stream-JSON usage events with token tracking
- Pre-compiled regexes for performance

## [0.0.1] — 2026-04-30

### Added
- Project scaffold with cobra CLI and Bubbletea TUI
- Interactive chat REPL with textarea input, spinner, lipgloss styling
- eyrie wired as LLM provider dependency
- GitHub Actions CI
