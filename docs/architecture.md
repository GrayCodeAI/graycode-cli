# Architecture

hawk is a terminal-native AI coding agent built in Go. It reads, writes, and runs code through natural language interaction.

## System Overview

```
┌──────────────────────────────────────────────────────────┐
│                      hawk CLI                             │
│              cmd/ → cobra + bubbletea TUI                 │
├──────────────────────────────────────────────────────────┤
│                    internal/engine/                        │
│         Agent loop, compaction, self-improvement          │
├──────────┬──────────┬───────────┬──────────┬─────────────┤
│ internal │ internal │ internal  │ internal │  external   │
│ /tool/   │/session/ │ /config/  │/sandbox/ │  services   │
├──────────┴──────────┴───────────┴──────────┴─────────────┤
│                    eyrie (LLM runtime)                    │
│        tok (tokenizer)  ·  yaad (memory)  ·  trace       │
└──────────────────────────────────────────────────────────┘
```

## Directory Structure

```
hawk/
├── cmd/                    # CLI entry point (Cobra + Bubble Tea TUI)
├── internal/
│   ├── engine/             # Agent loop, compaction, beliefs, self-review
│   │   ├── compact/        # Context compaction strategies
│   │   ├── cost/           # Cost tracking & optimization
│   │   ├── token/          # Token counting & budget allocation
│   │   ├── prompt/         # Prompt construction & optimization
│   │   ├── diff/           # Diff handling, sandbox, summary
│   │   ├── review/         # Critic, self-assess, consensus
│   │   ├── control/        # Loop detection, stall, backtrack
│   │   ├── git/            # Git provider + context
│   │   └── ...             # 15+ more sub-packages
│   ├── tool/               # 40+ built-in tools with safety layer
│   ├── config/             # Settings, budget tracking, validation
│   ├── session/            # Persistence (JSONL, WAL, checkpoints)
│   ├── api/                # HTTP API server
│   ├── daemon/             # Background HTTP/SSE server
│   ├── sandbox/            # Command isolation (landlock, seccomp)
│   ├── permissions/        # User approval system with auto-learning
│   ├── hooks/              # Event-driven plugin system
│   ├── mcp/                # Model Context Protocol client
│   ├── intelligence/       # Code intelligence
│   │   ├── repomap/        # PageRank, BM25, TF-IDF for file relevance
│   │   ├── memory/         # yaad bridge for persistent cross-session memory
│   │   └── planner/        # Multi-step planning with decomposition
│   ├── multiagent/         # Multi-agent orchestration
│   │   ├── mission/        # Parallel feature execution in worktrees
│   │   ├── parallel/       # Worktree-based parallel execution
│   │   └── agents/         # Custom persona loader
│   ├── observability/      # Analytics, metrics, logging, tracing
│   ├── resilience/         # Circuit breaker, rate limiting, retries, health
│   ├── feature/            # eval, fingerprint, voice, IDE, shellmode
│   ├── bridge/             # External bridges (sight, inspect, sessioncapture)
│   ├── provider/           # Provider routing
│   └── system/             # Bus, cron, retention, shutdown, staleness
├── docs/                   # Architecture, research notes
└── testdata/               # Test fixtures
```

## Data Flow

```
User prompt
    │
    ▼
┌─────────┐     ┌──────────────┐     ┌─────────┐
│  cmd/   │────▶│  engine/     │────▶│  eyrie  │
│ (TUI)   │     │ (agent loop) │     │  (LLM)  │
└─────────┘     └──────┬───────┘     └────┬────┘
                       │                  │
                       ▼                  │
                 ┌──────────┐             │
                 │  tool/   │◀────────────┘
                 │ (execute)│  tool calls + text
                 └────┬─────┘
                      │
                      ▼
                 ┌──────────┐
                 │ session/ │  (persist)
                 └──────────┘
```

1. **Input** — User types prompt in TUI (`cmd/` via Bubble Tea)
2. **Context assembly** — `engine/` builds message array with repomap, memory, system prompt
3. **LLM call** — `eyrie` sends to provider (streaming SSE)
4. **Response** — LLM returns text + tool calls
5. **Tool execution** — `engine/` executes via `tool.Registry` (with permission checks)
6. **Feedback loop** — Tool results fed back to LLM for next turn
7. **Persistence** — Final response displayed, session saved to WAL

## Key Design Decisions

| Decision | Rationale |
|---|---|
| **Single static binary** | Zero runtime dependencies, easy distribution |
| **Streaming-first** | Token-by-token display, low perceived latency |
| **WAL crash recovery** | No data loss on unexpected exit |
| **Permission sandboxing** | All tool calls gated by configurable permission engine |
| **Model-agnostic** | eyrie abstracts all provider differences |
| **Offline-capable** | Works with local models (Ollama) without API keys |
| **`internal/` boundary** | Compiler-enforced privacy, free to refactor |

## Security Model

- **Sandbox** — Landlock (Linux), seccomp-bpf, seatbelt (macOS), no-op on Windows
- **Permissions** — Ask-before-execution for Bash, Write, Edit; auto-learn from decisions
- **Secrets** — API keys never logged, masked in config display, stored encrypted at rest
- **Path validation** — Symlink traversal prevention, no escape from working directory
