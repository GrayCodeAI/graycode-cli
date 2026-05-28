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
