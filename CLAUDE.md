# Hawk - AI Coding Agent

## Architecture

hawk is a model-agnostic AI coding agent with 59 packages organized around:

### Core Packages
- `cmd/` — CLI entry point (cobra + bubbletea TUI)
- `engine/` — Agent loop, compaction, self-improvement, loop detection
- `tool/` — 40 built-in tools with safety/permission layer
- `config/` — Settings, budget tracking, validation, agent personas

### Provider Layer
- `eyrie` (external) — LLM provider abstraction, streaming, retries
- hawk never talks to LLM APIs directly; eyrie handles all provider communication

### Intelligence
- `repomap/` — Code intelligence (PageRank, BM25, TF-IDF for file relevance)
- `memory/` — yaad bridge for persistent cross-session memory
- `planner/` — Multi-step planning with decomposition

### Infrastructure
- `session/` — Persistence (JSONL, WAL, checkpoints, branch tracking)
- `daemon/` — Background HTTP/SSE server (JSON + streaming)
- `sandbox/` — Command isolation (landlock, seccomp, docker, chroot)
- `permissions/` — User approval system with auto-learning
- `hooks/` — Event-driven plugin system
- `mcp/` — Model Context Protocol client with buffered I/O

### Multi-Agent
- `mission/` — Multi-agent orchestration on parallel git branches
- `parallel/` — Worktree-based parallel execution
- `agents/` — Custom persona loader (markdown + YAML frontmatter)

### Resilience
- `circuit/` — Circuit breaker pattern
- `ratelimit/` — Token bucket rate limiting
- `retry/` — Exponential backoff
- `health/` — Diagnostics and self-checks

## Key Patterns

### Error Handling
- `hawkerr/` package defines BridgeError for cross-package errors
- All errors use `%w` wrapping for proper error chain inspection
- Panic recovery in `cmd/errors.go` with triple-nested defer/recover

### Configuration
- Agent personas: markdown files with YAML frontmatter
- Settings: global (~/.hawk/) + project-level (.hawk/)
- Budget tracking per session (tokens + cost)

### Testing
- Race detector required: `go test -race`
- Integration tests: `integration_test.go` at root, engine/, tool/
- No mocking of databases — real SQLite in tests
- Tests timeout: 120s

## Development

### Build
```bash
go build .                    # Build hawk binary
go test -race ./...           # Run all tests
```

### Dependencies
hawk depends on 5 ecosystem modules. For local dev, use go.work:
```
replace (
  github.com/GrayCodeAI/eyrie => ../eyrie
  github.com/GrayCodeAI/tok => ../tok
  github.com/GrayCodeAI/yaad => ../yaad
  github.com/GrayCodeAI/inspect => ../inspect
  github.com/GrayCodeAI/sight => ../sight
)
```

### CI
- Tests clone dependencies at HEAD and create go.work
- golangci-lint with errcheck, staticcheck, gosec, unused, misspell
- Multi-platform builds (linux/darwin/windows × amd64/arm64)

## Daemon
- Binds to 127.0.0.1:4590 (localhost only)
- Endpoints: /v1/chat, /v1/health
- Supports SSE streaming

## Sandbox (Linux)
- Landlock: filesystem access restrictions
- seccomp-bpf: blocks 21 dangerous syscalls
- Fallback: no-op on non-Linux (sandbox_other.go)
