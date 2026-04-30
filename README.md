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
hawk -p "explain this code"   # Single prompt
hawk -m gpt-4o                # Specify model
hawk --provider openai        # Force provider
hawk -r abc123                # Resume session
hawk --mcp "npx @mcp/server"  # Connect MCP server
```

## Tools (12)

| Tool | Description |
|---|---|
| `bash` | Run shell commands |
| `file_read` | Read files with line ranges |
| `file_write` | Create/overwrite files |
| `file_edit` | String replacement editing |
| `glob` | File pattern matching |
| `grep` | Regex search in files |
| `web_fetch` | Fetch URLs, HTML→text |
| `web_search` | DuckDuckGo search |
| `agent` | Spawn sub-agents for parallel tasks |
| `ask_user` | Ask clarifying questions |
| `todo` | Task list management |
| `lsp` | Code diagnostics (go vet, tsc, etc.) |

Plus any tools from connected MCP servers.

## Slash Commands

| Command | Description |
|---|---|
| `/help` | Show commands |
| `/clear` | Clear display |
| `/compact` | Compact context |
| `/cost` | Token usage and cost |
| `/diff` | Review changes |
| `/model` | Show model |
| `/history` | List sessions |
| `/resume <id>` | Resume session |
| `/commit` | Auto-commit with AI message |
| `/doctor` | Run diagnostics |
| `/init` | Analyze project |
| `/permissions allow <tool>` | Always allow a tool |

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

MCP tools appear as `mcp_<server>_<tool>` in the tool registry.

## HAWK.md

Create a `HAWK.md` in your project root for project-specific instructions:

```markdown
# My Project
- Go project using chi router
- Tests use testify
- Run tests: go test ./...
```

## Permission System

hawk asks before running dangerous tools (bash, file_write, file_edit):

```
⚠ Run: go test ./...  [y/n]
```

Use `/permissions allow bash` to always allow a tool for the session.

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
