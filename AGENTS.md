# AGENTS.md — Hawk Coding Agent

This file describes the hawk project for AI agents working in this codebase.
The TUI `/memory` command references this file.

---

## Project Overview

hawk is an AI-powered coding agent for the terminal. It reads codebases, writes
and edits files, runs tests, and manages git — all through natural language.
Built in Go with zero CGO dependencies, it ships as a single static binary for
linux/darwin/windows on amd64/arm64.

**Tagline:** AI coding agent for your terminal — built for developers, not teams
or enterprises.

## Ecosystem

hawk is part of the hawk-eco mono-ecosystem:

| Component | Purpose |
|-----------|---------|
| **hawk**  | AI coding agent (this repo) |
| **eyrie** | LLM provider runtime — routing, streaming, retries, caching |
| **yaad**  | Graph-based persistent memory for coding agents |
| **tok**   | Tokenizer, compression, secrets scanning, rate limiting |
| **sight** | Diff-based code review and static analysis |
| **inspect** | Security audit library (CVE, API security, CI output) |
| **trace** | Session capture and replay CLI |

Modules are pinned in `go.mod`. External checkouts live under `external/` with a
`go.work` file for local development.

## Architecture

```
hawk/
├── cmd/                    # CLI entry point (Cobra + Bubble Tea TUI)
├── internal/
│   ├── engine/             # Agent loop, compaction, context management
│   │   ├── ctxmgr/         # Context providers, packing, visualization
│   │   ├── token/          # Budget allocation, prediction
│   │   ├── streaming/      # Response cache, stream optimizer, thinking
│   │   ├── session/        # Compression, cross-session learning
│   │   ├── memory/         # Knowledge distillation
│   │   ├── planning/       # Goals, task decomposition
│   │   ├── workflow/        # JSON-defined automation pipelines
│   │   ├── review/         # Code review bot, quality scorer
│   │   ├── observability/  # Profiler, debug recorder
│   │   ├── validation/     # Lint loop, test loop
│   │   └── ...
│   ├── tool/               # 40+ built-in tools (file edit, git, codegen, etc.)
│   ├── config/             # Settings, env manager, migration
│   ├── session/            # SQLite persistence, search, export, replay
│   ├── permissions/        # Guardian, rules DSL, boundary checker
│   ├── sandbox/            # Seatbelt, landlock, net proxy
│   ├── intelligence/       # Repo map, AST analysis, dependency graphs
│   ├── multiagent/         # Personas, inter-agent messaging, sub-agents
│   ├── hooks/              # Event-driven plugin system
│   ├── mcp/                # Model Context Protocol client/server
│   ├── daemon/             # Background HTTP/SSE server
│   ├── resilience/         # Circuit breaker, rate limiting, health checks
│   └── feature/            # Eval, fingerprint, scaffolding
├── shared/types/           # Cross-repo exported types (severity, etc.)
├── docs/                   # Architecture docs, research notes
└── testdata/               # Test fixtures
```

## Key Design Decisions

- **Zero CGO:** Pure Go, cross-compilable. Tree-sitter is optional.
- **`internal/` is private:** Other repos import `shared/types/` only.
- **Tool safety layer:** Every tool call goes through permissions (guardian,
  rules DSL, boundary checker) before execution.
- **Engine-first:** The agent loop in `internal/engine/` orchestrates context
  packing, tool dispatch, streaming, and session persistence.
- **Ecosystem integration:** eyrie handles all LLM API communication. hawk
  never talks to LLM APIs directly.

## Development Guidelines

### Build & Test

```bash
go build .                    # Build binary
go test -race ./...           # Run all tests with race detector
make ci                       # Full CI suite (lint, test, security)
make cover                    # Coverage report
make path                     # Developer path verification
make smoke                    # Build + quick verification
```

### Go Conventions

- Standard Go project layout: `cmd/` for entry points, `internal/` for private
- Tests live alongside source files (`foo.go` → `foo_test.go`)
- Use table-driven tests where practical
- Errors are values — wrap with `fmt.Errorf("context: %w", err)`
- No global mutable state; prefer dependency injection

### Commit Conventions

Use [Conventional Commits](https://www.conventionalcommits.org/):
```
feat: add new tool
fix: handle edge case in file edit
docs: update AGENTS.md
refactor: extract context packing logic
test: add coverage for guardian
```

### Code Style

- `gofmt` and `go vet` are mandatory (enforced by CI)
- Keep functions focused; extract helpers for clarity
- Prefer explicit error handling over panics
- Comments on exported types/functions only (per Go convention)

### Adding a New Tool

1. Create `internal/tool/mytool.go`
2. Implement the tool interface (name, description, parameters, execute)
3. Register in the tool registry
4. Add tests in `mytool_test.go`
5. The tool automatically gets permission checking via the safety layer

### Adding a New Feature

1. Check FEATURES.md for the feature list and conventions
2. Place code in the appropriate `internal/` package
3. Follow existing patterns (e.g., context providers are pluggable)
4. Add tests and update documentation

## File Organization Notes

- `FEATURES.md` — Complete feature reference with 100 features across 12 categories
- `CONTRIBUTING.md` — PR process, commit conventions
- `docs/` — Architecture details, security model, ecosystem message flow
- `external/` — Ecosystem repo checkouts for `go.work` development
- `shared/types/` — Types exported for sight/inspect/tok (they must not import `internal/`)

## Testing Philosophy

- Unit tests for all new code
- Integration tests for tool execution and engine loop
- Race detector enabled in CI (`-race`)
- No test files committed with `t.Skip()` without a tracking issue

## Common Pitfalls

- Do not import `internal/` from other ecosystem repos — use `shared/types/`
- Do not put API keys in `.env` or shell env for hawk — use `/config` (OS keychain)
- The `external/` directory is for local dev only; CI clones repos separately
- `go.work` is for local multi-repo development; it is not committed

## Naming Conventions

- **Tool types**: `FooTool` struct implementing the `Tool` interface (`Name()`, `Description()`, `Parameters()`, `Execute()`)
- **Config types**: `Settings`, `MCPServerConfig`, `CustomProviderConfig` — no prefix, in `config` package
- **Engine types**: `Session`, `CoreLoop`, `SafetyLayer`, `Intelligence`, `Optimizer` — in `engine` package
- **Health checks**: `Checker` func type, `Check` struct with `Name`, `Status`, `Message`
- **Resilience**: `Breaker` (circuit breaker), `Config` + `Do` (retry), `Limiter` (rate limit)
- **Error types**: `ValidationError` with `Field`, `Message`, `Value`; `ValidationResult` with `Errors`, `Valid`
- **Bridges**: `Ready() bool` method, `NewBridge()` constructor, graceful degradation when unavailable

## API Patterns

- **Tool registration**: `tool.NewRegistry(tools...)` → `registry.Get("ToolName")` → `tool.Execute(ctx, input)`
- **Settings loading**: `config.LoadSettings()` merges global + project; `config.LoadGlobalSettings()` for global-only
- **Session construction**: `engine.NewSessionWithClient(client, provider, model, systemPrompt, registry, deploymentRouting)`
- **Service composition**: `engine.NewSessionServices(opts...)` with `WithProvider()`, `WithTools()`, `WithMemory()`, etc.
- **Health checks**: `health.NewRegistry()` → `registry.Register("name", checker)` → `registry.Run(ctx)` → `registry.Status()`
- **Circuit breaker**: `circuit.New(cfg)` → `breaker.Call(fn)` or `breaker.Allow()` → `breaker.State()`
- **Retry**: `retry.Do(ctx, cfg, fn)` with exponential backoff + jitter; `retry.DoWithResult[T]` for typed returns
- **Config validation**: `config.ValidateSettings(s)` returns `ValidationResult{Errors, Valid}`
- **Ecosystem panel**: `config.FormatEcosystemPanel(ctx, provider, model)` for diagnostics

## Testing Patterns

- **Table-driven tests** with `t.Run(name, func(t *testing.T){...})` for all multi-case tests
- **`t.Parallel()`** on all tests that don't share mutable state
- **`t.TempDir()`** for filesystem isolation (auto-cleanup)
- **`credentials.MapStore{}`** for credential isolation in tests:
  ```go
  store := &credentials.MapStore{}
  credentials.SetDefaultStore(store)
  t.Cleanup(func() { credentials.SetDefaultStore(nil) })
  ```
- **`bytes.Buffer`** as `io.Writer` for logger output capture
- **Fuzz tests** for input parsing robustness: `func FuzzFoo(f *testing.F) { ... }`
- **No mocks framework** — use concrete types and test doubles
- **Meta-audit tests** in `internal/testaudit/` enforce architectural invariants via go/ast

## Refactoring Guidelines

- **Safe to refactor**: `internal/resilience/` (retry, circuit, ratelimit, health) — no public API
- **Safe to refactor**: `internal/observability/logger/` — internal only, no external consumers
- **Safe to refactor**: `internal/system/` (bus, shutdown, retention, cron, staleness)
- **Caution**: `internal/engine/session.go` Session struct — widely referenced across 30+ sub-packages
- **Caution**: `internal/config/settings.go` Settings struct — serialized to JSON, dual-format (snake_case + camelCase)
- **Caution**: `internal/tool/` Tool interface — implemented by 40+ tools
- **Blocked**: `shared/types/` — exported to eyrie, sight, inspect, tok; changes break ecosystem

## Key File Locations

| What | Where |
|---|---|
| CLI entry point | `cmd/root.go` |
| Agent loop | `internal/engine/session.go` (`Stream()`, `agentLoop()`) |
| Session services | `internal/engine/session_services.go` |
| Tool interface | `internal/tool/tool.go` (`Tool`, `Registry`) |
| Tool context | `internal/tool/tool.go` (`ToolContext`, `WithToolContext`) |
| Settings | `internal/config/settings.go` (`Settings`, `LoadSettings()`) |
| Config validation | `internal/config/validator.go` (`ValidateSettings()`) |
| Config migration | `internal/config/migrate.go` (`MigrationRegistry`) |
| Env manager | `internal/config/envmanager.go` (`EnvManager`) |
| Health checks | `internal/resilience/health/health.go` (`Registry`, `Checker`) |
| Circuit breaker | `internal/resilience/circuit.go` (`Breaker`, `Manager`) |
| Retry | `internal/resilience/retry/retry.go` (`Do()`, `DoWithResult()`) |
| Rate limiter | `internal/resilience/ratelimit/ratelimit.go` (`Limiter`) |
| Logger | `internal/observability/logger/logger.go` (`Logger`) |
| Metrics | `internal/observability/metrics/metrics.go` (`Counter`, `Gauge`, `Timer`) |
| OTEL tracing | `internal/observability/oteltrace/trace.go` |
| Multi-agent missions | `internal/multiagent/mission.go` (`Mission`) |
| Message bus | `internal/multiagent/messaging.go` (`MessageBus`) |
| Shared memory | `internal/multiagent/shared_memory.go` (`SharedMemory`) |
| Session persistence | `internal/session/persist.go` |
| MCP client | `internal/mcp/mcp.go` |
| MCP server | `internal/mcp/server.go` |
| Provider routing | `internal/provider/routing/router.go` |
| Bridges | `internal/bridge/{inspect,sight,sessioncapture}/bridge.go` |
| Doctor diagnostics | `cmd/diagnostics.go` |
| Meta-audit tests | `internal/testaudit/audit_test.go` |

## Anti-Patterns

- **No `os.Getenv` in `internal/`** — use `config.EnvManager` to centralize env access. Exception: `internal/observability/oteltrace/` for telemetry env vars.
- **No `panic()` for error handling** — return `error` values. Exception: `init()` functions for package-level assertions.
- **No `fmt.Print` for logging** — use `logger.Logger` with structured fields. Exception: `internal/onboarding/` and `internal/engine/scaffold/` for user-facing CLI output.
- **No API keys in settings.json** — use OS secret store via `credentials` package and `/config` command.
- **No importing `internal/` from other ecosystem repos** — use `shared/types/` for cross-repo types.
- **No global mutable state** — prefer dependency injection via `deps` structs or `context.WithValue`.
- **No `t.Skip()` without a tracking issue** — every skipped test needs a GitHub issue number.
