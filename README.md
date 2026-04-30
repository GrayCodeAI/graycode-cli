# 🦅 hawk

AI coding agent that reads, writes, and runs code in your terminal.

Built on [eyrie](https://github.com/GrayCodeAI/eyrie) — the universal LLM provider library.

## Install

```bash
go install github.com/GrayCodeAI/hawk@latest
```

## Quick Start

```bash
# Set your API key
export ANTHROPIC_API_KEY=sk-ant-...

# Launch interactive REPL
hawk

# Single prompt
hawk -p "explain this codebase"

# Specify model
hawk -m claude-sonnet-4-20250514

# Resume a previous session
hawk -r abc123
```

## Features

- **Agentic loop** — hawk calls tools, reads results, and keeps going until the task is done
- **Streaming** — token-by-token output in a Bubbletea TUI
- **6 built-in tools** — bash, file_read, file_write, file_edit, glob, grep
- **Multi-provider** — Anthropic, OpenAI, Gemini, Groq, Ollama, OpenRouter, and more via eyrie
- **Session persistence** — conversations saved to `~/.hawk/sessions/`
- **Context-aware** — reads HAWK.md, git status, and cwd into the system prompt
- **Cost tracking** — token usage and estimated cost per session
- **Graceful cancel** — Ctrl+C cancels the current stream, second Ctrl+C quits

## Slash Commands

| Command | Description |
|---|---|
| `/help` | Show available commands |
| `/clear` | Clear the display |
| `/cost` | Show token usage and cost |
| `/model` | Show current provider/model |
| `/history` | List saved sessions |
| `/resume <id>` | Resume a saved session |
| `/quit` | Exit hawk |

## Providers

hawk auto-detects your provider from environment variables:

| Provider | Env Variable |
|---|---|
| Anthropic | `ANTHROPIC_API_KEY` |
| OpenAI | `OPENAI_API_KEY` |
| Gemini | `GEMINI_API_KEY` |
| Groq | `GROQ_API_KEY` |
| OpenRouter | `OPENROUTER_API_KEY` |
| Ollama | `OLLAMA_BASE_URL` |

Or force a provider: `hawk --provider openai`

## HAWK.md

Create a `HAWK.md` file in your project root to give hawk project-specific instructions:

```markdown
# Project: my-app
- This is a Go project using chi router
- Tests use testify
- Run tests with: go test ./...
```

## Architecture

- **CLI**: [cobra](https://github.com/spf13/cobra)
- **TUI**: [Bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss)
- **LLM**: [eyrie](https://github.com/GrayCodeAI/eyrie) — zero-dependency Go LLM client
- **Tools**: bash, file_read, file_write, file_edit, glob, grep

## License

MIT — [GrayCode AI](https://github.com/GrayCodeAI)
