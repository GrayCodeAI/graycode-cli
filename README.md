<h1 align="center">AI Coding Agent for Your Terminal</h1>

<p align="center">
  AI coding agent for your terminal — built for <strong>developers</strong>, not teams or enterprises (yet).
</p>

<p align="center">
  <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License"></a>
  <a href="https://github.com/GrayCodeAI/graycode-cli/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/GrayCodeAI/graycode-cli/ci.yml?style=flat-square&label=tests" alt="CI"></a>
  <a href="https://github.com/GrayCodeAI/graycode-cli/releases"><img src="https://img.shields.io/github/v/release/GrayCodeAI/graycode-cli?style=flat-square&label=release&color=green" alt="Release"></a>
  <a href="https://pkg.go.dev/github.com/GrayCodeAI/graycode-cli"><img src="https://img.shields.io/badge/godoc-reference-00ADD8?style=flat-square&logo=go" alt="GoDoc"></a>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> ·
  <a href="#features">Features</a> ·
  <a href="#usage">Usage</a> ·
  <a href="#skills">Skills</a> ·
  <a href="#tools">Tools</a> ·
  <a href="#architecture">Architecture</a> ·
  <a href="#contributing">Contributing</a>
</p>

---

## Why graycode

graycode is an AI-powered coding agent that lives in your terminal. It reads your codebase, writes and edits files, runs tests, and manages git — all through natural language. Unlike IDE-bound tools, graycode works over SSH, in containers, and on any machine with a shell.

**Developer path:** one machine, keychain credentials, local memory. Run `graycode path` to check readiness.

- **Model-agnostic** — supports 28 first-class providers through [eyrie](https://github.com/GrayCodeAI/graycode-router), including Anthropic, OpenAI, Gemini, Fireworks AI, Concentrate AI (pay-as-you-go), DeepSeek, and Ollama
- **Zero CGO** — single static binary, cross-compiled for linux/darwin/windows on amd64/arm64
- **Privacy-first** — your code never leaves your machine except to the LLM API you choose
- **Docker-only execution** — agent commands run in an isolated container and
  fail closed when Docker is unavailable
- **Extensible** — 40+ built-in tools, MCP server support, community skill registry

## Status

**Graycode is in active development.** Contributor source builds are the primary path today while we keep hardening the product in the open. Tagged releases and install assets may exist for validation, but they are not the recommended first path yet.

Follow [GrayCode](https://github.com/GrayCodeAI) for progress. When Graycode is ready to try, we will announce it on [graycodeai.com](https://graycodeai.com/changelog).

## Install (60 seconds)

Pick one — all install the same `graycode` binary (versioned into `~/.graycode/bin`, symlinked as `graycode`):

```bash
# 1. Script (any shell, verifies checksum; cosign signature when available)
curl -fsSL https://raw.githubusercontent.com/GrayCodeAI/graycode-cli/main/install.sh | sh

# 2. Homebrew (macOS / Linuxbrew) — after the next tagged release
brew install graycodeai/tap/graycode

# 3. npm (wraps the same release binaries)
npm install -g @graycodeai/graycode
```

If `~/.graycode/bin` is not on your `PATH`, add it to your shell profile.

Then:

```bash
graycode            # interactive REPL (/config on first run: API key + model)
graycode path       # verify readiness
```

## Quick Start (contributors — from source)

```bash
git clone https://github.com/GrayCodeAI/graycode-cli && cd graycode-cli
make setup   # generates go.work referencing sibling support repos in the graycode-eco workspace
go build -o graycode ./cmd/graycode
./graycode

# First run — paste API key in /config (stored in macOS Keychain / Linux keyring)
# Verify readiness
./graycode path
```

Docker is required for agent command execution. Start the Docker daemon before
launching Graycode; there is no host-execution fallback. Graycode automatically uses
the versioned public `graycodeai/graycode-sandbox` image. When the image is not
local, Graycode pulls it anonymously; if the registry is unavailable, Graycode builds
the bundled sandbox image locally through Docker.

See [docs/SECURITY-DEVELOPER.md](docs/SECURITY-DEVELOPER.md) for the credential model. Do not put API keys in shell env or `.env` for graycode.

Optional for contributors:

```bash
go install github.com/GrayCodeAI/graycode-cli/cmd/graycode@latest
```

## Features

### Interactive Terminal UI

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) for a smooth, keyboard-driven experience with vim-style keybindings.

### 40+ Built-in Tools

| Category | Tools |
|---|---|
| **Files** | `Read`, `Write`, `Edit`, `LS`, `Glob`, `Grep` |
| **Shell** | `Bash`, `PowerShell`, `CronCreate`, `CronDelete` |
| **Git** | `GitCommit`, `SmartCommit`, `EnterWorktree`, `ExitWorktree` |
| **Web** | `WebFetch`, `WebSearch`, `CodeSearch` |
| **Tasks** | `TodoWrite`, `TaskCreate`, `TaskList`, `TaskUpdate` |
| **Code** | `LSP` diagnostics, `CodeSearch`, `NotebookEdit`, `SQL` (read-only DB exploration) |
| **MCP** | `ListMcpResources`, `ReadMcpResource` |

### Portable Execution Graph

Export the latest or a selected Graycode session as validated graph nodes, edges,
and lifecycle events:

```bash
graycode graph export
graycode graph export <session-id>
graycode graph export <session-id> --swift-checkpoint abc123def456
graycode graph export --mission-dir /path/to/mission

# Explicitly privacy-normalize and sync the graph for a connected cloud project
graycode cloud graph sync <session-id>
graycode cloud graph sync --mission-dir /path/to/mission
```

The export contains metadata and hashes, not prompts, tool arguments/results,
policy reasons, verification evidence, or runtime output. Swift remains
available separately as `graycode swift graph export`. Persisted chat sessions
automatically append privacy-safe permission, enabled approval-gate, and
`VerifyPlanExecution` summaries for subsequent graph exports. Harrier memory
subgraphs and Graycode code-index chunks actually selected for inference are also
journaled as metadata-only knowledge nodes and linked to the session. Merlin's
observed bridge path similarly journals bounded, metadata-only report/finding
quality subgraphs. Kestrel exposes the same observed bridge boundary for
metadata-only code-review quality subgraphs. Mission runs also persist a
portable `mission-graph.json`; the mission form is validated and synchronized
explicitly with the `--mission-dir` variants above.

### Multi-Agent Mission Mode (optional)

For larger tasks, decompose work into parallel feature branches (power-user / future team workflows):

```bash
graycode mission "Add auth, rate limiting, and logging"
```

Each sub-agent runs in its own git worktree with full autonomy.

### Community Skills

Discover and install modular instruction packages for specialized workflows:

```bash
graycode skills search api        # Search community registry
graycode skills install go-review # Install from GitHub
graycode skills audit             # Security scan installed skills
```

### Permission Center

graycode exposes two independent chat command centers — trust tier and the
spec-driven workflow gate — rather than one merged permission mode:

```text
/autonomy
/autonomy tier <scout|builder|operator|autonomous>
/autonomy sandbox <strict|workspace|off>
/autonomy dry-run <on|off>
/autonomy allow <rule>
/autonomy deny <rule>
/autonomy rules
/autonomy reset
/autonomy save [project|global]

/spec
/spec [what to build]
/spec status
/spec reset
```

The model is:

- `Tier` controls autonomy (bare `/autonomy` opens a picker for this):
  - `Always Ask` — prompts for permission on every tool call
  - `Scout`
  - `Builder`
  - `Operator`
  - `Autonomous`
- `Sandbox` controls the execution boundary:
  - `strict`
  - `workspace`
  - `off`
- `Dry-run` is a kill switch: denies every tool call unconditionally,
  regardless of tier or spec stage.
- `Rules` control explicit allow/deny exceptions.
- `Spec` is a separate, independent workflow gate (bare `/spec` opens a
  picker): starting it walks the model through `Specify → Plan → Tasks`,
  writing real files to `.graycode/specs/<slug>/`, and blocks Write/Edit/Bash
  until you approve moving to implementation — at **any** trust tier,
  including Autonomous.

`/autonomy` and `/spec` are the main control surfaces for normal chat usage.
Older merged permission-mode chat commands have been removed in favor of
these two independent flows.

### MCP & LSP Support

Connect external tools via [Model Context Protocol](https://modelcontextprotocol.io/) and get code intelligence through Language Server Protocol. MCP also supports a WebSocket transport (opt-in) in addition to stdio/HTTP.

### Watch Mode (AI-comment loop)

`graycode --watch` watches your tree for `AI!` (do it now) and `AI?` (answer my question) comments and acts on them automatically — leave a directive in code, save, and graycode responds. Off by default; enabled via the `--watch` flag.

### CI / GitHub Action

A bundled GitHub Action (`.github/actions/graycode`) runs graycode in your pipeline: interactive mode on `@graycode` mentions in issue/PR comments, automation mode on labeled issues/PRs, and skill dispatch when a prompt begins with `/` (e.g. `/code-review`).

### Messaging Gateways (opt-in)

The daemon exposes Telegram, Discord, and Slack gateways so you can chat with graycode from your messaging app. Disabled by default; enabled per-channel via daemon config.

### AST Repo-Map & Codebase Analysis

An AST-based repository map (`internal/context/repomap`) gives the model a structural overview of your code. On first run, graycode can auto-analyze the codebase to seed context (default-off, opt-in).

### Auto-Lint / Auto-Fix Cycle

After edits, graycode can run the matching linter and iterate on fixes (bounded retries) before handing back. Opt-in; preserves existing behavior when disabled.

### Image / Multimodal Context

Feed screenshots and images into the conversation for vision-capable models (`internal/engine/vision.go`).

### Plan & Explore Sub-Agents

Read-only sub-agent modes: `plan` decomposes a task into steps, `explore` investigates the codebase with a configurable thoroughness budget (`quick` / `medium` / `very-thorough`).

### Conventional-Commit Generation

`SmartCommit` and the diff summarizer generate Conventional Commit messages from your staged changes.

### Durable Workflows & Approval Gates

LangGraph-style durable workflows with named, resumable step checkpoints and optional human-in-the-loop approval gates that persist the gate decision.

### Structured Output

Request JSON-Schema-constrained responses; results are validated against the schema and retried once on mismatch.

### YAML Agents & Tasks

Define personas and eval tasks in YAML (in addition to markdown personas), including per-agent display `color` and lifecycle `hooks`.

### IT-Managed Policy Tier

An optional, non-excludable org-policy rule tier (highest precedence) with HTML-comment stripping of rule files for IT-managed deployments.

### Adopted Capabilities (env-gated)

Features adopted from open-source agent projects. All are off by default unless explicitly enabled; see each section for details.

| Feature | Flag / Command | What it does |
|---|---|---|
| Best-of-N fan-out | `graycode exec --fanout N` | Run the same prompt in N isolated worktrees, compare, merge winner |
| Completion notifications | `GRAYCODE_NOTIFY_WEBHOOK_URL` / `GRAYCODE_NOTIFY_TELEGRAM_TOKEN` + `_CHAT_ID` | Webhook or Telegram ping when a run finishes |
| Incremental system-context | `GRAYCODE_INCREMENTAL_CONTEXT=1` | Reconcile dynamic sections instead of rebuilding the prompt |
| Tool-catalog shrink | `GRAYCODE_TOOL_SHRINK=1` | Compress the tool catalog sent on every request |
| Compaction segments | `GRAYCODE_COMPACTION_SEGMENT_DETAIL=verbose\|balanced\|minimal\|none` | Persist verbatim compacted turns to disk |
| Skill curator | `graycode skills curator status/run/pin/unpin/archive` + `GRAYCODE_SKILL_CURATOR=1` | Auto-archive cold agent-created skills (recoverable) |
| Structural code match | `CodeMatch` tool | Tree-sitter query search over Go/Python/TS/TSX |
| Composable toolsets | `graycode toolset [name]` + `Toolset` tool | Named tool groups (research, dev, ops, full_stack) |
| App verification | `AppVerify` tool | Boot-smoke check with readiness polling and evidence artifacts |
| Media generation | `GenerateMedia` tool | Image/video generation with local persistence. Backend via `tool.SetMediaEngine`; an OpenAI-compatible client ships in `eyrie/client` (`ImageClient`), wired by the host (boundary-guarded — graycode routes through the eyrie facade) |
| Voice transcription | Telegram voice notes + `stt` package | Transcribe Telegram voice/audio into the prompt. Backend via `stt.SetTranscriber`; an OpenAI-compatible client ships in `eyrie/client` (`AudioClient`), wired by the host |
| Git-tree file snapshots | `internal/gitsnapshot` | Content-addressed tree capture/diff/preview/restore |
| Turn-boundary rewind | `internal/filestate` | Per-prompt before/after snapshots with durable store |
| Continual harness | `internal/intelligence/harness` | Versioned, evidence-backed refinement of supplemental prompts/memories/skills/subagents with rollback |
| Bounded autonomous budgets | `internal/engine` (`AutonomousBudget`) | Track turns/tokens/time/continuations; report why a run stopped (budget vs gate-passed vs error) |
| Agent family messaging | `internal/multiagent` (`FamilyMessenger`) | Direct parent/sibling/child messages with pending caps + rate limits |
| Path reservations | `internal/multiagent` ledger | Detect overlapping-file changes between parallel branches |
| Live agent status | `GET /v1/agent/status` (daemon) | Machine-readable working/idle/stale per session |
| X/Twitter search | `SearchX` tool | Live X search by forwarding a query to an xAI endpoint with server-side search; returns a cited summary. Requires `XAI_API_KEY` (or `GROK_API_KEY`) |
| Desktop computer-use | `ComputerUse` tool | snapshot/click/type/scroll/press/screenshot via a pluggable `tool.SetComputerBackend` seam (host wires a native macOS accessibility backend) |
| Token-cheaper file views | `Read` tool `--minify` | Read-only, comment-stripped, whitespace-dense file view (Go via `go/parser`; other languages string-aware; never touches disk) — fewer tokens per read |
| Classified provider hints | `internal/errhint` | Buckets provider errors (Auth/RateLimit/Connectivity/ModelNotFound/ContextOverflow) into a one-line fixable next step. Wired into TUI error rows (`friendlyErrorMessage`) and `graycode exec` CLI errors |
| Atomic install transactions | `internal/installtxn` | Cross-process staged install/remove with rollback. Wired into skill install (atomic `SKILL.md` publish) |
| Stale-lock reclaim | `internal/lockutil` | Race-correct atomic reclaim of O_EXCL lock files with live-restore (ready for O_EXCL lock sites) |
| Test command discovery | `internal/testrunner` | Auto-detect test/verify commands (Go/npm/bun/pnpm/yarn/pytest/cargo) and parse runner output into structured results. Wired into `graycode verify` |
| Circuit breaker | `internal/circuitbreaker` | Closed/open/half-open retry-storm protection with cooldown. Wired into auto-compaction (cooldown + half-open auto-retry) |
| Smart turn routing | `internal/smartrouting` | Deterministic simple/strong turn classifier with fail-toward-strong safety. Wired into per-turn model selection (`settings.smart_routing`) |
| Conversation arc | `internal/conversationarc` | Durable sidecar memory of goals/decisions/milestones/phase with a byte-stable summary. Wired into sessions (loaded on open, saved on close, injected into the system prompt) |
| Relevance pruning | `internal/relevanceprune` | Token-budgeted context pruning preserving recent turns/tool calls/errors. Wired into compaction as a `relevance` strategy |
| Tool-result clearing | `internal/engine` (`ClearOldToolResults`) | Two-tier context management: at 80% of the context window, stale tool-result content is replaced with `[output cleared]` placeholders (tool_use kept intact) before compacting — a gentler tier below compaction |
| Approval pause timing | `internal/permissions` | Approval requests record decision timestamp + human deliberation duration (`DecisionAt`/`PauseDuration`) for approval-latency observability |
| Graceful exhaustion | `internal/engine` (`SynthesisForExhaustion`) | When turn/token/time limits hit, one final tools-disabled LLM call synthesizes a coherent completion (accomplished/remaining/next steps) instead of a bare stop line. Opt-in via `GRAYCODE_GRACEFUL_EXHAUSTION=1` |
| Deterministic replay cache | `internal/replaycache` (`GRAYCODE_REPLAY_CACHE_DIR`) | Disk-persisted SHA-256-keyed cache of completions; identical requests replay stored responses for reproducible regression runs |

## Usage

### Interactive Mode

```bash
graycode                              # Start REPL
graycode -r abc123                    # Resume session
graycode -c                           # Continue latest session
graycode --provider openai --model gpt-4o  # Override provider
```

### Permission Examples

```bash
# Inside the TUI
/autonomy
/autonomy tier builder
/autonomy sandbox workspace
/autonomy allow Bash(git:*)
/autonomy deny Bash(rm -rf *)
/autonomy save project

/spec add dark mode support
```

### Non-Interactive Mode

```bash
graycode -p "explain this repo"                    # Print response, exit
graycode -p "fix tests" --allowed-tools "Bash(go test:*) Edit Read"
graycode -p "review this repo" --permission-mode plan --sandbox workspace
graycode exec "refactor auth module"               # Full engine, non-interactive
graycode exec --auto full "add error handling"     # Full autonomy
graycode exec --worktree "add rate limiting"       # Isolated branch
graycode exec --agent reviewer "review last commit" # Custom persona
```

### Diagnostics & ecosystem

```bash
graycode path                   # Developer path readiness (setup + security + sandbox)
graycode doctor                  # Full health report (eyrie + harrier + shrike panel)
graycode ecosystem               # Ecosystem panel only
graycode harrier                    # Persistent memory graph
graycode harrier search <query>     # Search harrier memories
graycode preflight               # Quick ready-to-chat check
make path                    # Developer path verification
make smoke                   # Build + quick verification script
```

See [docs/SECURITY-DEVELOPER.md](docs/SECURITY-DEVELOPER.md).

See [docs/ecosystem-message-flow.md](docs/ecosystem-message-flow.md) for how eyrie, harrier, and shrike connect during a chat session, and [docs/ECOSYSTEM-WIRING.md](docs/ECOSYSTEM-WIRING.md) for the current-to-proposed architecture and repository boundaries.

In the TUI: `/path`, `/ecosystem`, `/harrier`, `/harrier search <query>`, `/memory` (AGENTS.md).

### Daemon Mode

```bash
graycode daemon start              # Background HTTP server on port 4590
graycode daemon status             # Check if running
graycode daemon stop               # Graceful shutdown
```

Endpoints: `GET /v1/health`, `GET /v1/ready` (dependency-aware readiness), `POST /v1/chat` (JSON or SSE streaming)

### Mission Mode

```bash
graycode mission "Add auth, rate limiting, and logging"
graycode mission --workers 6 "Refactor into microservices"
graycode mission --dry-run "What would this decompose into?"
graycode mission --from-tasks                 # Execute validated dependency waves
```

## Providers

graycode works with any LLM provider. **Developer path:** paste keys in `/config` (stored in OS keychain) — not shell env or `.env`. Use `graycode credentials status` to verify.

| Provider | ID | Key (via `/config`) |
|---|---|---|
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY` |
| OpenAI | `openai` | `OPENAI_API_KEY` |
| Google Gemini | `gemini` | `GEMINI_API_KEY` |
| OpenRouter | `openrouter` | `OPENROUTER_API_KEY` |
| Fireworks AI | `fireworks` | `FIREWORKS_API_KEY` |
| xAI (Grok) | `grok` | `XAI_API_KEY` |
| Z.AI | `z-ai` | `ZAI_API_KEY` |
| CanopyWave | `canopywave` | `CANOPYWAVE_API_KEY` |
| OpenCode Go | `opencodego` | `OPENCODEGO_API_KEY` |
| Kimi (Moonshot) | `kimi` | `MOONSHOT_API_KEY` |
| Xiaomi (MiMo) Pay-as-you-go | `xiaomi_mimo_payg` | `XIAOMI_MIMO_PAYG_API_KEY` |
| Xiaomi (MiMo) Token Plan | `xiaomi_mimo_token_plan` | `XIAOMI_MIMO_TOKEN_PLAN_API_KEY` (pick region in `/config`) |
| Ollama (local) | `ollama` | `OLLAMA_BASE_URL` (no API key) |

Provider routing, model resolution, and retries are handled by [eyrie](https://github.com/GrayCodeAI/graycode-router).
For deployment-aware routing, set `"deployment_routing": true` in `.graycode/settings.json`
or export `GRAYCODE_DEPLOYMENT_ROUTING=true`. Graycode will route canonical model IDs through
Eyrie's deployment catalog, so new models can be exposed by refreshing the catalog
instead of changing Graycode. In chat, run `/refresh-model-catalog` to fetch the latest
deployment-aware catalog into `~/.eyrie/model_catalog.json`.

## Architecture

graycode is built in Go with a modular, layered architecture:

```
graycode/
├── bin/                    # Built binaries (graycode, graycode_bin)
├── cmd/                    # CLI entry point (Cobra + Bubble Tea TUI)
├── internal/
│   ├── engine/             # Agent loop, compaction, self-improvement
│   │   └── lifecycle/      # Self-improvement loop, limits tracking
│   ├── tool/               # 40+ built-in tools with safety layer
│   │   └── codegen_builtins.go  # Code generation templates
│   ├── config/             # Settings, budget tracking, agent personas
│   ├── session/            # Persistence (JSONL, WAL, checkpoints)
│   ├── api/                # HTTP API server
│   ├── daemon/             # Background HTTP/SSE server
│   ├── sandbox/            # Command isolation (landlock, seccomp, docker)
│   ├── permissions/        # User approval system with auto-learning
│   ├── hooks/              # Event-driven plugin system
│   ├── mcp/                # Model Context Protocol client
│   ├── intelligence/       # Code intelligence (repomap, memory, planner)
│   ├── multiagent/         # Mission orchestration, parallel execution
│   ├── observability/      # Analytics, metrics, logging, tracing
│   ├── resilience/         # Circuit breaker, rate limiting, retries
│   ├── feature/            # eval, fingerprint, voice, IDE integration
│   ├── bridge/             # External bridges (kestrel, merlin, sessioncapture)
│   ├── provider/           # Provider routing
│   └── system/             # Bus, cron, retention, shutdown
├── docs/                   # Architecture, security, integration docs
└── testdata/               # Test fixtures

Ecosystem sibling repos (independent Git repos in the `graycode-eco` parent
folder):
├── eyrie/              # LLM provider runtime
```

### Ecosystem

graycode is the main CLI/product and integrates these GrayCodeAI repositories in
three runtime layers plus optional tooling/platform services:

- **Primary product:** **graycode** is the only end-user product surface in this ecosystem.
- **Support engines mounted by Graycode:** **eyrie**, **harrier**, **shrike**, **swift**, **kestrel**, **merlin**. Graycode imports or shells into these engines behind its own command surface.
- **Shared foundations:** **falcon**
  provides shared MCP server scaffolding.
- **API consumers/extensions:** **graycode-skills** provides Graycode skills
  installed on demand (`graycode skills install`).
- **Tooling/platform:** **owl** visualizes the generated ecosystem graph;
  **graycode-platform** contains the optional web/BFF/Graycode Cloud plane and is
  outside the Graycode Go runtime graph.

Local development uses:

- **`go.mod` modules:** pinned requirements for the support engines
- **Workspace + `go.work`:** sibling support repos are cloned in the `graycode-eco` workspace (as `../<repo>`); `go.work` resolves the module paths to those local checkouts
- **Module-mode builds:** standalone / Docker builds resolve the pinned `go.mod` versions from the module proxy (no workspace)

Cross-repo contracts now live in `internal/contracts` (vendored from the
removed `github.com/GrayCodeAI/eagle` module) so support repos do not depend
on Graycode internals. The old `graycode/shared/types` path has been removed;
external consumers should vendor the needed DTOs from `internal/contracts`
until a published contracts module exists.

Current contract packages (`internal/contracts/`):

- `types` — severity, findings
- `graph` — portable graph vocabulary: nodes, edges, events, provenance
- `agent` — typed subagent spawn DTOs and hook events
- `policy` — permission and policy verdict contracts
- `contracts/review` — normalized review findings, comments, stats, results
- `contracts/verify` — normalized verification findings, stats, reports
- `events` — tool, trace, and usage events
- `harness` — harness evaluation reports and dimension scores

You may keep a **personal** parent **`go.work`** that lists alternate clones on disk for multi-repo development.

| Component | Repository | Purpose |
|---|---|---|
| **graycode** | This repo | AI coding agent |
| **eyrie** | [GrayCodeAI/graycode-router](https://github.com/GrayCodeAI/graycode-router) | LLM provider runtime |

For the consolidated repo map and the current-vs-proposed architecture diagrams, see [docs/architecture/graycode-current-vs-proposed.md](docs/architecture/graycode-current-vs-proposed.md).
For execution-graph ownership, automatic capture seams, export/sync commands,
and the Swift correlation contract, see
[docs/architecture/execution-graph.md](docs/architecture/execution-graph.md).

## Development

### Prerequisites

- Go 1.26+

### Build & Test

```bash
go build ./cmd/graycode           # Build binary
go test -race ./...           # Run all tests with race detector
make ci                       # Run full CI suite (lint, test, security)
make cover                    # Generate coverage report
```

### Project Structure

graycode follows Go conventions: `cmd/` for entry points, `internal/` for private code, tests alongside source files. See [docs/architecture.md](docs/architecture.md) for details.

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, commit conventions, and the PR process.

Quick start:

1. Fork and create a branch: `git checkout -b feat/short-description`
2. Make changes in small, focused commits
3. Run `make ci` locally
4. Open a pull request

Use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages — release-please uses them for versioning.

## License

MIT — see [LICENSE](LICENSE) for details.

© 2026 GrayCode AI
