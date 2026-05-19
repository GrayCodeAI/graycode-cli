# Hawk - AI Coding Agent

## Architecture

hawk is a model-agnostic AI coding agent with 60+ packages organized around:

### Core Packages
- `cmd/` — CLI entry point (cobra + bubbletea TUI)
- `internal/engine/` — Agent loop, compaction, self-improvement, loop detection
- `internal/tool/` — 40 built-in tools with safety/permission layer
- `internal/config/` — Settings, budget tracking, validation, agent personas

### Provider Layer
- `eyrie` (external) — LLM provider abstraction, streaming, retries
- hawk never talks to LLM APIs directly; eyrie handles all provider communication

### Intelligence
- `internal/intelligence/repomap/` — Code intelligence (PageRank, BM25, TF-IDF for file relevance)
- `internal/intelligence/memory/` — yaad bridge for persistent cross-session memory
- `internal/intelligence/planner/` — Multi-step planning with decomposition

### Infrastructure
- `internal/session/` — Persistence (JSONL, WAL, checkpoints, branch tracking)
- `internal/daemon/` — Background HTTP/SSE server (JSON + streaming)
- `internal/sandbox/` — Command isolation (landlock, seccomp, docker, chroot)
- `internal/permissions/` — User approval system with auto-learning
- `internal/hooks/` — Event-driven plugin system
- `internal/mcp/` — Model Context Protocol client with buffered I/O

### Multi-Agent
- `internal/multiagent/mission/` — Multi-agent orchestration on parallel git branches
- `internal/multiagent/parallel/` — Worktree-based parallel execution
- `internal/multiagent/agents/` — Custom persona loader (markdown + YAML frontmatter)

### Resilience
- `internal/resilience/circuit/` — Circuit breaker pattern
- `internal/resilience/ratelimit/` — Token bucket rate limiting
- `internal/resilience/retry/` — Exponential backoff
- `internal/resilience/health/` — Diagnostics and self-checks

## Key Patterns

### Error Handling
- `internal/hawkerr/` package defines BridgeError for cross-package errors
- All errors use `%w` wrapping for proper error chain inspection
- Panic recovery in `cmd/errors.go` with triple-nested defer/recover

### Configuration
- Agent personas: markdown files with YAML frontmatter
- Settings: global (~/.hawk/) + project-level (.hawk/)
- Budget tracking per session (tokens + cost)

### Testing
- Race detector required: `go test -race`
- Integration tests: `integration_test.go` at root
- No mocking of databases — real SQLite in tests
- Tests timeout: 120s

## Development

### Build
```bash
go build .                    # Build hawk binary
go test -race ./...           # Run all tests
```

### Dependencies

**GrayCodeAI repos wired into hawk**

| Module | In `go.mod` | In-repo checkout | Used from |
|--------|-------------|------------------|-----------|
| eyrie | ✓ | sibling **`../eyrie`** + **`go.work`** + **`replace` in `go.mod`** | Provider client, setup, streaming |
| sight | ✓ | proxy (optional local `replace`) | `hawk sight`, `internal/bridge/sight` |
| inspect | ✓ | proxy | Inspect bridges |
| tok | ✓ | proxy | Tokenizer pipeline |
| yaad | ✓ | proxy | Memory bridge |
| trace | — | separate **`trace` CLI** | Session capture only; not a Go import |

**Eyrie sibling checkout** (hawk + eyrie):

```bash
# hawk-eco layout: clone eyrie next to hawk, then:
cd hawk && go work sync
```

Committed **`go.work`** lists `.` and **`../eyrie`**. **`go.mod`** includes **`replace github.com/GrayCodeAI/eyrie => ../eyrie`** (CI enforces this path).

**`shared/types`** forwards **`internal/types`** for **sight**, **inspect**, **tok**, and friends so they never import hawk `internal/` directly.

For sibling clones on one machine, use a **personal** parent **`go.work`** or temporary **`replace`** — do not commit those **`replace`** lines into **`go.mod`**.

### CI

- CI clones **eyrie** to **`../eyrie`** via **`.github/actions/checkout-eyrie`**
- Module hygiene: **`go work sync`** and **`go build -mod=readonly`** (not `go mod tidy`, which mis-resolves workspace Eyrie)
- golangci-lint with errcheck, staticcheck, gosec, unused, misspell
- Multi-platform builds (linux/darwin/windows × amd64/arm64)

## Daemon
- Binds to 127.0.0.1:4590 (localhost only)
- Endpoints: /v1/chat, /v1/health
- Supports SSE streaming

## Sandbox (Linux)
- Landlock: filesystem access restrictions
- seccomp-bpf: blocks 21 dangerous syscalls
- Fallback: no-op on non-Linux (`internal/sandbox/landlock_other.go`)

## Milestone: API key → model → sandbox

Active branch: **`feature/secure-credentials-sandbox`** (hawk + eyrie sibling).

| Concern | Where |
|---------|--------|
| First-run `/config`, setup guards | `internal/config/setup_status.go`, `cmd/chat.go` |
| Keychain + `PersistAPIKey` | `internal/config/credentials_store.go`, eyrie `credentials/` |
| Catalog discover + routing only on disk | `internal/config/eyrie_apply.go`, eyrie `setup/apply_credentials.go` |
| No API keys in `provider.json` | eyrie `SanitizeDeploymentConfigForDisk`, hawk `MigrateProviderSecrets` |
| Verification tests | `internal/config/milestone_verify_test.go`, `./scripts/verify-milestone.sh` |
| Plan + phase status | `plans/MILESTONE-api-key-model-sandbox.md` |

**Not in this milestone:** conversation DAG as source of truth, langdag Go import.

**`/sandbox` vs Docker:** `/sandbox` toggles **approval mode** in the TUI. **Docker container mode** is the default for bash (`shouldUseContainer`); use `--no-container` for host execution.
