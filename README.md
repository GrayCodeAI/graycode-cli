<h1 align="center">AI Coding Agent for Your Terminal</h1>

<p align="center">
  AI coding agent for your terminal — built for <strong>developers</strong>, not teams or enterprises (yet).
</p>

<p align="center">
  <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License"></a>
  <a href="https://github.com/GrayCodeAI/hawk/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/GrayCodeAI/hawk/ci.yml?style=flat-square&label=tests" alt="CI"></a>
  <a href="https://github.com/GrayCodeAI/hawk/releases"><img src="https://img.shields.io/github/v/release/GrayCodeAI/hawk?style=flat-square&label=release&color=green" alt="Release"></a>
  <a href="https://pkg.go.dev/github.com/GrayCodeAI/hawk"><img src="https://img.shields.io/badge/godoc-reference-00ADD8?style=flat-square&logo=go" alt="GoDoc"></a>
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

## Why hawk

hawk is an AI-powered coding agent that lives in your terminal. It reads your codebase, writes and edits files, runs tests, and manages git — all through natural language. Unlike IDE-bound tools, hawk works over SSH, in containers, and on any machine with a shell.

**Developer path:** one machine, keychain credentials, local memory. Run `hawk path` to check readiness.

- **Model-agnostic** — works with Claude, GPT-4, Gemini, DeepSeek, Ollama, and 75+ models through [eyrie](https://github.com/GrayCodeAI/eyrie)
- **Zero CGO** — single static binary, cross-compiled for linux/darwin/windows on amd64/arm64
- **Privacy-first** — your code never leaves your machine except to the LLM API you choose
- **Extensible** — 40+ built-in tools, MCP server support, community skill registry

## Status

**Hawk is in active development.** Contributor source builds are the primary path today while we keep hardening the product in the open. Tagged releases and install assets may exist for validation, but they are not the recommended first path yet.

Follow [GrayCode](https://github.com/GrayCodeAI) for progress. When Hawk is ready to try, we will announce it on [graycodeai.gateandtech.in](https://graycodeai.gateandtech.in/changelog).

## Quick Start (contributors — from source)

```bash
git clone https://github.com/GrayCodeAI/hawk && cd hawk
make setup   # clones required support repos into external/ and syncs go.work
go build -o hawk ./cmd/hawk
./hawk

# First run — paste API key in /config (stored in macOS Keychain / Linux keyring)
# Verify readiness
./hawk path
```

See [docs/SECURITY-DEVELOPER.md](docs/SECURITY-DEVELOPER.md) for the credential model. Do not put API keys in shell env or `.env` for hawk.

Optional for contributors:

```bash
go install github.com/GrayCodeAI/hawk@latest   # only after Hawk and support repo tags are published
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

### Multi-Agent Mission Mode (optional)

For larger tasks, decompose work into parallel feature branches (power-user / future team workflows):

```bash
hawk mission "Add auth, rate limiting, and logging"
```

Each sub-agent runs in its own git worktree with full autonomy.

### Community Skills

Discover and install modular instruction packages for specialized workflows:

```bash
hawk skills search api        # Search community registry
hawk skills install go-review # Install from GitHub
hawk skills audit             # Security scan installed skills
```

### Permission Center

hawk now exposes one visible permission command center in chat:

```text
/permissions
/permissions tier <scout|builder|operator|autonomous>
/permissions sandbox <strict|workspace|off>
/permissions mode <default|edits|bypass|dontask|plan>
/permissions allow <rule>
/permissions deny <rule>
/permissions rules
/permissions reset
/permissions save [project|global]
```

The model is:

- `Tier` controls autonomy:
  - `Scout`
  - `Builder`
  - `Operator`
  - `Autonomous`
- `Sandbox` controls the execution boundary:
  - `strict`
  - `workspace`
  - `off`
- `Rules` control explicit allow/deny exceptions.

For normal chat usage, `/permissions` is the main control surface. Older
permission chat commands have been removed in favor of this single flow.

### MCP & LSP Support

Connect external tools via [Model Context Protocol](https://modelcontextprotocol.io/) and get code intelligence through Language Server Protocol. MCP also supports a WebSocket transport (opt-in) in addition to stdio/HTTP.

### Watch Mode (AI-comment loop)

`hawk --watch` watches your tree for `AI!` (do it now) and `AI?` (answer my question) comments and acts on them automatically — leave a directive in code, save, and hawk responds. Off by default; enabled via the `--watch` flag.

### CI / GitHub Action

A bundled GitHub Action (`.github/actions/hawk`) runs hawk in your pipeline: interactive mode on `@hawk` mentions in issue/PR comments, automation mode on labeled issues/PRs, and skill dispatch when a prompt begins with `/` (e.g. `/code-review`).

### Messaging Gateways (opt-in)

The daemon exposes Telegram, Discord, and Slack gateways so you can chat with hawk from your messaging app. Disabled by default; enabled per-channel via daemon config.

### AST Repo-Map & Codebase Analysis

An AST-based repository map (`internal/context/repomap`) gives the model a structural overview of your code. On first run, hawk can auto-analyze the codebase to seed context (default-off, opt-in).

### Auto-Lint / Auto-Fix Cycle

After edits, hawk can run the matching linter and iterate on fixes (bounded retries) before handing back. Opt-in; preserves existing behavior when disabled.

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

## Usage

### Interactive Mode

```bash
hawk                              # Start REPL
hawk -r abc123                    # Resume session
hawk -c                           # Continue latest session
hawk --provider openai --model gpt-4o  # Override provider
```

### Permission Examples

```bash
# Inside the TUI
/permissions
/permissions tier builder
/permissions sandbox workspace
/permissions mode plan
/permissions allow Bash(git:*)
/permissions deny Bash(rm -rf *)
/permissions save project
```

### Non-Interactive Mode

```bash
hawk -p "explain this repo"                    # Print response, exit
hawk -p "fix tests" --allowed-tools "Bash(go test:*) Edit Read"
hawk -p "review this repo" --permission-mode plan --sandbox workspace
hawk exec "refactor auth module"               # Full engine, non-interactive
hawk exec --auto full "add error handling"     # Full autonomy
hawk exec --worktree "add rate limiting"       # Isolated branch
hawk exec --agent reviewer "review last commit" # Custom persona
```

### Diagnostics & ecosystem

```bash
hawk path                   # Developer path readiness (setup + security + sandbox)
hawk doctor                  # Full health report (eyrie + yaad + tok panel)
hawk ecosystem               # Ecosystem panel only
hawk yaad                    # Persistent memory graph
hawk yaad search <query>     # Search yaad memories
hawk preflight               # Quick ready-to-chat check
make path                    # Developer path verification
make smoke                   # Build + quick verification script
```

See [docs/SECURITY-DEVELOPER.md](docs/SECURITY-DEVELOPER.md).

See [docs/ecosystem-message-flow.md](docs/ecosystem-message-flow.md) for how eyrie, yaad, and tok connect during a chat session.

In the TUI: `/path`, `/ecosystem`, `/yaad`, `/yaad search <query>`, `/memory` (AGENTS.md).

### Daemon Mode

```bash
hawk daemon start              # Background HTTP server on port 4590
hawk daemon status             # Check if running
hawk daemon stop               # Graceful shutdown
```

Endpoints: `GET /v1/health`, `GET /v1/ready` (dependency-aware readiness), `POST /v1/chat` (JSON or SSE streaming)

### Mission Mode

```bash
hawk mission "Add auth, rate limiting, and logging"
hawk mission --workers 6 "Refactor into microservices"
hawk mission --dry-run "What would this decompose into?"
```

## Providers

hawk works with any LLM provider. **Developer path:** paste keys in `/config` (stored in OS keychain) — not shell env or `.env`. Use `hawk credentials status` to verify.

| Provider | ID | Key (via `/config`) |
|---|---|---|
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY` |
| OpenAI | `openai` | `OPENAI_API_KEY` |
| Google Gemini | `gemini` | `GEMINI_API_KEY` |
| OpenRouter | `openrouter` | `OPENROUTER_API_KEY` |
| xAI (Grok) | `grok` | `XAI_API_KEY` |
| Z.AI | `z-ai` | `ZAI_API_KEY` |
| CanopyWave | `canopywave` | `CANOPYWAVE_API_KEY` |
| OpenCode Go | `opencodego` | `OPENCODEGO_API_KEY` |
| Kimi (Moonshot) | `kimi` | `MOONSHOT_API_KEY` |
| Xiaomi (MiMo) Pay-as-you-go | `xiaomi_mimo_payg` | `XIAOMI_MIMO_PAYG_API_KEY` |
| Xiaomi (MiMo) Token Plan | `xiaomi_mimo_token_plan` | `XIAOMI_MIMO_TOKEN_PLAN_API_KEY` (pick region in `/config`) |
| Ollama (local) | `ollama` | `OLLAMA_BASE_URL` (no API key) |

Provider routing, model resolution, and retries are handled by [eyrie](https://github.com/GrayCodeAI/eyrie).
For deployment-aware routing, set `"deployment_routing": true` in `.hawk/settings.json`
or export `HAWK_DEPLOYMENT_ROUTING=true`. Hawk will route canonical model IDs through
Eyrie's deployment catalog, so new models can be exposed by refreshing the catalog
instead of changing Hawk. In chat, run `/refresh-model-catalog` to fetch the latest
deployment-aware catalog into `~/.eyrie/model_catalog.json`.

## Architecture

hawk is built in Go with a modular, layered architecture:

```
hawk/
├── cmd/                    # CLI entry point (Cobra + Bubble Tea TUI)
├── internal/
│   ├── engine/             # Agent loop, compaction, self-improvement
│   ├── tool/               # 40+ built-in tools with safety layer
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
│   ├── bridge/             # External bridges (sight, inspect, sessioncapture)
│   ├── provider/           # Provider routing
│   └── system/             # Bus, cron, retention, shutdown
├── docs/                   # Architecture, security, integration docs
└── testdata/               # Test fixtures
```

### Ecosystem

hawk integrates these GrayCodeAI repos in three layers:

- **Primary product:** **hawk** is the only end-user product surface in this ecosystem.
- **Support engines mounted by Hawk:** **eyrie**, **yaad**, **tok**, **trace**, **sight**, **inspect**. Hawk imports or shells into these engines behind its own command surface.
- **Shared foundation:** **hawk-core-contracts** holds the neutral cross-repo types that keep the engines independent from Hawk internals.

Local development uses:

- **`go.mod` modules:** pinned requirements for the support engines and `hawk-core-contracts`
- **External checkout + `go.work`:** clone support repos under `external/<repo>`; `go.work` maps the module paths to those local checkouts
- **Submodules in this repo:** the same external layout is pinned under `external/` for reproducible CI and multi-repo work

Cross-repo contracts now live in **`github.com/GrayCodeAI/hawk-core-contracts`** so support repos do not depend on Hawk internals. The old `hawk/shared/types` path has been removed; use `hawk-core-contracts/types` for shared severity and finding contracts.

Current contract packages:

- `hawk-core-contracts/types` — severity, findings
- `hawk-core-contracts/review` — normalized review findings, comments, stats, results
- `hawk-core-contracts/verify` — normalized verification findings, stats, reports
- `hawk-core-contracts/tools` — tool call/result contracts
- `hawk-core-contracts/events` — normalized tool/trace events
- `hawk-core-contracts/policy` — permission and policy verdict contracts

You may keep a **personal** parent **`go.work`** that lists alternate clones on disk for multi-repo development.

| Component | Repository | Purpose |
|---|---|---|
| **hawk** | This repo | AI coding agent |
| **eyrie** | [GrayCodeAI/eyrie](https://github.com/GrayCodeAI/eyrie) | LLM provider runtime |
| **sight** | [GrayCodeAI/sight](https://github.com/GrayCodeAI/sight) | Diff-based code review (`hawk sight`) |
| **inspect** | [GrayCodeAI/inspect](https://github.com/GrayCodeAI/inspect) | Site audit library |
| **tok** | [GrayCodeAI/tok](https://github.com/GrayCodeAI/tok) | Tokenizer & compression |
| **yaad** | [GrayCodeAI/yaad](https://github.com/GrayCodeAI/yaad) | Graph-based memory |
| **trace** | [GrayCodeAI/trace](https://github.com/GrayCodeAI/trace) | Session capture and replay engine mounted as `hawk trace ...` |
| **hawk-core-contracts** | [GrayCodeAI/hawk-core-contracts](https://github.com/GrayCodeAI/hawk-core-contracts) | Shared contracts and neutral cross-repo vocabulary |

For the consolidated repo map and the current-vs-proposed architecture diagrams, see [docs/architecture/hawk-current-vs-proposed.md](docs/architecture/hawk-current-vs-proposed.md).

## Development

### Prerequisites

- Go 1.26+

### Build & Test

```bash
go build ./cmd/hawk           # Build binary
go test -race ./...           # Run all tests with race detector
make ci                       # Run full CI suite (lint, test, security)
make cover                    # Generate coverage report
```

### Project Structure

hawk follows Go conventions: `cmd/` for entry points, `internal/` for private code, tests alongside source files. See [docs/architecture.md](docs/architecture.md) for details.

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
