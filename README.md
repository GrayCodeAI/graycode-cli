# 🦅 hawk

AI coding agent that reads, writes, and runs code in your terminal.

Built on [eyrie](https://github.com/GrayCodeAI/eyrie) (universal LLM provider), [tok](https://github.com/GrayCodeAI/tok) (tokenizer/compression), [yaad](https://github.com/GrayCodeAI/yaad) (graph memory), and [trace](https://github.com/GrayCodeAI/trace) (session capture).

## Install

```bash
# Homebrew (macOS/Linux)
brew install GrayCodeAI/tap/hawk

# Go
go install github.com/GrayCodeAI/hawk@latest

# Script (with checksum verification)
curl -fsSL https://raw.githubusercontent.com/GrayCodeAI/hawk/main/install.sh | sh

# From source
git clone https://github.com/GrayCodeAI/hawk && cd hawk && go install .
```

## Quick Start

```bash
export ANTHROPIC_API_KEY=sk-ant-...
hawk
```

## Model Agnostic

Hawk is fully model-agnostic. Model discovery, pricing, and routing are handled by **eyrie**. Hawk stores only provider, model, and API-key configuration.

```bash
# One-time setup
hawk config provider openai
hawk config key openai sk-...
hawk config model gpt-4o

# Per-run override
hawk --provider openai --model gpt-4o

# Non-interactive
hawk -p "summarize this repo" --provider anthropic --model claude-sonnet-4-20250514
```

In chat: `/config provider <name>`, `/config key <provider> <api-key>`, `/model <name>`.

## Usage

```bash
hawk                          # Interactive REPL
hawk -p "explain this code"   # Print response and exit
hawk -m gpt-4o                # Specify model
hawk --provider openai        # Force provider
hawk -r abc123                # Resume session
hawk -c                       # Continue latest session
hawk --fork-session -r abc123 # Resume as new session
hawk --mcp "npx @mcp/server"  # Connect MCP server
hawk -p "fix tests" --allowed-tools "Bash(go test:*) Edit Read"
hawk -p "plan only" --permission-mode plan --tools "Read,Grep,Glob"
hawk exec "fix all tests"     # Non-interactive with full engine
hawk exec --auto full -o json "add error handling"
hawk exec --worktree "refactor auth module"
hawk exec --agent reviewer "review last commit"
hawk daemon start             # Background HTTP server
hawk daemon status            # Check if daemon running
hawk mission "add auth, rate limiting, and tests"
hawk mission --dry-run "decompose this into features"
hawk search "authentication"  # Search across all sessions
hawk agent list               # List custom personas
hawk agent create debugger    # Create new persona
hawk doctor                   # Run diagnostics
hawk config                   # Show effective settings
hawk sessions                 # List saved sessions
hawk tools                    # List built-in tools
hawk skills search api        # Search community skills
hawk skills install GrayCodeAI/hawk-skills go-review
hawk skills audit             # Security scan installed skills
hawk --auto-skill             # Auto-detect project, install matching skills
```

## Skills

Hawk has a community skill registry — modular instruction packages that teach the agent specialized workflows.

```bash
# In the REPL
/skills                          # List installed skills
/skills search <query>           # Search community registry
/skills trending                 # Most popular skills
/skills install <owner/repo>     # Install from GitHub
/skills use <name>               # Activate for this session
/skills deactivate <name>        # Deactivate
/skills new <description>        # Create a new skill (LLM wizard)
/skills info <name>              # Show details
/skills remove <name>            # Uninstall
/skills feedback <name> <1-5>    # Rate a skill
/skills audit                    # Security scan for hidden Unicode threats
/skills publish <dir>            # Validate and publish

/learn                           # LLM-powered skill advisor
/learn deep                      # Advisor with source file analysis
/learn update                    # Re-analyze and flag outdated skills

# Non-interactive (CI/scripts)
hawk skills list
hawk skills search api --category engineering --json
hawk skills install GrayCodeAI/hawk-skills go-review --scope user
hawk skills audit --json
hawk --auto-skill
```

Skills are discovered from `.hawk/skills/`, `.agents/skills/` (agentskills.io), `~/.hawk/skills/`, and Claude/Codex directories for cross-agent compatibility. Install includes automatic security scanning — skills with dangerous hidden Unicode are sanitized on install.

## Tools (40)

| Tool | Description |
|---|---|
| `Bash` | Run shell commands |
| `Read` | Read files with line ranges |
| `Write` | Create/overwrite files |
| `Edit` | String replacement editing |
| `LS` | Directory listing |
| `Glob` | File pattern matching |
| `Grep` | Regex search in files |
| `WebFetch` | Fetch URLs, HTML→text |
| `WebSearch` | DuckDuckGo search |
| `ToolSearch` | Search available tools |
| `Skill` | Load local skill instructions |
| `Agent` | Spawn sub-agents for parallel tasks |
| `AskUserQuestion` | Ask clarifying questions |
| `TodoWrite` | Task list management |
| `TaskCreate/Get/List/Update` | Persistent task CRUD |
| `TaskOutput` | Read background task output |
| `TaskStop` | Stop a background task |
| `LSP` | Code diagnostics (go vet, tsc, etc.) |
| `EnterPlanMode` / `ExitPlanMode` | Plan mode control |
| `EnterWorktree` / `ExitWorktree` | Git worktree isolation |
| `NotebookEdit` | Edit Jupyter notebook cells |
| `ListMcpResources` / `ReadMcpResource` | MCP resource access |
| `Config` | Read/modify hawk config |
| `SendUserMessage` | Send brief status update |
| `Sleep` | Pause execution |
| `CronCreate/Delete/List` | Scheduled task management |
| `VerifyPlanExecution` | Verify plan completion |
| `Workflow` | Execute scripted workflows |
| `McpAuth` | MCP authentication |
| `Diagnostics` | System diagnostics |
| `CodeSearch` | Semantic code search via yaad |
| `PowerShell` | PowerShell commands (cross-platform) |

Plus any tools from connected MCP servers.

## Slash Commands (91)

Core commands:

| Command | Description |
|---|---|
| `/help` | Show commands |
| `/model` | Show or switch model |
| `/config` | Open config panel |
| `/env` | Show provider environment |
| `/cost` | Token usage and cost |
| `/diff` | Show git diff (preview before commit) |
| `/commit` | Auto-commit with AI message |
| `/undo` | Restore last file change from backup |
| `/focus <path>` | Narrow agent to specific files/dirs |
| `/pin [n]` | Protect last N messages from compaction |
| `/compact` | Compact context |
| `/clear` | Clear display |
| `/branch` | Show git branch/status |
| `/files` | Show modified files |
| `/review` | Ask hawk to review changes |
| `/test` | Run project tests |
| `/lint` | Run linter |
| `/think <topic>` | Plan before coding (Waza method) |
| `/hunt <symptom>` | Root-cause diagnosis before fixing (Waza method) |
| `/check` | Pre-ship review with auto-fix (Waza method) |
| `/design <goal>` | Screenshot-driven UI iteration (Waza method) |
| `/bughunter` | Hunt for bugs |
| `/security-review` | Review security risks |
| `/power <level>` | Set power level (1-10) |
| `/vibe` | Enter vibe coding mode |
| `/research <cmd>` | Autonomous optimization loop |
| `/tools` | List enabled tools |
| `/skills` | List local skills |
| `/memory` | Show loaded project instructions |
| `/history` | List sessions |
| `/resume <id>` | Resume session |
| `/doctor` | Run diagnostics |
| `/init` | Analyze project |
| `/vim` | Toggle vim keybindings |
| `/permissions` | Manage tool permissions |

Run `/help` for the full list of 91 commands.

## Providers

Hawk passes configured provider/model values to eyrie. API keys via `hawk config key <provider> <api-key>` or environment variables:

| Provider | Env Variable |
|---|---|
| Anthropic | `ANTHROPIC_API_KEY` |
| OpenAI | `OPENAI_API_KEY` |
| Gemini | `GEMINI_API_KEY` |
| OpenRouter | `OPENROUTER_API_KEY` |
| Grok | `XAI_API_KEY` |
| Groq | `GROQ_API_KEY` |
| DeepSeek | `DEEPSEEK_API_KEY` |
| Mistral | `MISTRAL_API_KEY` |
| Bedrock | `AWS_ACCESS_KEY_ID` |
| Vertex | `GOOGLE_APPLICATION_CREDENTIALS` |
| Ollama | `OLLAMA_BASE_URL` |

## Architecture

| Layer | Technology |
|---|---|
| CLI | [cobra](https://github.com/spf13/cobra) |
| TUI | [Bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss) |
| LLM | [eyrie](https://github.com/GrayCodeAI/eyrie) — model-agnostic provider |
| Tokenizer | [tok](https://github.com/GrayCodeAI/tok) — BPE counting + context compression |
| Memory | [yaad](https://github.com/GrayCodeAI/yaad) — graph-based persistent memory |
| Session capture | [trace](https://github.com/GrayCodeAI/trace) — git-native session recording |
| MCP | JSON-RPC over stdio |
| Hooks | Event-driven plugin system |
| Plugins | Manifest-based external commands |
| LSP | JSON-RPC language server client |
| Sandbox | Namespace/docker/chroot/seatbelt/landlock/seccomp isolation |

### Package Structure

```
cmd/          CLI commands and TUI (split: chat, commands, config, view, stream, print, welcome)
engine/       Agent loop, compaction, beliefs, backtracking, cascade router, context budget,
              reflection, self-review, session lifecycle (self-improvement loop)
tool/         40 built-in tools with safety layer (credentials, sensitive paths, backups)
config/       Settings loading, validation, budget, aliases, templates
session/      Session persistence (JSONL), WAL, snapshots, checkpoints, compression
prompt/       System prompt preamble (identity, safety)
prompts/      Modular prompt templates (role, tools, practices, communication, examples)
repomap/      Code intelligence (PageRank, BM25, TF-IDF, Shapley, import graph, change-set context)
memory/       Unified MemoryManager (auto, evolving, zenbrain, yaad bridge)
model/        Provider routing, health checking, roles (delegates catalog to eyrie)
mcp/          MCP client with buffered I/O and timeout
lsp/          LSP client with persistent reader
plugin/       Plugin runtime and smart skill auto-invocation
permissions/  Advanced permission system (auto-mode, classifier, killswitch)
hooks/        Event hook system with decision hooks
analytics/    Session traces, activity tracking, cost optimization, model cascade
trace/        Built-in tracer + optional OTel SDK (build tag)
sandbox/      Command isolation (namespace, docker, chroot, seatbelt, landlock, seccomp)
mission/      Multi-agent orchestration (parallel feature execution in worktrees)
daemon/       Background HTTP server (JSON + SSE streaming)
agents/       Custom agent persona loader (markdown + frontmatter)
parallel/     Worktree-based parallel task execution
retry/        Exponential backoff with jitter
circuit/      Circuit breaker (closed/open/half-open)
ratelimit/    Token bucket rate limiting
```

Zero CGO. Single static binary. Cross-compiled for linux/darwin/windows amd64/arm64.

## Exec Mode

Run hawk non-interactively for scripting and CI:

```bash
hawk exec "fix all failing tests"
hawk exec --auto full "refactor to use context"
hawk exec --output-format json "list all TODO comments"
hawk exec --worktree "add rate limiting"           # isolated branch
hawk exec --agent reviewer "review the last 3 commits"
hawk exec -s exec-123456 "continue from last time" # resume session
echo "explain main.go" | hawk exec -               # stdin
```

## Mission Mode

Decompose work into parallel features:

```bash
hawk mission "Add auth, rate limiting, and logging"
hawk mission --workers 6 --model claude-sonnet-4-6 "Refactor into microservices"
hawk mission --dry-run "What would this decompose into?"
```

Each feature runs in its own git worktree with full autonomy. Results are committed on separate branches.

## Daemon

Background HTTP server for programmatic access:

```bash
hawk daemon start              # Start on port 4590
hawk daemon start --port 8080  # Custom port
hawk daemon status             # Check if running
hawk daemon stop               # Graceful shutdown
```

Endpoints:
- `GET /v1/health` — server status
- `POST /v1/chat` — send prompt, get response (JSON or SSE streaming)
- `GET /v1/sessions` — list active sessions

SSE streaming: set `Accept: text/event-stream` header.

## Custom Agents

Define reusable personas in `~/.hawk/agents/`:

```bash
hawk agent create reviewer -d "Security-focused code reviewer"
hawk agent list
hawk agent show reviewer
hawk exec --agent reviewer "check the auth module"
```

Agent files are markdown with YAML frontmatter:

```markdown
---
name: reviewer
description: Security-focused code reviewer
model: claude-sonnet-4-6
---
You are a security expert. Focus on injection, auth bypass, and secrets.
```

## AGENTS.md

Create an `AGENTS.md` in your project root for project-specific instructions (max 10KB). Any AI coding agent can read this — hawk, Claude Code, Cursor, Codex:

```markdown
# My Project
- Go project using chi router
- Tests use testify
- Run tests: go test ./...
```

Hawk also reads `AGENTS.md` for backward compatibility.

## Permission System

Hawk asks before running dangerous tools (`Bash`, `Write`, `Edit`, `NotebookEdit`):

```
⚠ Run: go test ./...  [y/n]
```

Features:
- **Auto-mode**: Learns from your decisions
- **Command classifier**: Safe/unsafe/unknown classification
- **Bypass killswitch**: Emergency disable for auto-mode
- **Shadowed rule detection**: Warns when rules conflict

```bash
hawk -p "review this repo" --permission-mode acceptEdits
hawk -p "fix lint" --allowed-tools "Bash(go test:*) Edit Read"
hawk -p "plan only" --permission-mode plan
```

## MCP (Model Context Protocol)

```bash
hawk --mcp "npx @modelcontextprotocol/server-filesystem ."
hawk --mcp "npx @modelcontextprotocol/server-github"
```

## License

MIT — [GrayCode AI](https://github.com/GrayCodeAI)
