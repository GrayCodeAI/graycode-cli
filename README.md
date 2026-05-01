# 🦅 hawk

AI coding agent that reads, writes, and runs code in your terminal.

Built on [eyrie](https://github.com/GrayCodeAI/eyrie) — the universal LLM provider library.

## Install

```bash
# Homebrew (macOS/Linux)
brew install GrayCodeAI/tap/hawk

# Go
go install github.com/GrayCodeAI/hawk@latest

# Script
curl -fsSL https://raw.githubusercontent.com/GrayCodeAI/hawk/main/install.sh | sh

# From source
git clone https://github.com/GrayCodeAI/hawk && cd hawk && go install .
```

## Quick Start

```bash
export ANTHROPIC_API_KEY=sk-ant-...
hawk
```

## API Key, Model, and Eyrie

Hawk uses **Eyrie** as the LLM client layer. In practice:
- Hawk stores only provider, model, and API-key configuration
- Eyrie owns provider support, model discovery, routing, and request behavior
- API credentials can come from env vars or `~/.hawk/settings.json`

Recommended flow:

```bash
# 1) one-time setup
hawk config provider openai
hawk config key openai sk-...
hawk config model gpt-4o

# 2) optional per-run override
hawk --provider openai --model gpt-4o

# 3) non-interactive usage
hawk -p "summarize this repo" --provider anthropic --model claude-sonnet-4-20250514
```

Notes:
- In chat, use `/config provider <name>`, `/config key <provider> <api-key>`, and `/model <name>`.
- `--provider` and `--model` override saved config for a single run.
- Use `/provider-status`, `/model`, `/config keys`, and `/env` to verify runtime settings.

## Usage

```bash
hawk                          # Interactive REPL
hawk -p "explain this code"   # Print response and exit
hawk --prompt "explain this"  # Legacy single-prompt alias
hawk -m gpt-4o                # Specify model
hawk --provider openai        # Force provider
hawk -r abc123                # Resume session
hawk -c                       # Continue latest session in this directory
hawk --fork-session -r abc123 # Resume as a new session
hawk --mcp "npx @mcp/server"  # Connect MCP server
hawk -p "fix tests" --allowed-tools "Bash(go test:*) Edit Read"
hawk -p "plan only" --permission-mode plan --tools "Read,Grep,Glob"
hawk --settings '{"model":"gpt-4o"}' --add-dir ../shared
hawk doctor                   # Run local diagnostics
hawk config                   # Show effective settings
hawk config get model         # Read a config value
hawk config set model gpt-4o  # Update global config
hawk mcp                      # Show MCP server configuration
hawk sessions                 # List saved sessions
hawk tools                    # List built-in tools
```

## Tools (24)

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
| `TodoWrite` | Task list management, including archive-style `todos` arrays |
| `TaskOutput` | Read background Bash task output |
| `TaskStop` | Stop a background Bash task |
| `LSP` | Code diagnostics (go vet, tsc, etc.) |
| `EnterPlanMode` | Request plan mode |
| `ExitPlanMode` | Leave plan mode |
| `NotebookEdit` | Edit Jupyter notebook cells |
| `ListMcpResourcesTool` | List MCP resources |
| `ReadMcpResourceTool` | Read MCP resources |
| `Config` | Read/modify Hawk config |
| `SendUserMessage` | Send a brief status update |

Lowercase Go-port names like `bash`, `file_read`, and `file_edit` remain accepted as aliases. Plus any tools from connected MCP servers.

## Slash Commands

| Command | Description |
|---|---|
| `/help` | Show commands |
| `/add-dir <path>` | Add a directory to context |
| `/branch` | Show git branch/status |
| `/bughunter` | Ask hawk to hunt for bugs |
| `/clear` | Clear display |
| `/compact` | Compact context |
| `/cost` | Token usage and cost |
| `/diff` | Review changes |
| `/env` | Show provider environment status |
| `/files` | Show modified files |
| `/model` | Show or switch model |
| `/mcp` | Show MCP status |
| `/memory` | Show loaded project instructions |
| `/history` | List sessions |
| `/resume <id>` | Resume session |
| `/commit` | Auto-commit with AI message |
| `/doctor` | Run diagnostics |
| `/init` | Analyze project |
| `/permissions allow <tool>` | Always allow a tool |
| `/permissions deny <tool>` | Always deny a tool |
| `/permissions mode <mode>` | Change permission mode |
| `/pr-comments` | Ask hawk to handle PR comments |
| `/release-notes` | Draft release notes |
| `/review` | Ask hawk to review changes |
| `/security-review` | Ask hawk to review security risks |
| `/skills` | List local skills |
| `/summary` | Summarize the current session |
| `/tools` | List enabled tools |
| `/models` | Explain that Eyrie manages model discovery |
| `/plugin list` | List installed plugins |
| `/plugin-command <name>` | Run a plugin command |
| `/welcome` | Show startup summary |

## Session Flags

Archive-compatible session controls are available:

```bash
hawk --session-id my-session
hawk --continue
hawk --resume my-session --fork-session
hawk -p --input-format stream-json --output-format stream-json < events.jsonl
```

`--settings` accepts either a JSON object or a path to a JSON file. `--add-dir` includes extra directory context, reads `HAWK.md` from those directories when present, and allows file tools to access those roots.

Settings are loaded from `~/.hawk/settings.json` and `.hawk/settings.json`, with project settings overriding global settings. Both snake_case Go keys and archive-style aliases are accepted, including `apiKey`, `apiKeys`, `autoAllow`, `maxBudgetUSD`, `customHeaders`, `mcpServers`, `allowed_tools`, and `disallowed_tools`.

## Providers

Hawk passes configured provider/model values to Eyrie. Provider-specific keys can be saved with `hawk config key <provider> <api-key>` or supplied through environment variables:

| Provider | Env Variable |
|---|---|
| Anthropic | `ANTHROPIC_API_KEY` |
| OpenAI | `OPENAI_API_KEY` |
| Gemini | `GEMINI_API_KEY` |
| OpenRouter | `OPENROUTER_API_KEY` |
| Grok | `XAI_API_KEY` |
| Ollama | `OLLAMA_BASE_URL` |

## Model Catalog

Model lists, supported providers, pricing, and routing behavior are handled by Eyrie. Hawk does not carry a hardcoded model catalog.

## Plugin System

Install and manage plugins:

```bash
hawk plugin list          # List installed plugins
hawk plugin install ./my-plugin
hawk plugin uninstall my-plugin
```

Plugins can provide commands, skills, and hooks. See `plugin/` package for manifest format and runtime details.

## Advanced Permissions

Beyond basic allow/deny, hawk includes:

- **Auto-mode**: Learns from your decisions and auto-allows/denies similar commands
- **Command classifier**: Classifies commands as safe/unsafe/unknown
- **Bypass killswitch**: Emergency disable for auto-mode
- **Shadowed rule detection**: Warns when allow rules are hidden by broader deny rules

```bash
hawk -p "review this repo" --permission-mode acceptEdits
```

## Session Memory

hawk extracts and stores important decisions from sessions:

```bash
hawk /memory              # Show loaded project instructions
```

Memories are automatically extracted from messages containing keywords like "Important", "Note", "Remember", etc.

## Analytics

Session traces and events are logged for analysis:

```bash
# Analytics are stored in ~/.hawk/analytics/
# Use the API to query session costs, provider usage, etc.
```

## Auto-Update

Check for updates:

```bash
hawk /version             # Shows current version
# hawk update             # Check and install updates (future)
```

## IDE Integration

 hawk provides IDE integration hints:

- VSCode extension manifest generation
- LSP server configuration suggestions
- Keyboard shortcut recommendations

See `ide/` package for extension development support.

## Sandbox Mode (Experimental)

Run commands in isolated environments:

```bash
# Supports namespace, docker, and chroot isolation
# Configure in settings: {"sandbox": {"enabled": true, "type": "docker"}}
```

## MCP (Model Context Protocol)

Connect external tool servers:

```bash
hawk --mcp "npx @modelcontextprotocol/server-filesystem ."
hawk --mcp "npx @modelcontextprotocol/server-github"
```

MCP tools appear as `mcp__<server>__<tool>` in the tool registry. The older `mcp_<server>_<tool>` form remains accepted as an alias.

## HAWK.md

Create a `HAWK.md` in your project root for project-specific instructions:

```markdown
# My Project
- Go project using chi router
- Tests use testify
- Run tests: go test ./...
```

## Permission System

hawk asks before running dangerous tools (`Bash`, `Write`, `Edit`, `NotebookEdit`):

```
⚠ Run: go test ./...  [y/n]
```

Use `/permissions allow Bash` to always allow a tool for the session. Lowercase aliases such as `/permissions allow bash` are also accepted.

Non-interactive mode accepts archive-style permission controls:

```bash
hawk -p "review this repo" --tools "Read,Grep,Glob"
hawk -p "fix lint" --allowed-tools "Bash(go test:*) Edit Read"
hawk -p "do not modify files" --permission-mode plan
hawk -p "run without prompts" --permission-mode bypassPermissions
```

Supported permission modes are `default`, `acceptEdits`, `bypassPermissions`, `dontAsk`, and `plan`. Permission rules support archive syntax such as `Bash(git:*)`, `Write(*.env)`, and bare tool names.

## Architecture

| Layer | Technology |
|---|---|
| CLI | [cobra](https://github.com/spf13/cobra) |
| TUI | [Bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss) |
| LLM | [eyrie](https://github.com/GrayCodeAI/eyrie) |
| MCP | JSON-RPC over stdio |
| Hooks | Event-driven plugin system |
| Plugins | Manifest-based external commands |
| LSP | JSON-RPC language server client |
| Sandbox | Namespace/docker/chroot isolation |

### Package Structure

- `cmd/` - CLI commands and TUI
- `engine/` - Agent loop, session management, streaming
- `tool/` - Built-in tools (Bash, Read, Write, Edit, etc.)
- `config/` - Settings loading and validation
- `session/` - Session persistence (JSONL format)
- `prompt/` - System prompt construction
- `mcp/` - MCP client and server management
- `hooks/` - Event hook system
- `plugin/` - Plugin runtime and manifest validation
- `permissions/` - Permission checking and advanced features
- `model/` - Model catalog and provider routing
- `memory/` - Session memory extraction
- `analytics/` - Event logging and session traces
- `auth/` - Token storage and OAuth
- `update/` - Auto-update checking
- `lsp/` - LSP client and server manager
- `voice/` - STT integration
- `magicdocs/` - Automatic documentation generation
- `ide/` - IDE integration hints
- `remote/` - Remote session management
- `sandbox/` - Command isolation

Zero CGO. Single static binary. Cross-compiled for linux/darwin/windows amd64/arm64.

## License

MIT — [GrayCode AI](https://github.com/GrayCodeAI)
