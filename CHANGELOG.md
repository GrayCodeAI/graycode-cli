# Changelog

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
