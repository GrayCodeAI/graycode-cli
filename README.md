# 🦅 hawk

AI coding agent that reads, writes, and runs code in your terminal.

Built on [eyrie](https://github.com/GrayCodeAI/eyrie) — the universal LLM provider library.

## Install

```bash
go install github.com/GrayCodeAI/hawk@latest
```

## Usage

```bash
# Interactive REPL
hawk

# Single prompt
hawk -p "explain this codebase"

# Specify model
hawk -m claude-sonnet-4-20250514
```

## Status

**v0.0.1** — Project scaffold. The agent loop, tools, and LLM integration are coming in Phase 1-2.

## Architecture

- **CLI**: [cobra](https://github.com/spf13/cobra)
- **TUI**: [Bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss)
- **LLM**: [eyrie](https://github.com/GrayCodeAI/eyrie) — Anthropic, OpenAI, Gemini, and 200+ models
- **Tools**: File read/write/edit, bash, glob, grep, web fetch/search (coming soon)

## License

MIT — see [LICENSE](LICENSE).
