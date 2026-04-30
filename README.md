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
| `/model` | Show model |
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
| `/version` | Show hawk version |
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

Settings are loaded from `~/.hawk/settings.json` and `.hawk/settings.json`, with project settings overriding global settings. Both snake_case Go keys and archive-style aliases are accepted, including `apiKey`, `autoAllow`, `maxBudgetUSD`, `customHeaders`, `mcpServers`, `allowed_tools`, and `disallowed_tools`.

## Providers

Auto-detected from environment:

| Provider | Env Variable |
|---|---|
| Anthropic | `ANTHROPIC_API_KEY` |
| OpenAI | `OPENAI_API_KEY` |
| Gemini | `GEMINI_API_KEY` |
| Groq | `GROQ_API_KEY` |
| OpenRouter | `OPENROUTER_API_KEY` |
| Grok | `XAI_API_KEY` |
| Ollama | `OLLAMA_BASE_URL` |

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

Zero CGO. Single static binary. Cross-compiled for linux/darwin/windows amd64/arm64.

## License

MIT — [GrayCode AI](https://github.com/GrayCodeAI)
