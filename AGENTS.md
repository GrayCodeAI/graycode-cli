# Hawk - AI Coding Agent

## Product direction: solo-first

hawk targets **individual developers** on their own machines — not teams or enterprises (yet). When choosing defaults, docs, and new features, optimize for:

- One person, local config (`~/.hawk/`), OS keychain credentials
- `hawk solo` / `/config` as the onboarding path
- Local yaad memory, optional Docker isolation
- Defer: SSO, org admin, shared memory, fleet sandbox

See `docs/SOLO-FIRST.md`, `docs/SECURITY-SOLO.md`, and `plans/SOLO-DEVELOPER-PATH.md`.

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

## Behavioral Guidelines

Derived from [Andrej Karpathy's observations](https://x.com/karpathy/status/2015883857489522876) on LLM coding pitfalls. The hawk CLI loads the same content from `internal/prompts/templates/practices.md`; Cursor loads `.cursor/rules/behavioral-guidelines.mdc`.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

### Think Before Coding
- Don't assume. If uncertain, ask.
- If multiple interpretations exist, present them — don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.
- State assumptions explicitly before implementing.

### Simplicity First
- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.
- Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

### Surgical Changes
- Touch only what you must. Clean up only your own mess.
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it — don't delete it.
- Remove imports/variables/functions that YOUR changes made unused. Don't remove pre-existing dead code unless asked.
- Every changed line should trace directly to the user's request.

### Goal-Driven Execution
- Transform vague tasks into verifiable goals before implementing.

| Instead of... | Transform to... |
|--------------|-----------------|
| "Add validation" | "Write tests for invalid inputs, then make them pass" |
| "Fix the bug" | "Write a test that reproduces it, then make it pass" |
| "Refactor X" | "Ensure tests pass before and after" |

- For multi-step work, state a brief plan: `[Step] → verify: [check]`.
- Strong success criteria let the agent loop independently. Weak criteria ("make it work") require constant clarification.

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.

## Git commits

- Never add `Co-authored-by:` or `Co-Authored-By:` trailers to commit messages, PR descriptions, or squash-merge bodies.
- Commits should list only the human author.
- Do not attribute Cursor, hawk, Claude, Copilot, or any AI tool as a co-author.
- Enable the repo hook to strip accidental trailers: `git config core.hooksPath .githooks`

## Development

### Build
```bash
go build .                    # Build hawk binary
go test -race ./...           # Run all tests
./scripts/smoke-hawk.sh       # Quick build + doctor + ecosystem tests
```

Ecosystem integration (eyrie · yaad · tok): see [docs/ecosystem-message-flow.md](docs/ecosystem-message-flow.md).

### Dependencies

**GrayCodeAI repos wired into hawk**

| Module | In `go.mod` | In-repo checkout | Used from |
|--------|-------------|------------------|-----------|
| eyrie | ✓ | external checkout **`./external/eyrie`** + **`go.work`** | Provider client, setup, streaming |
| sight | ✓ | external checkout **`./external/sight`** + **`go.work`** | `hawk sight`, `internal/bridge/sight` |
| inspect | ✓ | external checkout **`./external/inspect`** + **`go.work`** | Inspect bridges |
| tok | ✓ | external checkout **`./external/tok`** + **`go.work`** | Tokenizer pipeline |
| yaad | ✓ | external checkout **`./external/yaad`** + **`go.work`** | Memory bridge |
| trace | — | external checkout **`./external/trace`**; separate **`trace` CLI** | Session capture only; not a Go import |

**External checkouts** (hawk + ecosystem repos):

```bash
# herm-style layout: clone ecosystem repos under hawk/external, then:
git submodule update --init external/eyrie external/inspect external/sight external/tok external/trace external/yaad
cd hawk && go work sync
```

Committed **`go.work`** lists `.` and the **`./external/*`** ecosystem checkouts. **`go.mod`** keeps normal module requirements for imported modules.

**`shared/types`** forwards **`internal/types`** for **sight**, **inspect**, **tok**, and friends so they never import hawk `internal/` directly.

For extra sibling clones on one machine, use a **personal** parent **`go.work`** or temporary **`replace`** — do not commit those **`replace`** lines into **`go.mod`**.

### CI

- CI clones ecosystem repos to **`./external/*`** via **`.github/actions/checkout-eyrie`**
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

Active branch: **`feature/secure-credentials-sandbox`** (hawk + external eyrie).

| Concern | Where |
|---------|--------|
| First-run `/config`, setup guards | `internal/config/setup_status.go`, `cmd/chat.go` |
| Keychain + `PersistAPIKey` / `RemoveStoredCredential` | `internal/config/credentials_store.go`, eyrie `credentials/` |
| Remove stored key (TUI) | `/config key remove` → `cmd/chat_config_remove.go` |
| Remove stored key (CLI) | `hawk credentials remove` → `cmd/credentials.go` |
| Catalog discover + routing only on disk | `internal/config/eyrie_apply.go`, eyrie `setup/apply_credentials.go` |
| Catalog empty / refresh hints | `internal/config/catalog_health.go`, `catalog_startup.go` |
| No API keys in `provider.json` | eyrie `SanitizeDeploymentConfigForDisk`, hawk `MigrateProviderSecrets` |
| Verification tests | `internal/config/milestone_verify_test.go`, `./scripts/verify-milestone.sh` |
| Solo developer readiness | `hawk solo`, `/solo`, `internal/config/solo_path.go`, `./scripts/verify-solo-path.sh` |
| Plan + phase status | `plans/MILESTONE-api-key-model-sandbox.md`, `plans/SOLO-DEVELOPER-PATH.md` |

**Not in this milestone:** conversation DAG as source of truth.

**`/sandbox` vs Docker:** `/sandbox` toggles **approval mode** in the TUI. **Docker container mode** is the default for bash (`shouldUseContainer`); use `--no-container` for host execution.
