# Changelog

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
