<p align="center">
  <img src="https://github.com/GrayCodeAI/hawk/raw/main/assets/logo.svg" alt="hawk" width="480"/>
</p>

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

**Hawk is in active development.** There is no public install script or release channel yet. We are building features, tests, and hardening in the open.

Follow [GrayCode](https://github.com/GrayCodeAI) for progress. When Hawk is ready to try, we will announce it on [graycodeai.gateandtech.in](https://graycodeai.gateandtech.in/changelog).

## Quick Start (contributors — from source)

```bash
git clone https://github.com/GrayCodeAI/hawk && cd hawk
go build -o hawk ./cmd/hawk
./hawk

# First run — paste API key in /config (stored in macOS Keychain / Linux keyring)
# Verify readiness
./hawk path
```

See [docs/SECURITY-DEVELOPER.md](docs/SECURITY-DEVELOPER.md) for the credential model. Do not put API keys in shell env or `.env` for hawk.

Optional for contributors:

```bash
go install github.com/GrayCodeAI/hawk@latest   # requires a published tag
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
| **Code** | `LSP` diagnostics, `CodeSearch`, `NotebookEdit` |
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

### Permission System

hawk asks before running dangerous tools. Auto-mode learns from your decisions, with emergency killswitch support.

### MCP & LSP Support

Connect external tools via [Model Context Protocol](https://modelcontextprotocol.io/) and get code intelligence through Language Server Protocol.

## Usage

### Interactive Mode

```bash
hawk                              # Start REPL
hawk -r abc123                    # Resume session
hawk -c                           # Continue latest session
hawk --provider openai --model gpt-4o  # Override provider
```

### Non-Interactive Mode

```bash
hawk -p "explain this repo"                    # Print response, exit
hawk -p "fix tests" --allowed-tools "Bash(go test:*) Edit Read"
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

Endpoints: `GET /v1/health`, `POST /v1/chat` (JSON or SSE streaming)

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

hawk integrates these GrayCodeAI repos in three ways:

- **`go.mod` modules:** **eyrie**, **sight**, **inspect**, **tok**, **yaad** — pinned module requirements.
- **External checkout + `go.work`:** **eyrie**, **sight**, **inspect**, **tok**, **yaad**, **trace** — clone ecosystem repos under `external/<repo>`. `go.work` lists the local external checkouts. CI clones the same layout via **`.github/actions/checkout-eyrie`**.
- **Optional CLI (no Go import):** **trace** — installed separately; `hawk` shells into `trace` for session capture when present.

Cross-repo types (severity, etc.) are exported from **`github.com/GrayCodeAI/hawk/shared/types`** so **sight** / **inspect** / **tok** do not import **`internal/`**.

You may keep a **personal** parent **`go.work`** that lists alternate clones on disk for multi-repo development.

| Component | Repository | Purpose |
|---|---|---|
| **hawk** | This repo | AI coding agent |
| **eyrie** | [GrayCodeAI/eyrie](https://github.com/GrayCodeAI/eyrie) | LLM provider runtime |
| **sight** | [GrayCodeAI/sight](https://github.com/GrayCodeAI/sight) | Diff-based code review (`hawk sight`) |
| **inspect** | [GrayCodeAI/inspect](https://github.com/GrayCodeAI/inspect) | Site audit library |
| **tok** | [GrayCodeAI/tok](https://github.com/GrayCodeAI/tok) | Tokenizer & compression |
| **yaad** | [GrayCodeAI/yaad](https://github.com/GrayCodeAI/yaad) | Graph-based memory |
| **trace** | [GrayCodeAI/trace](https://github.com/GrayCodeAI/trace) | Session capture CLI |

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
